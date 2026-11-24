package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// The SSH key code `configure ssh` and `configure git` share (CTD-177). The
// tools it runs go through package vars so tests can record them.
var (
	// runTerminalTool runs a tool that talks to the user itself: ssh-keygen
	// asks for the passphrase, ssh-copy-id for the server's password.
	runTerminalTool = func(ctx context.Context, name string, args ...string) error {
		c := exec.CommandContext(ctx, name, args...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		return c.Run()
	}
	// runOutputTool runs a tool and returns what it printed.
	runOutputTool = func(ctx context.Context, name string, args ...string) (string, error) {
		out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	// runInputTool feeds input to a tool and discards its output: wl-copy and
	// xclip leave a process behind to serve the clipboard, and waiting on its
	// output pipe would wait until something else is copied.
	runInputTool = func(ctx context.Context, input, name string, args ...string) error {
		c := exec.CommandContext(ctx, name, args...)
		c.Stdin = strings.NewReader(input)
		return c.Run()
	}
	hasTool = func(name string) bool {
		_, err := exec.LookPath(name)
		return err == nil
	}
	// terminalOut is where the OSC 52 clipboard sequence goes.
	terminalOut io.Writer = os.Stdout
	keyGOOS               = runtime.GOOS
)

// keyType is a kind of key ssh-keygen can make.
type keyType struct {
	name string // ssh-keygen -t
	note string
	bits string // ssh-keygen -b, empty for its default
}

var keyTypes = []keyType{
	{name: "ed25519", note: "recommended"},
	{name: "ed25519-sk", note: "on a hardware security key"},
	{name: "ecdsa"},
	{name: "rsa", note: "4096 bits", bits: "4096"},
}

// fileName is ssh-keygen's own default name for the type: id_ed25519_sk.
func (t keyType) fileName() string {
	return "id_" + strings.ReplaceAll(t.name, "-", "_")
}

// newKey is a key about to be created. path is the private key's; ssh-keygen
// writes the public key beside it as path.pub.
type newKey struct {
	typ     keyType
	path    string
	comment string
}

func (k newKey) keygenArgs() []string {
	args := []string{"-t", k.typ.name}
	if k.typ.bits != "" {
		args = append(args, "-b", k.typ.bits)
	}
	return append(args, "-f", k.path, "-C", k.comment)
}

// keyAction is something to do to the keys picked under Your keys.
type keyAction int

const (
	copyToServer keyAction = iota + 1
	uploadToGitHub
	copyToClipboard
	deleteKeys
)

// keyActionsFor lists what can be done to n keys. The clipboard holds one
// key, so copying to it is offered only for one.
func keyActionsFor(n int) []keyAction {
	if n == 1 {
		return []keyAction{copyToServer, uploadToGitHub, copyToClipboard, deleteKeys}
	}
	return []keyAction{copyToServer, uploadToGitHub, deleteKeys}
}

func (a keyAction) label(n int) string {
	one := n == 1
	switch a {
	case copyToServer:
		if one {
			return "Copy it to a server"
		}
		return "Copy each one to a server"
	case uploadToGitHub:
		if one {
			return "Upload it to GitHub"
		}
		return "Upload each one to GitHub"
	case copyToClipboard:
		return "Copy it to the clipboard"
	case deleteKeys:
		if one {
			return "Delete it"
		}
		return "Delete them"
	}
	return ""
}

// githubKeyUse is what a key uploaded to GitHub is for. GitHub keeps an
// authentication key and a signing key as two separate entries.
type githubKeyUse int

const (
	githubAuthentication githubKeyUse = iota + 1
	githubSigning
	githubBoth
)

// githubKey is one entry on GitHub's SSH keys page.
type githubKey struct {
	kind  string // authentication or signing
	title string
}

func githubKeysFor(title string, use githubKeyUse) []githubKey {
	var keys []githubKey
	if use != githubSigning {
		keys = append(keys, githubKey{kind: "authentication", title: title})
	}
	if use != githubAuthentication {
		keys = append(keys, githubKey{kind: "signing", title: title + "-signing"})
	}
	return keys
}

// ghArgs adds the public key pub as k. gh's default type is authentication.
func (k githubKey) ghArgs(pub string) []string {
	args := []string{"ssh-key", "add", pub, "--title", k.title}
	if k.kind == "signing" {
		args = append(args, "--type", "signing")
	}
	return args
}

func sshCopyIDArgs(pub, target, port string) []string {
	args := []string{"-i", pub}
	if port != "" {
		args = append(args, "-p", port)
	}
	return append(args, target)
}

// clipboardTool is the command that copies to the clipboard. The zero value
// means none fits, and the copy goes to the terminal as OSC 52.
type clipboardTool struct {
	name string
	args []string
}

// clipboardFor picks the clipboard for this session. In an SSH session with no
// display forwarded, the clipboard that matters is the one on the computer
// you're typing at, which only the terminal itself can reach (OSC 52). On
// macOS pbcopy would write the remote Mac's own pasteboard, so an SSH session
// there goes to OSC 52 too.
func clipboardFor(goos string, getenv func(string) string, has func(string) bool) clipboardTool {
	remote := getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != ""
	switch {
	case goos == "darwin":
		if !remote && has("pbcopy") {
			return clipboardTool{name: "pbcopy"}
		}
	case getenv("WAYLAND_DISPLAY") != "" && has("wl-copy"):
		return clipboardTool{name: "wl-copy"}
	case getenv("DISPLAY") != "" && has("xclip"):
		return clipboardTool{name: "xclip", args: []string{"-selection", "clipboard"}}
	}
	return clipboardTool{}
}

// osc52 is the escape sequence that asks the terminal to put text on its
// clipboard.
func osc52(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a"
}

func sshDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".ssh"), nil
}

