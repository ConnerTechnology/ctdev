// Package cleanup is ctdev's disk reclaimer — a CleanMyMac-style catalog of
// cleanup tasks for Linux (apt/snap/flatpak/journal/docker) and macOS
// (brew/Xcode/caches/trash/tmutil). Each Task can scan for reclaimable space
// without changing anything, and run to reclaim it. Tasks are tiered by Risk so
// the command can preselect only the safe ones and never auto-touch user data.
package cleanup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// Risk tiers a task by how safe it is to run unattended.
type Risk int

const (
	// Safe reclaims regenerable or orphaned data; preselected by default.
	Safe Risk = iota
	// OptIn is recoverable but affects app state (caches, trash); shown unchecked.
	OptIn
	// ReportOnly locates user data (device backups, mail) — surfaced, never deleted.
	ReportOnly
)

// Task is one cleanup action.
type Task struct {
	ID     string
	Name   string
	Group  string // category header (e.g. "Packages", "Caches", "Developer")
	Detail string // dimmed note (e.g. a path), shown after the name
	Risk   Risk

	// Scan measures reclaimable bytes without changing anything. A negative
	// byte count means "unknown" (omitted from the reclaimable total).
	Scan func(ctx context.Context) ScanResult
	// Run reclaims the space. Nil for ReportOnly tasks.
	Run func(ctx context.Context, o sysutil.Opts) error
}

// ScanResult is what a Task's scan found.
type ScanResult struct {
	Bytes int64  // reclaimable bytes; <0 = unknown
	Note  string // optional human note (e.g. "3 old revisions")
}

// Catalog returns the platform-appropriate task list, gated to the tools and
// paths actually present on this machine.
func Catalog(info platform.Info) []Task {
	switch info.OS {
	case platform.Linux:
		return linuxTasks(info)
	case platform.MacOS:
		return macTasks(info)
	default:
		return nil
	}
}

// ScanAll runs every task's scan concurrently and returns results keyed by ID.
func ScanAll(ctx context.Context, tasks []Task) map[string]ScanResult {
	results := make(map[string]ScanResult, len(tasks))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6) // bound concurrent du/shell-outs

	for _, t := range tasks {
		if t.Scan == nil {
			continue
		}
		t := t
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res := t.Scan(ctx)
			mu.Lock()
			results[t.ID] = res
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results
}

// --- scan helpers ---

// measurePaths returns the total disk usage of the paths that exist, best
// effort. Globs are expanded; root-only paths fall back to `sudo -n du`. Paths
// starting with ~ are expanded to the home directory.
func measurePaths(ctx context.Context, paths ...string) int64 {
	var total int64
	for _, p := range expandAll(paths) {
		if _, err := os.Lstat(p); err != nil {
			continue
		}
		if kb, ok := duKB(ctx, p, false); ok {
			total += kb * 1024
			continue
		}
		if kb, ok := duKB(ctx, p, true); ok {
			total += kb * 1024
		}
	}
	return total
}

// duKB returns the disk usage of a single path in KiB via `du -sk` (portable
// across GNU and BSD du), optionally through non-interactive sudo.
func duKB(ctx context.Context, path string, useSudo bool) (int64, bool) {
	var c *exec.Cmd
	if useSudo {
		c = exec.CommandContext(ctx, "sudo", "-n", "du", "-sk", path)
	} else {
		c = exec.CommandContext(ctx, "du", "-sk", path)
	}
	out, err := c.Output()
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0, false
	}
	kb, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, false
	}
	return kb, true
}

// expandAll expands ~ and globs across a set of paths into concrete paths.
func expandAll(paths []string) []string {
	var out []string
	for _, p := range paths {
		p = expandHome(p)
		if strings.ContainsAny(p, "*?[") {
			if matches, err := filepath.Glob(p); err == nil {
				out = append(out, matches...)
			}
			continue
		}
		out = append(out, p)
	}
	return out
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// homeDir returns the user's home directory.
func homeDir() (string, error) { return os.UserHomeDir() }

// captureOut runs a command and returns trimmed combined-ish stdout, ignoring a
// nonzero exit (many of these tools exit nonzero yet still print what we need).
func captureOut(ctx context.Context, name string, args ...string) string {
	out, _ := exec.CommandContext(ctx, name, args...).Output()
	return strings.TrimSpace(string(out))
}

// --- size parsing / formatting ---

var sizeUnits = map[string]int64{
	"B": 1, "K": 1 << 10, "KB": 1 << 10, "KIB": 1 << 10,
	"M": 1 << 20, "MB": 1 << 20, "MIB": 1 << 20,
	"G": 1 << 30, "GB": 1 << 30, "GIB": 1 << 30,
	"T": 1 << 40, "TB": 1 << 40, "TIB": 1 << 40,
}

// parseSize turns a human size like "312 MB", "1.4G", or "240.0M" into bytes.
// Returns -1 when it can't parse one.
func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1
	}
	// Split the numeric prefix from the unit suffix.
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == ',') {
		i++
	}
	numStr := strings.ReplaceAll(s[:i], ",", "")
	unit := strings.ToUpper(strings.TrimSpace(s[i:]))
	if unit == "" {
		unit = "B"
	}
	mult, ok := sizeUnits[unit]
	if !ok {
		return -1
	}
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return -1
	}
	return int64(num * float64(mult))
}

// Humanize renders a byte count as a short human string (e.g. "1.4 GB").
func Humanize(bytes int64) string {
	if bytes < 0 {
		return "?"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// parseAptFreed extracts the bytes from apt's "After this operation, X will be
// freed." line in an `apt-get -s` simulation. Returns 0 when nothing is freed.
func parseAptFreed(out string) int64 {
	for _, line := range strings.Split(out, "\n") {
		l := strings.TrimSpace(line)
		if strings.Contains(l, "will be freed") {
			// "After this operation, 312 MB disk space will be freed."
			if idx := strings.Index(l, ","); idx >= 0 {
				l = l[idx+1:]
			}
			l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "After this operation,"))
			l = strings.TrimSuffix(l, "disk space will be freed.")
			l = strings.ReplaceAll(l, "disk space will be freed", "")
			return parseSize(strings.TrimSpace(l))
		}
	}
	return 0
}

// parseJournalUsage extracts bytes from `journalctl --disk-usage`:
// "Archived and active journals take up 240.0M in the file system."
func parseJournalUsage(out string) int64 {
	out = strings.TrimSpace(out)
	marker := "take up "
	idx := strings.Index(out, marker)
	if idx < 0 {
		return -1
	}
	rest := out[idx+len(marker):]
	if end := strings.Index(rest, " in the file system"); end >= 0 {
		rest = rest[:end]
	}
	return parseSize(rest)
}

// parseBrewFreed extracts bytes from `brew cleanup -n`'s
// "This operation would free approximately 1.2GB of disk space." line.
func parseBrewFreed(out string) int64 {
	for _, line := range strings.Split(out, "\n") {
		l := strings.TrimSpace(line)
		if strings.Contains(l, "would free") {
			l = l[strings.Index(l, "would free")+len("would free"):]
			l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "approximately"))
			if end := strings.Index(l, " of disk space"); end >= 0 {
				l = l[:end]
			}
			return parseSize(strings.TrimSpace(l))
		}
	}
	return 0
}

// commandExists reports whether a command is on PATH (re-exported for tasks).
func commandExists(name string) bool { return sysutil.CommandExists(name) }

// pathExists reports whether a path (after ~ expansion) exists.
func pathExists(p string) bool {
	_, err := os.Lstat(expandHome(p))
	return err == nil
}
