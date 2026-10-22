package backup

import (
	"fmt"
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

// within reports whether p is dir or below it.
func within(p, dir string) bool {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// notInside refuses to back up, or configure, a folder holding d's repository.
func (d *Destination) notInside(root string) error {
	if isLocal(d.Address) && within(d.Address, root) {
		return fmt.Errorf("the repository for %s is inside %s, which would back it up into itself", d.Name, DisplayPath(d.m.Home, root))
	}
	return nil
}
