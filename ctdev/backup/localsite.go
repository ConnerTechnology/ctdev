package backup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// siteState is what was at a local destination's path before creation, which
// decides what a failed creation may remove.
type siteState int

const (
	siteUnknown  siteState = iota // couldn't tell, or a symlink: never removed
	siteMissing                   // nothing there: restic made it all
	siteEmptyDir                  // an empty directory: restic made what's inside
	siteFull                      // a directory with files in it
)

// resticLayout is what restic init puts in a repository folder.
var resticLayout = []string{"config", "data", "index", "keys", "locks", "snapshots"}

type localSite struct {
	path  string
	state siteState
	// madeParents are the path's ancestors that didn't exist either, nearest
	// first: restic makes them, so a cleanup removes them once empty.
	madeParents []string
}

// inspectLocal looks at a local path without following a symlink. Only a
// definite "doesn't exist" counts as missing; any other error is unknown.
func inspectLocal(path string) localSite {
	s := localSite{path: path}
	fi, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		s.state = siteMissing
		for p := filepath.Dir(path); p != filepath.Dir(p); p = filepath.Dir(p) {
			if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
				break
			}
			s.madeParents = append(s.madeParents, p)
		}
	case err != nil, fi.Mode()&fs.ModeSymlink != 0, !fi.IsDir():
		s.state = siteUnknown
	default:
		entries, err := os.ReadDir(path)
		switch {
		case err != nil:
			s.state = siteUnknown
		case len(entries) == 0:
			s.state = siteEmptyDir
		default:
			s.state = siteFull
		}
	}
	return s
}

// cleanup removes what a failed creation left at the site, and reports what
// it did as part of err.
func (s localSite) cleanup(err error) error {
	// Look again: the path must still be a real directory, not a symlink.
	if fi, lerr := os.Lstat(s.path); lerr == nil && (fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir()) {
		return fmt.Errorf("%w; %s isn't a plain folder now, so the half-created repository there was left: delete it before trying again", err, s.path)
	}
	switch s.state {
	case siteMissing, siteEmptyDir:
		// Only restic's own layout goes: anything else that appeared in the
		// folder meanwhile isn't ctdev's to delete.
		for _, name := range resticLayout {
			if rerr := os.RemoveAll(filepath.Join(s.path, name)); rerr != nil {
				return fmt.Errorf("%w; removing the half-created repository at %s also failed: %v", err, s.path, rerr)
			}
		}
		if s.state == siteMissing {
			// The folder and the parents made for it go too, while empty;
			// os.Remove takes only an empty directory.
			for _, p := range append([]string{s.path}, s.madeParents...) {
				if fi, lerr := os.Lstat(p); lerr != nil || !fi.IsDir() || os.Remove(p) != nil {
					break
				}
			}
		}
	default:
		return fmt.Errorf("%w; ctdev couldn't tell what was at %s before, so the half-created repository there was left: delete restic's config, data, index, keys, locks and snapshots there before trying again", err, s.path)
	}
	return fmt.Errorf("%w (the half-created repository at %s was removed)", err, s.path)
}
