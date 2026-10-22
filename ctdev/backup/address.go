package backup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// backendPrefixes are the restic repository schemes; any other address is
// a local path.
var backendPrefixes = []string{"b2:", "s3:", "sftp:", "rest:", "azure:", "gs:", "swift:", "rclone:"}

func isLocal(address string) bool {
	for _, p := range backendPrefixes {
		if strings.HasPrefix(address, p) {
			return false
		}
	}
	return true
}

// CheckAddress returns a destination address ready to save. A local path
// has ~ expanded and is made absolute: restic expands neither, and it runs
// backups from inside the folder, so "~/Backups" would otherwise become a
// repository inside the folder being backed up. A local repository may not
// be inside folder (the one being set up, if any) or inside any backed-up
// folder, which would back the repository up into itself.
func (m *Machine) CheckAddress(address, folder string) (string, error) {
	if !isLocal(address) {
		return address, nil
	}
	p := strings.TrimSpace(address)
	if p == "" {
		return "", fmt.Errorf("the path can't be empty")
	}
	if p == "~" {
		p = m.Home
	} else if rest, ok := strings.CutPrefix(p, "~/"); ok {
		p = filepath.Join(m.Home, rest)
	}
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if folder != "" && within(p, folder) {
		return "", fmt.Errorf("%s is inside %s, which would back the repository up into itself", DisplayPath(m.Home, p), DisplayPath(m.Home, folder))
	}
	root, err := nearestConfig(p)
	if err != nil {
		return "", err
	}
	if root != "" {
		return "", fmt.Errorf("%s is inside %s, a backed-up folder, which would back the repository up into itself", DisplayPath(m.Home, p), DisplayPath(m.Home, root))
	}
	return p, nil
}

// CheckNewAddress is CheckAddress for a destination about to be created. A
// local path must be new, an empty folder, or a restic repository already
// (which creating then declines, pointing at connecting instead): restic
// would otherwise mix its files into a folder of someone's documents.
func (m *Machine) CheckNewAddress(address, folder string) (string, error) {
	p, err := m.CheckAddress(address, folder)
	if err != nil || !isLocal(p) {
		return p, err
	}
	fi, err := os.Stat(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return p, nil
	case err != nil:
		return "", err
	case !fi.IsDir():
		return "", fmt.Errorf("%s is a file, not a folder", DisplayPath(m.Home, p))
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return p, nil
	}
	if looksLikeRepository(p) {
		return p, nil
	}
	return "", fmt.Errorf("%s already holds files and isn't a restic repository: choose a new or empty folder", DisplayPath(m.Home, p))
}

// CheckNewAddressFromAll is CheckNewAddress for a destination created under
// `ctdev backup --all`, which works from any directory: a relative local path
// would quietly resolve against wherever that is, so it's refused. A path
// named in full may be anywhere CheckNewAddress allows, the current
// directory included: `~/Backups` from ~ is a fine destination.
func (m *Machine) CheckNewAddressFromAll(address string) (string, error) {
	p := strings.TrimSpace(address)
	if isLocal(p) && p != "~" && !strings.HasPrefix(p, "~/") && !filepath.IsAbs(p) {
		return "", fmt.Errorf("%q is relative to the current directory: give an absolute path or one starting with ~/", p)
	}
	return m.CheckNewAddress(p, "")
}

// looksLikeRepository reports whether dir has restic's layout: a config file
// beside keys/ and data/. A lone file named config is just a file.
func looksLikeRepository(dir string) bool {
	if fi, err := os.Stat(filepath.Join(dir, "config")); err != nil || !fi.Mode().IsRegular() {
		return false
	}
	for _, d := range []string{"keys", "data"} {
		if fi, err := os.Stat(filepath.Join(dir, d)); err != nil || !fi.IsDir() {
			return false
		}
	}
	return true
}

// within reports whether p is dir or below it.
// Both sides have their symlinks resolved first, so a path that reaches dir
// through a link still counts.
func within(p, dir string) bool {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(resolve(dir), resolve(p))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolve evaluates the symlinks in the longest part of p that exists, and
// keeps the rest, which may not exist yet, as it is.
func resolve(p string) string {
	rest := ""
	for q := filepath.Clean(p); ; q = filepath.Dir(q) {
		if r, err := filepath.EvalSymlinks(q); err == nil {
			return filepath.Join(r, rest)
		}
		if filepath.Dir(q) == q {
			return p
		}
		rest = filepath.Join(filepath.Base(q), rest)
	}
}

// notInside refuses to back up, or configure, a folder holding d's repository.
func (d *Destination) notInside(root string) error {
	if isLocal(d.Address) && within(d.Address, root) {
		return fmt.Errorf("the repository for %s is inside %s, which would back it up into itself", d.Name, DisplayPath(d.m.Home, root))
	}
	return nil
}
