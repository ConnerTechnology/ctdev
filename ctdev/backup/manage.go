package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// StopBackingUp makes the folder at root no longer backed up: it deletes the
// folder's config file and takes it off this computer's schedule list. Its
// backups are kept, as a folder that isn't configured, found by its path and
// this computer's name; DeleteAll removes them.
func (m *Machine) StopBackingUp(root string) error {
	cfgPath := filepath.Join(root, ConfigFileName)
	if m.Opts.DryRun {
		fmt.Fprintf(m.out(), "[dry-run] remove %s\n", cfgPath)
		fmt.Fprintf(m.out(), "[dry-run] remove %s from %s\n", root, m.foldersFile())
		return nil
	}
	if err := m.removeFolder(root); err != nil {
		return err
	}
	if err := os.Remove(cfgPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// StopAndDelete is StopBackingUp that also forgets every backup with the
// folder's ID, from every computer. The backups go first, while the ID is
// still in the config file: once it's gone, the folder's backups can only be
// found by path and this computer's name. It returns how many it forgot.
func (m *Machine) StopAndDelete(ctx context.Context, d *Destination, root string, cfg Config) (int, error) {
	n, err := d.DeleteAll(ctx, Ref{FolderID: cfg.ID})
	if err != nil {
		return n, err
	}
	return n, m.StopBackingUp(root)
}

// Delete forgets one backup. It doesn't prune: the space is freed at the
// destination's next weekly maintenance, which holds the exclusive lock.
func (d *Destination) Delete(ctx context.Context, snap Snapshot) error {
	return d.forget(ctx, snap.ID)
}

// DeleteAll forgets every backup of a folder, as Snapshots finds them, and
// says how many it forgot. Like Delete, it doesn't prune.
func (d *Destination) DeleteAll(ctx context.Context, ref Ref) (int, error) {
	snaps, err := d.Snapshots(ctx, ref)
	if err != nil {
		return 0, err
	}
	if len(snaps) == 0 {
		return 0, nil
	}
	ids := make([]string, len(snaps))
	for i, s := range snaps {
		ids[i] = s.ID
	}
	if err := d.forget(ctx, ids...); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// ApplyRetention forgets the configured folder's backups that its keep rules
// no longer cover, leaving every other folder's alone. It runs after each
// successful backup of a configured folder, by hand or scheduled; one-offs of
// an unconfigured folder have no keep rules and are kept until deleted. Like
// Delete, it doesn't prune.
func (d *Destination) ApplyRetention(ctx context.Context, cfg Config) error {
	k := cfg.Keep
	if k == (Keep{}) {
		return nil // no rules: restic would keep everything anyway
	}
	// One group per folder, whichever computer or path made the backup, so
	// the rules count the folder's backups as a whole.
	args := []string{"forget", "--retry-lock", retryLock, "--tag", tagFolder + cfg.ID, "--group-by", ""}
	for _, rule := range []struct {
		flag string
		n    int
	}{{"--keep-hourly", k.Hourly}, {"--keep-daily", k.Daily}, {"--keep-weekly", k.Weekly}, {"--keep-monthly", k.Monthly}, {"--keep-yearly", k.Yearly}} {
		if rule.n > 0 {
			args = append(args, rule.flag, strconv.Itoa(rule.n))
		}
	}
	r := d.asComputer("", args...)
	if d.m.Opts.DryRun {
		d.dryRun(r)
		return nil
	}
	_, err := d.restic(ctx, r)
	return err
}

func (d *Destination) forget(ctx context.Context, ids ...string) error {
	r := d.asComputer("", append([]string{"forget", "--retry-lock", retryLock}, ids...)...)
	if d.m.Opts.DryRun {
		d.dryRun(r)
		return nil
	}
	_, err := d.restic(ctx, r)
	return err
}

// zipTime is a backup's time in a zip's name: UTC in ISO 8601's compact
// form, which has no colons, so the name is safe on macOS, Windows and SMB.
const zipTime = "20060102T150405Z"

// ZipName is the name a backup of the folder at root downloads as.
func ZipName(root string, snap Snapshot) string {
	return filepath.Base(root) + "-" + snap.Time.UTC().Format(zipTime) + ".zip"
}

// DownloadDir is where a zip of the folder at root goes: beside the folder,
// or when that is inside a backed-up folder, beside the outermost one, so a
// download is never itself backed up (CTD-87).
func DownloadDir(root string) (string, error) {
	dir := filepath.Dir(root)
	for {
		enclosing, err := nearestConfig(dir)
		if err != nil {
			return "", err
		}
		if enclosing == "" {
			return dir, nil
		}
		parent := filepath.Dir(enclosing)
		if parent == enclosing {
			return "", errors.New("every folder up to / is backed up, so there's nowhere outside them for the zip")
		}
		dir = parent
	}
}

// Download writes a backup of the folder at root as a zip beside the folder
// (see DownloadDir) and returns its path. The zip holds the folder's contents
// at their relative paths. restic writes it into a temporary file that is
// renamed into place once complete, so a zip under the final name is whole.
func (m *Machine) Download(ctx context.Context, d *Destination, root string, snap Snapshot) (string, error) {
	dir, err := DownloadDir(root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, ZipName(root, snap))
	if _, err := os.Lstat(path); err == nil {
		return "", fmt.Errorf("%s already exists: move it away to download this backup again", path)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if m.Opts.DryRun {
		d.dryRun(d.asComputer("", "dump", "--retry-lock", retryLock, "--archive", "zip", "--target", path+".partial", snap.ID, "/"))
		return path, nil
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".partial-*")
	if err != nil {
		return "", err
	}
	tmp.Close()
	done := false
	defer func() {
		if !done {
			os.Remove(tmp.Name())
		}
	}()
	// "/" is the backup's root, which is the folder: backups are made from
	// inside it with relative paths.
	if _, err := d.restic(ctx, d.asComputer("", "dump", "--retry-lock", retryLock, "--archive", "zip", "--target", tmp.Name(), snap.ID, "/")); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	done = true
	return path, nil
}