// tildePath shows a path under the home directory as ~/….
func tildePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if rest, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return p
}

// keyPathTaken reports whether either half of a key at path already exists.
func keyPathTaken(path string) bool {
	for _, p := range []string{path, path + ".pub"} {
		if _, err := os.Lstat(p); err == nil {
			return true
		}
	}
	return false
}

// purposeSuffix turns "Work laptop" into "work_laptop" for a key file name.
func purposeSuffix(purpose string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(purpose)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case !strings.HasSuffix(b.String(), "_"):
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

// resolveKeyPath reads a file-name answer: a bare name lives in dir, ~/ is
// the home directory.
func resolveKeyPath(dir, answer string) string {
	switch {
	case answer == "":
		return ""
	case strings.HasPrefix(answer, "~/"):
		return filepath.Join(filepath.Dir(dir), answer[2:])
	case filepath.IsAbs(answer):
		return answer
	}
	return filepath.Join(dir, answer)
}

// promptKeyPath asks where a new key of type t goes in dir: id_<type>, or
// id_<type>_<purpose> when that's taken. It never returns a path either half
// of an existing key uses.
func promptKeyPath(ctx context.Context, dir string, t keyType) (string, error) {
	def := filepath.Join(dir, t.fileName())
	if keyPathTaken(def) {
		fmt.Println(styles.Dimmed.Render("  " + tildePath(def) + " already exists."))
		purpose, err := promptRequiredCtx(ctx, "What is the new key for? (e.g. github, work)", "")
		if err != nil {
			return "", err
		}
		def = filepath.Join(dir, t.fileName()+"_"+purposeSuffix(purpose))
	}
	for {
		shown := ""
		if !keyPathTaken(def) {
			shown = tildePath(def)
		}
		answer, err := promptWithDefaultCtx(ctx, "File name", shown)
		if err != nil {
			return "", err
		}
		path := resolveKeyPath(dir, answer)
		switch {
		case path == "":
			fmt.Println(styles.Warning.Render("  required — please enter a file name"))
		case keyPathTaken(path):
			fmt.Println(styles.Warning.Render("  " + tildePath(path) + " already exists, and ctdev never overwrites a key. Pick another name."))
		default:
			return path, nil
		}
	}
}

// defaultKeyComment is user@hostname.
func defaultKeyComment() string {
	name := os.Getenv("USER")
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	host, _ := os.Hostname()
	return name + "@" + host
}

func printDryRun(name string, args ...string) {
	fmt.Printf("  [dry-run] would run: %s %s\n", name, strings.Join(args, " "))
}

// createKey runs ssh-keygen, which asks for the passphrase itself so ctdev
// never holds it, then on macOS adds the key to the Keychain. ok is false
// when no key was made (--dry-run).
func createKey(ctx context.Context, k newKey) (pair sysutil.SSHKeyPair, ok bool, err error) {
	addArgs := []string{"--apple-use-keychain", k.path}
	if flagDryRun {
		printDryRun("ssh-keygen", k.keygenArgs()...)
		if keyGOOS == "darwin" {
			printDryRun("ssh-add", addArgs...)
		}
		return pair, false, nil
	}
	if keyPathTaken(k.path) {
		return pair, false, fmt.Errorf("%s already exists; ctdev never overwrites a key", k.path)
	}
	if err := os.MkdirAll(filepath.Dir(k.path), 0o700); err != nil {
		return pair, false, err
	}

	fmt.Printf("  Generating a new %s key...\n\n", k.typ.name)
	if err := runTerminalTool(ctx, "ssh-keygen", k.keygenArgs()...); err != nil {
		return pair, false, fmt.Errorf("ssh-keygen failed: %w", err)
	}
	pair = sysutil.SSHKeyPair{Private: k.path, Public: k.path + ".pub", Comment: k.comment}
	data, err := os.ReadFile(pair.Public)
	if err != nil {
		return pair, false, fmt.Errorf("ssh-keygen made no public key: %w", err)
	}
	if fields := strings.Fields(string(data)); len(fields) > 0 {
		pair.Type = fields[0]
	}
	fmt.Printf("\n  %s\n\n", styles.Dimmed.Render(strings.TrimSpace(string(data))))

	if keyGOOS == "darwin" {
		if err := runTerminalTool(ctx, "ssh-add", addArgs...); err != nil {
			fmt.Println(styles.Warning.Render("  Couldn't add the key to the Keychain: " + err.Error()))
		} else {
			fmt.Println(styles.Success.Render("  Added the key to the Keychain"))
		}
	}
	return pair, true, nil
}

// githubReady reports whether gh is installed and signed in.
func githubReady(ctx context.Context) bool {
	_, err := runOutputTool(ctx, "gh", "auth", "status")
	return err == nil
}

// uploadKeyToGitHub adds the public key pub to GitHub with gh for use.
func uploadKeyToGitHub(ctx context.Context, pub, title string, use githubKeyUse) {
	failed := false
	for _, k := range githubKeysFor(title, use) {
		if flagDryRun {
			printDryRun("gh", k.ghArgs(pub)...)
			continue
		}
		if out, err := runOutputTool(ctx, "gh", k.ghArgs(pub)...); err != nil {
			fmt.Println(styles.Warning.Render(fmt.Sprintf("  Failed to add the %s key to GitHub: %s", k.kind, out)))
			failed = true
		} else {
			fmt.Println(styles.Success.Render(fmt.Sprintf("  Added SSH %s key to GitHub as %q", k.kind, k.title)))
		}
	}
	if failed {
		printManualGitHubInstructions(title, use)
	}
}

func printManualGitHubInstructions(title string, use githubKeyUse) {
	keyType := map[githubKeyUse]string{
		githubAuthentication: "Authentication Key",
		githubSigning:        "Signing Key",
		githubBoth:           "Authentication Key, then add it again as Signing Key",
	}[use]
	fmt.Println()
	fmt.Println("  Add this key to GitHub:")
	fmt.Println("    1. Go to https://github.com/settings/ssh/new")
	fmt.Printf("    2. Title: %s\n", title)
	fmt.Printf("    3. Key type: %s\n", keyType)
	fmt.Println("    4. Paste your public key (shown above)")
	fmt.Println()
}

// parsePicks reads 1-based numbers out of n, like "1 3" or "1,3". ok is
// false when a number is out of range or not a number.
func parsePicks(input string, n int) (picks []int, ok bool) {
	for _, f := range strings.FieldsFunc(input, func(r rune) bool { return r == ' ' || r == ',' }) {
		i, err := strconv.Atoi(f)
		if err != nil || i < 1 || i > n {
			return nil, false
		}
		picks = append(picks, i)
	}
	return picks, true
}
