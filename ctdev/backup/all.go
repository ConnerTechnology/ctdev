package backup

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// FoundFolder is one backed-up folder as one computer backed it up: what
// `ctdev backup --all` lists. A folder with an ID backed up from two
// computers is found once under each.
type FoundFolder struct {
	// Ref opens the folder's backups with Snapshots: by its ID, from every
	// computer, or for one-offs, by path and computer.
	Ref  Ref
	Host string // the computer that made these backups
	Path string // the folder's path on that computer, ~/…, as of its latest backup

	Backups int       // this computer's backups of the folder
	Latest  time.Time // the newest of them
	Size    int64     // bytes in the newest
}

// Overview is the whole destination, for `ctdev backup --all`.
type Overview struct {
	Computers   int // computers with a key on the destination, by hostname
	FolderCount int // distinct folders: one per ID, and one per one-off path and computer

	// Folders are grouped by computer, this computer first and the rest by
	// hostname, and by path within a computer.
	Folders []FoundFolder
}

// Overview lists every folder backed up to the destination, from every
// computer. Backups without ctdev's path tag weren't made by ctdev and are
// left out.
func (d *Destination) Overview(ctx context.Context) (Overview, error) {
	var o Overview
	computers, err := d.Computers(ctx)
	if err != nil {
		return o, err
	}
	// A computer set up again, after a reinstall say, has a second key
	// under the same hostname.
	hosts := map[string]bool{}
	for _, c := range computers {
		hosts[c.Hostname] = true
	}
	o.Computers = len(hosts)

	snaps, err := d.snapshots(ctx, "snapshots", "--json", "--retry-lock", retryLock)
	if err != nil {
		return o, err
	}
	type key struct{ host, id, path string }
	found := map[key]*FoundFolder{}
	folders := map[Ref]bool{}
	for _, s := range snaps { // oldest first, so the last seen is the latest
		if s.Path == "" {
			continue
		}
		k := key{host: s.Host, id: s.FolderID}
		ref := Ref{FolderID: s.FolderID}
		if s.FolderID == "" {
			k.path = s.Path
			ref = Ref{Path: s.Path, Host: s.Host}
		}
		folders[ref] = true
		f := found[k]
		if f == nil {
			f = &FoundFolder{Ref: ref, Host: s.Host}
			found[k] = f
		}
		f.Backups++
		f.Latest, f.Size, f.Path = s.Time, s.Size, s.Path
	}
	o.FolderCount = len(folders)
	for _, f := range found {
		o.Folders = append(o.Folders, *f)
	}
	this := d.m.Hostname
	slices.SortFunc(o.Folders, func(a, b FoundFolder) int {
		if (a.Host == this) != (b.Host == this) {
			if a.Host == this {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(a.Host, b.Host), cmp.Compare(a.Path, b.Path), cmp.Compare(a.Ref.FolderID, b.Ref.FolderID))
	})
	return o, nil
}

// Linkable is the folders in o that Configure's "Connect to an existing
// folder's backups" may offer: those with an ID, less any this computer
// still backs up, meaning on its schedule list at a path that exists and
// still holds that ID's config. Two live folders sharing an ID would share
// retention and deletes.
func (m *Machine) Linkable(o Overview) ([]FoundFolder, error) {
	var list foldersFile
	if err := readJSON(m.foldersFile(), &list); err != nil {
		return nil, err
	}
	live := map[string]bool{}
	for _, e := range list.Folders {
		if cfg, err := ReadConfig(e.Path); err == nil && cfg.ID == e.ID {
			live[e.ID] = true
		}
	}
	var linkable []FoundFolder
	for _, f := range o.Folders {
		if f.Ref.FolderID != "" && !live[f.Ref.FolderID] {
			linkable = append(linkable, f)
		}
	}
	return linkable, nil
}

// StoredAll is the space every backup in the destination takes, after
// deduplication and compression. restic reads every backup's tree for it, so
// it's slower than Overview, which leaves it out.
func (d *Destination) StoredAll(ctx context.Context) (int64, error) {
	out, err := d.restic(ctx, d.asComputer("", "stats", "--json", "--retry-lock", retryLock, "--mode", "raw-data"))
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
