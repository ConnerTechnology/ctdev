package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/state"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// leftoverProfilePaths names what machine profiles left on a machine before
// they were removed: the local profiles directory (always under ~/.config,
// whatever XDG_CONFIG_HOME says, because that is where profiles read it from)
// and the profile the old apply command recorded.
//
// A path whose location can't be worked out is left out: with no home
// directory there is no profiles directory to find, and unless XDG_STATE_HOME
// is set, state.StateDir would panic.
func leftoverProfilePaths() []string {
	var paths []string
	home, err := os.UserHomeDir()
	if err == nil {
		paths = append(paths, filepath.Join(home, ".config", "ctdev", "profiles"))
	}
	if err == nil || os.Getenv("XDG_STATE_HOME") != "" {
		paths = append(paths, filepath.Join(state.StateDir(), "applied-profile"))
	}
	return paths
}

// removeLeftoverProfiles deletes whichever of paths exist and prints each one
// it deleted. With dryRun (--dry-run or --check) it only reports what it would
// delete. A path that isn't there is skipped silently, so a machine that never
// used profiles hears nothing. A path that fails doesn't stop the rest; every
// failure comes back joined in one error.
func removeLeftoverProfiles(w io.Writer, paths []string, dryRun bool) error {
	var errs []error
	for _, p := range paths {
		if _, err := os.Lstat(p); err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, fmt.Errorf("check %s: %w", p, err))
			}
			continue
		}
		done := "would delete"
		if !dryRun {
			if err := os.RemoveAll(p); err != nil {
				errs = append(errs, fmt.Errorf("delete %s: %w", p, err))
				continue
			}
			done = "deleted"
		}
		fmt.Fprintln(w, styles.Dimmed.Render("Machine profiles are gone; "+done+" "+p))
	}
	return errors.Join(errs...)
}
