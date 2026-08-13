package cmd

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

//go:embed backup_paths.html
var backupPathsHTML string

var backupPathsCmd = &cobra.Command{
	Use:   "paths",
	Short: "Pick what to back up in a local web UI",
	Long: "Open a local web page to browse this machine's folders and choose what restic " +
		"backs up. Include or exclude a folder (and everything under it), exclude a single " +
		"file, or exclude files like it (*.ext). Folder sizes are shown so you can spot what " +
		"to cut. Saving writes " + component.ResticPathsFile + " and " + component.ResticExcludesFile + ".",
	RunE: runBackupPaths,
}

// sessionCookie carries the API token between page loads. The token itself
// never appears in a URL, so it can't leak via browser history or the argv of
// the browser-opening command.
const sessionCookie = "ctdev_token"

// pathPicker holds the in-memory state for one picker session. The server is the
// source of truth for the include/exclude lists; the browser just renders them.
type pathPicker struct {
	bootToken string // single-use launch token, carried in the opened URL
	apiToken  string // session token, exchanged into a cookie on first load
	home      string
	includes  map[string]bool
	excludes  []string

	mu        sync.Mutex
	bootUsed  bool // the boot token authenticates exactly one browser
	sizeCache map[string]int64

	done     chan struct{}
	doneOnce sync.Once
	dryRun   bool
}

func runBackupPaths(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)

	if isBatchMode() {
		return fmt.Errorf("ctdev backup paths needs a browser; run it in an interactive session (or edit %s directly)", component.ResticPathsFile)
	}
	if err := ensureSudo(cmdContext(cmd)); err != nil {
		return fmt.Errorf("sudo required (to read/write %s): %w", component.ResticPathsFile, err)
	}
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	if !flagDryRun {
		if err := sysutil.SudoRun(ctx, o, "install", "-d", "-m", "700", "/etc/restic"); err != nil {
			return fmt.Errorf("create /etc/restic: %w", err)
		}
	}

	home, _ := os.UserHomeDir()
	bootTok, err := generatePassword(24)
	if err != nil {
		return err
	}
	apiTok, err := generatePassword(24)
	if err != nil {
		return err
	}
	p := &pathPicker{
		bootToken: bootTok,
		apiToken:  apiTok,
		home:      home,
		includes:  map[string]bool{},
		excludes:  nil,
		sizeCache: map[string]int64{},
		done:      make(chan struct{}),
		dryRun:    flagDryRun,
	}
	p.load(ctx)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start local server: %w", err)
	}
	pageURL := fmt.Sprintf("http://%s/?t=%s", ln.Addr().String(), bootTok)

	srv := &http.Server{Handler: p.routes()}
	go srv.Serve(ln)

	fmt.Println(styles.Title.Render("Backup path picker"))
	fmt.Printf("  Open: %s\n", styles.Value.Render(pageURL))
	if err := openBrowser(pageURL); err != nil {
		fmt.Println(styles.Dimmed.Render("  (couldn't open a browser automatically)"))
	}
	fmt.Println(styles.Dimmed.Render("  On a headless/remote host, forward the port from your laptop, e.g.:"))
	fmt.Printf("    %s\n", styles.Dimmed.Render(fmt.Sprintf("ssh -L %[1]s:localhost:%[1]s %s", portOf(ln), hostLabel())))
	fmt.Println(styles.Dimmed.Render("  Waiting for you to Save & Close (Ctrl-C to cancel)..."))

	select {
	case <-ctx.Done():
		fmt.Println("\n  Cancelled — nothing was saved.")
	case <-p.done:
	}
	shutCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = srv.Shutdown(shutCtx)
	return nil
}

// load seeds the session from the existing files. Backups are opt-in, so an
// empty paths file means nothing is included yet — the user picks. Excludes
// fall back to sensible defaults (they only matter once something is included).
func (inst *pathPicker) load(ctx context.Context) {
	for _, p := range component.ResticReadLines(ctx, component.ResticPathsFile) {
		inst.includes[p] = true
	}
	exc := component.ResticReadLines(ctx, component.ResticExcludesFile)
	if len(exc) == 0 {
		exc = component.DefaultBackupExcludes()
	}
	inst.excludes = exc
}

func (inst *pathPicker) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", inst.handleIndex)
	mux.HandleFunc("/api/state", inst.guard(inst.handleState))
	mux.HandleFunc("/api/list", inst.guard(inst.handleList))
	mux.HandleFunc("/api/size", inst.guard(inst.handleSize))
	mux.HandleFunc("/api/mutate", inst.guard(inst.handleMutate))
	mux.HandleFunc("/api/save", inst.guard(inst.handleSave))
	return mux
}

