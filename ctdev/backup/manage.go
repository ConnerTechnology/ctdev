package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// StopBackingUp makes the folder at root no longer backed up: it deletes the
// folder's config file, then takes it off this computer's schedule list.
// Its backups are kept, still tagged with the folder's ID, which is never
// rewritten: the folder, now unconfigured, lists only ID-less one-offs, and
// `ctdev backup --all` finds the kept backups.
func (m *Machine) StopBackingUp(root string) error {
	cfgPath := filepath.Join(root, ConfigFileName)
	if m.Opts.DryRun {
		fmt.Fprintf(m.out(), "[dry-run] remove %s\n", cfgPath)
		fmt.Fprintf(m.out(), "[dry-run] remove %s from %s\n", root, m.foldersFile())
		return nil
	}
	// The config file first: a folder left on the list without one is
	// reported as missing, where one left with a config and off the list
	// would still look backed up.
	if err := os.Remove(cfgPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return m.removeFolder(root)
}

// StopAndDelete is StopBackingUp that also forgets the folder's backups:
// exactly snaps, the folder's backups from every computer as listed by its ID
// for the confirmation. They go first, while the config file still holds the
// ID; if forgetting fails, the folder stays configured and scheduled.
func (m *Machine) StopAndDelete(ctx context.Context, d *Destination, root string, snaps []Snapshot) error {
	if err := d.DeleteBackups(ctx, snaps); err != nil {
		return err
	}
	if err := m.StopBackingUp(root); err != nil {
		return fmt.Errorf("the folder's %d backups were deleted, but stopping backing it up failed: %w", len(snaps), err)
	}
	return nil
}

// Delete forgets one backup. It doesn't prune: the space is freed at the
// destination's next weekly maintenance, which holds the exclusive lock.
func (d *Destination) Delete(ctx context.Context, snap Snapshot) error {
	return d.forget(ctx, snap.ID)
}

// deleteAll forgets every backup of a folder, as Snapshots finds them, and
// says how many it forgot. Like Delete, it doesn't prune.
func (d *Destination) deleteAll(ctx context.Context, ref Ref) (int, error) {
	snaps, err := d.Snapshots(ctx, ref)
	if err != nil {
		return 0, err
	}
	if err := d.DeleteBackups(ctx, snaps); err != nil {
		return 0, err
	}
	return len(snaps), nil
}

// DeleteBackups forgets exactly these backups, such as a listing just shown
// to someone confirming its count. Like Delete, it doesn't prune.
func (d *Destination) DeleteBackups(ctx context.Context, snaps []Snapshot) error {
	if len(snaps) == 0 {
		return nil // with no IDs, forget would act on its policy, not on nothing
	}
	ids := make([]string, len(snaps))
	for i, s := range snaps {
		ids[i] = s.ID
	}
	return d.forget(ctx, ids...)
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

// zipName is the name a backup of the folder at root downloads as.
func zipName(root string, snap Snapshot) string {
	return filepath.Base(root) + "-" + snap.Time.UTC().Format(zipTime) + ".zip"
}

// downloadDir is where a zip of the folder at root goes: beside the folder,
// or when that is inside a backed-up folder, beside the outermost one, so a
// download is never itself backed up (CTD-87).
func downloadDir(root string) (string, error) {
	return outsideBackedUp(filepath.Dir(root))
}

// outsideBackedUp is dir, or when dir is inside a backed-up folder, the
// parent of the outermost one around it.
func outsideBackedUp(dir string) (string, error) {
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
// (see downloadDir) and returns its path. The zip holds the folder's contents
// at their relative paths. restic writes it into a temporary file that is
// linked (or, without hard links, copied) into place once complete, so a zip
// under the final name is whole,
// and a file that took the name meanwhile is never overwritten.
func (m *Machine) Download(ctx context.Context, d *Destination, root string, snap Snapshot) (string, error) {
	dir, err := downloadDir(root)
	if err != nil {
		return "", err
	}
	return m.downloadInto(ctx, d, filepath.Join(dir, zipName(root, snap)), snap)
}

// DownloadFromAll is Download for a backup found under `ctdev backup --all`,
// whose folder may be on another computer. The zip goes beside where the
// folder would be on this computer, when that parent exists here; otherwise
// into cwd, or beside the backed-up folder cwd is in. Like Download, it
// never lands inside a backed-up folder (CTD-87).
func (m *Machine) DownloadFromAll(ctx context.Context, d *Destination, cwd string, snap Snapshot) (string, error) {
	if snap.Path == "" {
		return "", errors.New("the backup doesn't record its folder's path")
	}
	original := m.localPath(snap.Path)
	start, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(original) {
		original = filepath.Base(original) // only its name is any use here
	} else if fi, err := os.Stat(filepath.Dir(original)); err == nil && fi.IsDir() {
		start = filepath.Dir(original)
	}
	dir, err := outsideBackedUp(start)
	if err != nil {
		return "", err
	}
	return m.downloadInto(ctx, d, filepath.Join(dir, zipName(original, snap)), snap)
}

// localPath is a recorded display path (~/…) as a path on this computer.
func (m *Machine) localPath(display string) string {
	if display == "~" {
		return m.Home
	}
	if rest, ok := strings.CutPrefix(display, "~/"); ok {
		return filepath.Join(m.Home, filepath.FromSlash(rest))
	}
	return filepath.FromSlash(display)
}

// downloadInto writes a backup as a zip at path; see Download.
func (m *Machine) downloadInto(ctx context.Context, d *Destination, path string, snap Snapshot) (string, error) {
	dir := filepath.Dir(path)
	exists := func() error {
		return fmt.Errorf("%s already exists: move it away to download this backup again", path)
	}
	if _, err := os.Lstat(path); err == nil {
		return "", exists()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	tmpPattern := "." + filepath.Base(path) + ".partial-*"
	if m.Opts.DryRun {
		d.dryRun(d.asComputer("", "dump", "--retry-lock", retryLock, "--archive", "zip", "--target", filepath.Join(dir, tmpPattern), snap.ID, "/"))
		return path, nil
	}
	tmp, err := os.CreateTemp(dir, tmpPattern)
	if err != nil {
		return "", err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	// "/" is the backup's root, which is the folder: backups are made from
	// inside it with relative paths.
	if _, err := d.restic(ctx, d.asComputer("", "dump", "--retry-lock", retryLock, "--archive", "zip", "--target", tmp.Name(), snap.ID, "/")); err != nil {
		return "", err
	}
	// A hard link, unlike a rename, fails when the name is taken.
	err = linkFile(tmp.Name(), path)
	if errors.Is(err, fs.ErrExist) {
		return "", exists()
	}
	if err != nil {
		// No hard links here, as on exFAT and some SMB shares: copy instead,
		// into a file created only if the name is free.
		if err := copyNew(tmp.Name(), path); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return "", exists()
			}
			return "", err
		}
	}
	return path, nil
}

// linkFile is os.Link; tests replace it to stand in for a filesystem without
// hard links.
var linkFile = os.Link

// copyNew copies src to dst, which it creates and must not exist. On an
// error it removes dst, which it made, so no half-written zip is left.
func copyNew(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
	}
	return err
}
