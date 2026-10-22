package backup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Every backup is tagged with its folder's ID (none for a one-off), its path
// as shown on the computer that made it, and that computer's hostname.
const (
	tagFolder = "ctdev-folder:"
	tagPath   = "ctdev-path:"
	tagHost   = "ctdev-host:"
)

// tagValue escapes what restic's tag syntax gives meaning to: a comma
// separates tags in --tag, so a path holding one would split in two.
func tagValue(s string) string {
	return strings.NewReplacer("%", "%25", ",", "%2C").Replace(s)
}

func untagValue(s string) string {
	return strings.NewReplacer("%2C", ",", "%25", "%").Replace(s)
}

// Ref picks out one folder's backups: by its folder ID once configured, and
// by path and hostname for the one-offs of a folder that isn't.
type Ref struct {
	FolderID string
	Path     string // display form, ~/…
	Host     string
}

// Ref is the reference for the folder at root, configured or not.
func (m *Machine) Ref(root string, cfg *Config) Ref {
	r := Ref{Path: DisplayPath(m.Home, root), Host: m.Hostname}
	if cfg != nil {
		r.FolderID = cfg.ID
	}
	return r
}

// Snapshot is one backup.
type Snapshot struct {
	ID       string
	ShortID  string
	Time     time.Time
	Host     string
	FolderID string
	Path     string
	Files    int   // files in the backup
	Size     int64 // bytes in the backup
	Added    int64 // new data the backup added to the destination

	FilesNew     int           // files the backup found new since the one before
	FilesChanged int           // files changed since the one before
	Duration     time.Duration // how long the backup took

	Warning string
}

// Backup backs up the whole folder at root to d, as the user, from inside
// the folder so the backup holds relative paths. With a nil cfg it is a
// one-off: tagged with the path and hostname only. The result is recorded in
// this machine's state.
func (m *Machine) Backup(ctx context.Context, d *Destination, root string, cfg *Config) (Snapshot, error) {
	if root == "" {
		return Snapshot{}, errors.New("no folder to back up")
	}
	if err := d.notInside(root); err != nil {
		return Snapshot{}, err
	}
	ref := m.Ref(root, cfg)
	args := []string{"backup", ".", "--json", "--retry-lock", retryLock, "--host", m.Hostname,
		"--tag", tagPath + tagValue(ref.Path), "--tag", tagHost + tagValue(m.Hostname)}
	if ref.FolderID != "" {
		args = append(args, "--tag", tagFolder+ref.FolderID)
	}
	r := d.asComputer(root, args...)
	if m.Opts.DryRun {
		d.dryRun(r)
		return Snapshot{}, nil
	}
	started := m.now()
	out, err := d.restic(ctx, r)
	partial := errors.Is(err, errPartial)
	if partial {
		err = nil // the backup was made; Warning says what was skipped
	}
	var snap Snapshot
	if err == nil {
		snap, err = backupSummary(out)
	}
	snap.Host, snap.FolderID, snap.Path = m.Hostname, ref.FolderID, ref.Path
	if partial && err == nil {
		snap.Warning = errPartial.Error()
	}
	if serr := m.recordRun(root, started, err); serr != nil && err == nil {
		err = serr
	}
	return snap, err
}

// backupSummary reads the summary line restic backup --json ends with.
func backupSummary(out []byte) (Snapshot, error) {
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var s struct {
			Type       string    `json:"message_type"`
			SnapshotID string    `json:"snapshot_id"`
			Files      int       `json:"total_files_processed"`
			Bytes      int64     `json:"total_bytes_processed"`
			Added      int64     `json:"data_added"`
			End        time.Time `json:"backup_end"`
		}
		if json.Unmarshal(sc.Bytes(), &s) == nil && s.Type == "summary" {
			return Snapshot{ID: s.SnapshotID, ShortID: short(s.SnapshotID), Time: s.End, Files: s.Files, Size: s.Bytes, Added: s.Added}, nil
		}
	}
	return Snapshot{}, errors.New("restic backup finished without a summary")
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// Snapshots lists a folder's backups, oldest first. A Ref with no folder ID
// matches only backups with no folder ID: the one-offs of an unconfigured
// folder. A backup that carries an ID belongs to that folder wherever it has
// since moved, even though it has this path and host too (CTD-120).
func (d *Destination) Snapshots(ctx context.Context, ref Ref) ([]Snapshot, error) {
	args := []string{"snapshots", "--json", "--retry-lock", retryLock}
	if ref.FolderID != "" {
		args = append(args, "--tag", tagFolder+ref.FolderID)
	} else {
		args = append(args, "--tag", tagPath+tagValue(ref.Path), "--host", ref.Host)
	}
	all, err := d.snapshots(ctx, args...)
	if err != nil {
		return nil, err
	}
	snaps := all[:0]
	for _, s := range all {
		if ref.FolderID == "" && s.FolderID != "" {
			continue
		}
		snaps = append(snaps, s)
	}
	return snaps, nil
}

