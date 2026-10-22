package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// oneOffFolder makes ~/Downloads/scans under m's home and backs it up once,
// without configuring it.
func oneOffFolder(t *testing.T, m *Machine, d *Destination) string {
	t.Helper()
	dir := filepath.Join(m.Home, "Downloads", "scans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scan.txt"), []byte("example scan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backUp(t, m, d, dir, nil)
	return dir
}

// A new computer connects with the kit and the master password, and the
// destination's overview lists every folder backed up by the first one,
// grouped by computer with this one first, with paths as each recorded them.
func TestANewComputerListsEveryFolderGroupedByComputer(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	laptop := testMachine(t, "example-laptop")
	ld, s := createExample(t, laptop)
	website, cfg := configuredExample(t, laptop, ld)
	backUp(t, laptop, ld, website, &cfg)
	backUp(t, laptop, ld, website, &cfg)
	listed, err := ld.Snapshots(ctx, laptop.Ref(website, &cfg))
	if err != nil || len(listed) != 2 {
		t.Fatalf("listed %d (%v), want 2", len(listed), err)
	}
	latest := listed[1]
	oneOffFolder(t, laptop, ld)

	desktop := testMachine(t, "example-desktop")
	dd, err := desktop.ConnectDestination(ctx, s.Kit, exampleMaster)
	if err != nil {
		t.Fatal(err)
	}
	notes, notesCfg := otherFolder(t, desktop, dd)
	backUp(t, desktop, dd, notes, &notesCfg)
	// The laptop, set up again after a reinstall, has a second key under the
	// same hostname: still one computer.
	if _, err := testMachine(t, "example-laptop").ConnectDestination(ctx, s.Kit, exampleMaster); err != nil {
		t.Fatal(err)
	}

	o, err := dd.Overview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if o.Computers != 2 || o.FolderCount != 3 {
		t.Errorf("overview: %d computers, %d folders; want 2 and 3", o.Computers, o.FolderCount)
	}
	if stored, err := dd.StoredAll(ctx); err != nil || stored <= 0 {
		t.Errorf("StoredAll() = %d, %v; want some", stored, err)
	}
	type entry struct{ host, path, id string }
	want := []entry{
		{"example-desktop", "~/Documents/notes", notesCfg.ID},
		{"example-laptop", "~/Downloads/scans", ""},
		{"example-laptop", "~/Projects/website", cfg.ID},
	}
	if len(o.Folders) != len(want) {
		t.Fatalf("folders = %+v, want %d", o.Folders, len(want))
	}
	for i, w := range want {
		f := o.Folders[i]
		if f.Host != w.host || f.Path != w.path || f.Ref.FolderID != w.id {
			t.Errorf("folder %d = %s %s %q, want %s %s %q", i, f.Host, f.Path, f.Ref.FolderID, w.host, w.path, w.id)
		}
	}
	site := o.Folders[2]
	if site.Backups != 2 || !site.Latest.Equal(latest.Time) || site.Size != latest.Size {
		t.Errorf("website: %d backups, latest %v (%d bytes); want 2, %v (%d bytes)", site.Backups, site.Latest, site.Size, latest.Time, latest.Size)
	}

	// Each entry's Ref opens the same list a folder's own List backups shows.
	for _, f := range o.Folders {
		snaps, err := dd.Snapshots(ctx, f.Ref)
		if err != nil || len(snaps) != f.Backups {
			t.Errorf("%s: Snapshots(Ref) = %d backups (%v), want %d", f.Path, len(snaps), err, f.Backups)
		}
	}
}

// Configure's "Connect to an existing folder's backups": a folder set up on
// a new computer takes the ID of a folder found in the overview, so its next
// backup joins that history, and so do its earlier one-offs.
func TestLinkingANewFolderContinuesAnExistingFoldersHistory(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	laptop := testMachine(t, "example-laptop")
	ld, s := createExample(t, laptop)
	website, cfg := configuredExample(t, laptop, ld)
	backUp(t, laptop, ld, website, &cfg)

	desktop := testMachine(t, "example-desktop")
	dd, err := desktop.ConnectDestination(ctx, s.Kit, exampleMaster)
	if err != nil {
		t.Fatal(err)
	}
	// Rebuilt at a different path on the desktop, and backed up once first.
	rebuilt := filepath.Join(desktop.Home, "Sites", "website")
	if err := os.MkdirAll(rebuilt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rebuilt, "index.html"), []byte("<p>example, rebuilt</p>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backUp(t, desktop, dd, rebuilt, nil)

	o, err := dd.Overview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var existing FoundFolder
	for _, f := range o.Folders {
		if f.Path == "~/Projects/website" {
			existing = f
		}
	}
	if existing.Ref.FolderID != cfg.ID {
		t.Fatalf("the overview doesn't list the laptop's folder with its ID: %+v", o.Folders)
	}

	linked := NewConfig(dd.Name)
	linked.ID = existing.Ref.FolderID
	if err := desktop.Configure(ctx, dd, rebuilt, linked); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadConfig(rebuilt); err != nil || got.ID != cfg.ID {
		t.Fatalf("the rebuilt folder's config has ID %q (%v), want %s", got.ID, err, cfg.ID)
	}
	backUp(t, desktop, dd, rebuilt, &linked)

	snaps, err := dd.Snapshots(ctx, existing.Ref)
	if err != nil {
		t.Fatal(err)
	}
	hosts := map[string]int{}
	for _, s := range snaps {
		hosts[s.Host]++
		if s.FolderID != cfg.ID {
			t.Errorf("backup %s has folder ID %q, want %s", s.ShortID, s.FolderID, cfg.ID)
		}
	}
	if hosts["example-laptop"] != 1 || hosts["example-desktop"] != 2 {
		t.Errorf("the folder's backups by computer = %v, want the laptop's 1 and the desktop's one-off and backup", hosts)
	}

	// The overview now finds the folder once under each computer, and no
	// one-offs are left over.
	if o, err = dd.Overview(ctx); err != nil {
		t.Fatal(err)
	}
	if o.FolderCount != 1 || len(o.Folders) != 2 {
		t.Fatalf("overview: %d folders in %+v, want one folder found under two computers", o.FolderCount, o.Folders)
	}
	if f := o.Folders[0]; f.Host != "example-desktop" || f.Path != "~/Sites/website" || f.Backups != 2 {
		t.Errorf("this computer's entry = %+v, want ~/Sites/website with 2 backups", f)
	}
}

// Under --all, a zip goes beside where the folder would be when that parent
// exists on this computer, and otherwise into the current directory, or
// beside the backed-up folder the current directory is in. It never lands
// inside a backed-up folder.
func TestDownloadUnderAllPicksWhereTheZipGoes(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	website, cfg := configuredExample(t, m, d)
	snap := onlyBackup(t, m, d, website, &cfg)
	notes, _ := otherFolder(t, m, d)
	zip := "website-" + snap.Time.UTC().Format("20060102T150405Z") + ".zip"
	elsewhere := t.TempDir()
	inNotes := filepath.Join(notes, "drafts")
	if err := os.MkdirAll(inNotes, 0o755); err != nil {
		t.Fatal(err)
	}
	projects := filepath.Join(m.Home, "Projects")
	moved := filepath.Join(m.Home, "Moved")

	for _, tc := range []struct {
		name  string
		setup func()
		cwd   string
		want  string
	}{
		{"the original parent exists here", func() {}, elsewhere, projects},
		{"the original parent is inside a backed-up folder", func() {
			writeConfig(t, projects, NewConfig(d.Name))
		}, elsewhere, m.Home},
		{"no original parent: the current directory", func() {
			if err := os.Rename(projects, moved); err != nil {
				t.Fatal(err)
			}
		}, elsewhere, elsewhere},
		{"no original parent, inside a backed-up folder", func() {}, inNotes, filepath.Join(m.Home, "Documents")},
	} {
		tc.setup()
		path, err := m.DownloadFromAll(ctx, d, tc.cwd, snap)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if want := filepath.Join(tc.want, zip); path != want {
			t.Errorf("%s: downloaded to %s, want %s", tc.name, path, want)
		}
		if f, err := Find(filepath.Dir(path)); err != nil || f.Root != "" {
			t.Errorf("%s: %s is inside backed-up folder %q (%v)", tc.name, path, f.Root, err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

// "Connect to an existing folder's backups" never offers the ID of a folder
// this computer still backs up: two live folders sharing an ID would share
// retention and deletes. A stopped or deleted folder's ID is offered, and
// one-offs, which have no ID, never are.
func TestLinkableLeavesOutFoldersStillBackedUpHere(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	website, cfg := configuredExample(t, m, d)
	backUp(t, m, d, website, &cfg)
	notes, notesCfg := otherFolder(t, m, d)
	backUp(t, m, d, notes, &notesCfg)
	if err := m.StopBackingUp(notes); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(m.Home, "Old", "site")
	if err := os.MkdirAll(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	goneCfg := NewConfig(d.Name)
	if err := m.Configure(ctx, d, gone, goneCfg); err != nil {
		t.Fatal(err)
	}
	backUp(t, m, d, gone, &goneCfg)
	if err := os.RemoveAll(filepath.Join(m.Home, "Old")); err != nil {
		t.Fatal(err)
	}
	oneOffFolder(t, m, d)

	o, err := d.Overview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	linkable, err := m.Linkable(o)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range linkable {
		got = append(got, f.Path)
	}
	if want := "~/Documents/notes,~/Old/site"; strings.Join(got, ",") != want {
		t.Errorf("linkable = %v, want %s", got, want)
	}
}