// guard rejects any /api call without the session cookie or from a
// non-loopback origin — the server can read the filesystem and write root
// files, so a stray page in another tab must not be able to drive it.
func (inst *pathPicker) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !inst.hasSession(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !isLoopbackOrigin(origin) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// hasSession reports whether the request carries the session cookie issued by
// the boot-token exchange.
func (inst *pathPicker) hasSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	return err == nil && c.Value != "" && c.Value == inst.apiToken
}

// handleIndex serves the picker page to an established session, or exchanges
// the single-use boot token from the launch URL for the session cookie. The
// exchange redirects to a clean "/" so the token never lingers in the address
// bar or browser history.
func (inst *pathPicker) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !inst.hasSession(r) {
		inst.mu.Lock()
		ok := !inst.bootUsed && r.URL.Query().Get("t") == inst.bootToken
		if ok {
			inst.bootUsed = true
		}
		inst.mu.Unlock()
		if !ok {
			http.Error(w, "forbidden — this link was already used; relaunch with 'ctdev backup paths'", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookie,
			Value:    inst.apiToken,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, backupPathsHTML)
}

func (inst *pathPicker) handleState(w http.ResponseWriter, r *http.Request) {
	inst.writeState(w)
}

func (inst *pathPicker) handleList(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("path")
	if dir == "" {
		dir = inst.home
	}
	entries, err := listDir(dir)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error(), "entries": []any{}})
		return
	}
	inst.mu.Lock()
	inc := copyMap(inst.includes)
	exc := append([]string(nil), inst.excludes...)
	inst.mu.Unlock()
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		var size any
		if !e.isDir {
			size = e.size
		}
		out = append(out, map[string]any{
			"name":     e.name,
			"path":     e.path,
			"isDir":    e.isDir,
			"hidden":   strings.HasPrefix(e.name, "."),
			"readable": e.readable,
			"size":     size,
			"created":  e.created,
			"state":    entryState(e.path, inc, exc),
		})
	}
	writeJSON(w, map[string]any{"path": dir, "entries": out})
}

func (inst *pathPicker) handleSize(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}
	inst.mu.Lock()
	sz, ok := inst.sizeCache[path]
	inst.mu.Unlock()
	if !ok {
		sz = duBytes(r.Context(), path)
		inst.mu.Lock()
		inst.sizeCache[path] = sz
		inst.mu.Unlock()
	}
	writeJSON(w, map[string]any{"path": path, "size": sz})
}