// snapshots runs restic snapshots with args and reads its list, oldest first.
func (d *Destination) snapshots(ctx context.Context, args ...string) ([]Snapshot, error) {
	out, err := d.restic(ctx, d.asComputer("", args...))
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID       string    `json:"id"`
		ShortID  string    `json:"short_id"`
		Time     time.Time `json:"time"`
		Hostname string    `json:"hostname"`
		Tags     []string  `json:"tags"`
		Summary  *struct {
			Start   time.Time `json:"backup_start"`
			End     time.Time `json:"backup_end"`
			New     int       `json:"files_new"`
			Changed int       `json:"files_changed"`
			Files   int       `json:"total_files_processed"`
			Bytes   int64     `json:"total_bytes_processed"`
			Added   int64     `json:"data_added"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("read restic's snapshot list: %w", err)
	}
	snaps := make([]Snapshot, 0, len(raw))
	for _, r := range raw {
		s := Snapshot{ID: r.ID, ShortID: r.ShortID, Time: r.Time, Host: r.Hostname}
		for _, t := range r.Tags {
			switch {
			case strings.HasPrefix(t, tagFolder):
				s.FolderID = strings.TrimPrefix(t, tagFolder)
			case strings.HasPrefix(t, tagPath):
				s.Path = untagValue(strings.TrimPrefix(t, tagPath))
			}
		}
		if sum := r.Summary; sum != nil {
			s.Files, s.Size, s.Added = sum.Files, sum.Bytes, sum.Added
			s.FilesNew, s.FilesChanged = sum.New, sum.Changed
			s.Duration = sum.End.Sub(sum.Start)
		}
		snaps = append(snaps, s)
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].Time.Before(snaps[j].Time) })
	return snaps, nil
}

// Stored is the space the given backups, a folder's as Snapshots found them,
// take in the destination after deduplication and compression. It goes by
// their IDs: a tag filter can't leave out the backups Snapshots drops.
func (d *Destination) Stored(ctx context.Context, snaps []Snapshot) (int64, error) {
	if len(snaps) == 0 {
		return 0, nil // with no IDs, restic stats would measure every backup
	}
	args := []string{"stats", "--json", "--retry-lock", retryLock, "--mode", "raw-data"}
	for _, s := range snaps {
		args = append(args, s.ID)
	}
	out, err := d.restic(ctx, d.asComputer("", args...))
	if err != nil {
		return 0, err
	}
	var s struct {
		TotalSize int64 `json:"total_size"`
	}
	if err := json.Unmarshal(out, &s); err != nil {
		return 0, fmt.Errorf("read restic's stats: %w", err)
	}
	return s.TotalSize, nil
}

// files lists the paths in a backup.
func (d *Destination) files(ctx context.Context, id string) ([]string, error) {
	out, err := d.restic(ctx, d.asComputer("", "ls", "--json", "--retry-lock", retryLock, id))
	if err != nil {
		return nil, err
	}
	var paths []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		var n struct {
			Type string `json:"message_type"`
			Path string `json:"path"`
		}
		if json.Unmarshal(sc.Bytes(), &n) == nil && n.Type == "node" {
			paths = append(paths, n.Path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// Configure makes root a backed-up folder: it writes the folder's config
// file, adds the folder to this computer's list, and gives the folder's
// earlier one-off backups its new ID so they join its history.
func (m *Machine) Configure(ctx context.Context, d *Destination, root string, cfg Config) error {
	// Before Abs, which would turn "" into the current directory.
	if root == "" {
		return errors.New("no folder to configure")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if err := d.notInside(root); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, ConfigFileName)); err == nil {
		if _, err := ReadConfig(root); err != nil {
			return fmt.Errorf("the folder's config file can't be read, so it isn't replaced; fix or remove it first: %w", err)
		}
	}
	if _, err := CheckConfigurable(root); err != nil {
		return err
	}
	if cfg.Destination != d.Name {
		return fmt.Errorf("the config names destination %s, not %s", cfg.Destination, d.Name)
	}
	// An ID-less Ref lists only one-offs, so no other folder's backup joins.
	oneOffs, err := d.Snapshots(ctx, m.Ref(root, nil))
	if err != nil {
		return err
	}
	var ids []string
	for _, s := range oneOffs {
		ids = append(ids, s.ID)
	}
	tag := d.asComputer("", append([]string{"tag", "--retry-lock", retryLock, "--add", tagFolder + cfg.ID}, ids...)...)
	if m.Opts.DryRun {
		fmt.Fprintf(m.out(), "[dry-run] write %s\n", filepath.Join(root, ConfigFileName))
		fmt.Fprintf(m.out(), "[dry-run] add %s to %s\n", root, m.foldersFile())
		if len(ids) > 0 {
			d.dryRun(tag)
		}
		return nil
	}
	if err := WriteConfig(root, cfg); err != nil {
		return err
	}
	if err := m.addFolder(root, cfg.ID, cfg.Schedule); err != nil {
		return err
	}
	if len(ids) > 0 {
		if _, err := d.restic(ctx, tag); err != nil {
			return fmt.Errorf("add the folder's earlier backups to its history: %w", err)
		}
	}
	return nil
}
