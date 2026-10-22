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
	Computers   int   // computers with a key on the destination
	FolderCount int   // distinct folders: one per ID, and one per one-off path and computer
	Stored      int64 // space every backup takes, after deduplication and compression

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
	o.Computers = len(computers)

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

	if len(snaps) > 0 {
		if o.Stored, err = d.storedAll(ctx); err != nil {
			return o, err
		}
	}
	return o, nil
}

// storedAll is the space every backup in the destination takes.
func (d *Destination) storedAll(ctx context.Context) (int64, error) {
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
