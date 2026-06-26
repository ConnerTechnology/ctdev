package cleanup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// removeAll deletes each path entirely (after ~ and glob expansion). For
// user-owned paths only; respects DryRun.
func removeAll(o sysutil.Opts, paths ...string) error {
	for _, p := range expandAll(paths) {
		if o.DryRun {
			fmt.Fprintf(o.Stdout, "  [dry-run] rm -rf %s\n", p)
			continue
		}
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return nil
}

// emptyDir deletes the contents of each directory but keeps the directory
// itself (so re-creating it isn't an app's problem). User-owned; respects DryRun.
func emptyDir(o sysutil.Opts, dirs ...string) error {
	for _, dir := range expandAll(dirs) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if o.DryRun {
				fmt.Fprintf(o.Stdout, "  [dry-run] rm -rf %s\n", p)
				continue
			}
			if err := os.RemoveAll(p); err != nil {
				return err
			}
		}
	}
	return nil
}

// sudoEmptyDir deletes the contents of a root-owned directory via
// `sudo find <dir> -mindepth 1 -delete`, keeping the directory.
func sudoEmptyDir(ctx context.Context, o sysutil.Opts, dir string) error {
	if !pathExists(dir) {
		return nil
	}
	return sysutil.SudoRun(ctx, o, "find", dir, "-mindepth", "1", "-delete")
}

// sudoRemoveGlob removes root-owned paths matching the given globs.
func sudoRemoveGlob(ctx context.Context, o sysutil.Opts, globs ...string) error {
	matches := expandAll(globs)
	if len(matches) == 0 {
		return nil
	}
	return sysutil.SudoRun(ctx, o, "rm", append([]string{"-f"}, matches...)...)
}

// countLines counts the non-empty lines in s.
func countLines(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}