func (inst *pathPicker) handleMutate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Op    string `json:"op"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	inst.mu.Lock()
	switch req.Op {
	case "include":
		delete(inst.includes, req.Value) // normalize: re-add clean
		inst.includes[req.Value] = true
		inst.excludes = removeString(inst.excludes, req.Value)
	case "exclude":
		inst.excludes = addUnique(inst.excludes, req.Value)
		delete(inst.includes, req.Value)
	case "exclude-glob":
		inst.excludes = addUnique(inst.excludes, req.Value)
	case "remove-include":
		delete(inst.includes, req.Value)
	case "remove-exclude":
		inst.excludes = removeString(inst.excludes, req.Value)
	}
	inst.mu.Unlock()
	inst.writeState(w)
}

func (inst *pathPicker) handleSave(w http.ResponseWriter, r *http.Request) {
	inst.mu.Lock()
	paths := sortedKeys(inst.includes)
	excludes := append([]string(nil), inst.excludes...)
	inst.mu.Unlock()

	o := sysutil.Opts{Stdout: os.Stdout, DryRun: inst.dryRun}
	ctx := r.Context()
	pathsBody := backupPathsContent(paths)
	excludesBody := excludesContent(excludes)

	if inst.dryRun {
		fmt.Printf("\n[dry-run] would write %s:\n%s\n[dry-run] would write %s:\n%s\n",
			component.ResticPathsFile, pathsBody, component.ResticExcludesFile, excludesBody)
	} else {
		if err := writeRootFile(ctx, o, component.ResticPathsFile, pathsBody); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if err := writeRootFile(ctx, o, component.ResticExcludesFile, excludesBody); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true, "includes": len(paths), "excludes": len(excludes)})
	inst.doneOnce.Do(func() { close(inst.done) })
}

func (inst *pathPicker) writeState(w http.ResponseWriter) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	writeJSON(w, map[string]any{
		"home":     inst.home,
		"roots":    []string{inst.home, "/"},
		"includes": sortedKeys(inst.includes),
		"excludes": inst.excludes,
	})
}

// --- filesystem listing ---

type dirEntry struct {
	name     string
	path     string
	isDir    bool
	size     int64
	created  int64 // birth time (Unix seconds), mtime fallback
	readable bool
}

// listDir returns the entries of dir, directories first then names, with each
// dir's readability (so the UI can lock the ones we can't descend into).
func listDir(dir string) ([]dirEntry, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	entries := make([]dirEntry, 0, len(names))
	for _, name := range names {
		full := filepath.Join(dir, name)
		fi, err := os.Lstat(full)
		if err != nil {
			continue
		}
		isDir := fi.IsDir()
		e := dirEntry{name: name, path: full, isDir: isDir, size: fi.Size(), created: createdUnix(full, fi)}
		if isDir {
			e.size = -1
			e.readable = isReadableDir(full)
		} else {
			e.readable = true
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].isDir != entries[j].isDir {
			return entries[i].isDir // dirs first
		}
		return strings.ToLower(entries[i].name) < strings.ToLower(entries[j].name)
	})
	return entries, nil
}

func isReadableDir(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// entryState reports how a path is treated by the current lists, for tinting:
// "excluded" beats everything, then an explicit "included", then "inherited"
// (under an included ancestor), else "neutral". This is a preview; restic does
// the authoritative matching at backup time.
func entryState(path string, includes map[string]bool, excludes []string) string {
	for _, pat := range excludes {
		if excludeMatches(pat, path) {
			return "excluded"
		}
	}
	if includes[path] {
		return "included"
	}
	for anc := filepath.Dir(path); anc != "/" && anc != "."; anc = filepath.Dir(anc) {
		if includes[anc] {
			return "inherited"
		}
	}
	if includes["/"] {
		return "inherited"
	}
	return "neutral"
}

// excludeMatches is an approximate preview of restic's exclude matching:
// absolute paths match the path or its descendants; "*.ext" and other globs
// match the basename; "**/x" matches any segment equal to x.
func excludeMatches(pattern, path string) bool {
	base := filepath.Base(path)
	switch {
	case strings.HasPrefix(pattern, "/"):
		return path == pattern || strings.HasPrefix(path, strings.TrimRight(pattern, "/")+"/")
	case strings.HasPrefix(pattern, "**/"):
		tail := strings.TrimPrefix(pattern, "**/")
		if base == tail {
			return true
		}
		return strings.Contains(path, "/"+tail+"/") || strings.HasSuffix(path, "/"+tail)
	default:
		if ok, _ := filepath.Match(pattern, base); ok {
			return true
		}
		return false
	}
}

// --- disk usage ---

// duBytes returns a path's size in bytes via `du -sk`, retrying through
// non-interactive sudo for root-only trees. Returns -1 when it can't be sized.
func duBytes(ctx context.Context, path string) int64 {
	for _, useSudo := range []bool{false, true} {
		c := exec.CommandContext(ctx, "du", "-sk", path)
		if useSudo {
			c = sysutil.SudoNoPrompt(ctx, "du", "-sk", path)
		}
		out, err := c.Output()
		if err != nil {
			continue
		}
		fields := strings.Fields(string(out))
		if len(fields) == 0 {
			continue
		}
		if kb, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
			return kb * 1024
		}
	}
	return -1
}

// --- writing / misc helpers ---

func writeRootFile(ctx context.Context, o sysutil.Opts, path, content string) error {
	if err := sysutil.SudoWriteFile(ctx, o, content, path); err != nil {
		return err
	}
	// 0600 to match ResticSeedFile — everything under /etc/restic stays
	// root-only (the dir is 0700 anyway; consistency beats a stray loose file).
	return sysutil.SudoRun(ctx, o, "chmod", "0600", path)
}

func excludesContent(excludes []string) string {
	var b strings.Builder
	b.WriteString("# restic exclude patterns — one per line, '#' for comments.\n")
	b.WriteString("# Absolute paths, or globs like *.iso and **/node_modules.\n")
	for _, e := range excludes {
		b.WriteString(e)
		b.WriteString("\n")
	}
	return b.String()
}

func openBrowser(url string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{url}
	default:
		name, args = "xdg-open", []string{url}
	}
	if _, err := exec.LookPath(name); err != nil {
		return err
	}
	return exec.Command(name, args...).Start()
}

// isLoopbackOrigin reports whether an Origin header names a loopback host.
// The hostname must match exactly — a substring check would wave through
// "localhost.attacker.com".
func isLoopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func portOf(ln net.Listener) string {
	if a, ok := ln.Addr().(*net.TCPAddr); ok {
		return strconv.Itoa(a.Port)
	}
	return "PORT"
}

func hostLabel() string {
	h, _ := os.Hostname()
	if h == "" {
		return "user@host"
	}
	return h
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func copyMap(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func addUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func removeString(list []string, v string) []string {
	out := list[:0:0]
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
