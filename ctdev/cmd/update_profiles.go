package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/state"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// leftoverProfilePaths names what machine profiles left on a machine before
// they were removed: the local profiles directory (always under ~/.config,
// whatever XDG_CONFIG_HOME says, because that is where profiles read it from)
// and the profile `ctdev apply` recorded.
func leftoverProfilePaths() []string {
	var paths []string
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "ctdev", "profiles"))
	}
	return append(paths, filepath.Join(state.StateDir(), "applied-profile"))
}

// removeLeftoverProfiles deletes whichever of paths exist and prints each one
// it deleted. With dryRun (--dry-run or --check) it only reports what it would
// delete. A path that isn't there is skipped silently, so a machine that never
// used profiles hears nothing.
func removeLeftoverProfiles(w io.Writer, paths []string, dryRun bool) error {
	for _, p := range paths {
		if _, err := os.Lstat(p); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("check %s: %w", p, err)
		}
		if dryRun {
			fmt.Fprintln(w, styles.Dimmed.Render("Machine profiles are gone; would delete "+p))
			continue
		}
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("delete %s: %w", p, err)
		}
		fmt.Fprintln(w, styles.Dimmed.Render("Machine profiles are gone; deleted "+p))
	}
	return nil
}
