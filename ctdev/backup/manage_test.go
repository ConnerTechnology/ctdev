package backup

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// configuredExample is m's ~/Projects/website, configured for d.
func configuredExample(t *testing.T, m *Machine, d *Destination) (string, Config) {
	t.Helper()
	root := exampleFolder(t, m)
	cfg := NewConfig(d.Name)
	if err := m.Configure(context.Background(), d, root, cfg); err != nil {
		t.Fatal(err)
	}
	return root, cfg
}

func backUp(t *testing.T, m *Machine, d *Destination, root string, cfg *Config) Snapshot {
	t.Helper()
	snap, err := m.Backup(context.Background(), d, root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestListShowsEachBackupsStats(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root, cfg := configuredExample(t, m, d)

	backUp(t, m, d, root, &cfg)
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<p>example, changed</p>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("example addition\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backUp(t, m, d, root, &cfg)

	snaps, err := d.Snapshots(ctx, m.Ref(root, &cfg))
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Fatalf("listed %d backups, want 2", len(snaps))
	}
	first, second := snaps[0], snaps[1]
	if first.Files != 3 || first.FilesNew != 3 || first.FilesChanged != 0 {
		t.Errorf("first backup: files %d, new %d, changed %d; want 3, 3, 0", first.Files, first.FilesNew, first.FilesChanged)
	}
	if second.Files != 4 || second.FilesNew != 1 || second.FilesChanged != 1 {
		t.Errorf("second backup: files %d, new %d, changed %d; want 4, 1, 1", second.Files, second.FilesNew, second.FilesChanged)
	}
	for _, s := range snaps {
		if s.Size == 0 || s.Added == 0 || s.Duration <= 0 || s.Host != "example-laptop" || len(s.ShortID) != 8 {
			t.Errorf("backup %s is missing stats: %+v", s.ShortID, s)
		}
	}
}

// otherFolder makes and configures ~/Documents/notes under m's home, a second
// folder backed up to the same destination.
func otherFolder(t *testing.T, m *Machine, d *Destination) (string, Config) {
	t.Helper()
	dir := filepath.Join(m.Home, "Documents", "notes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "todo.txt"), []byte("example to-do\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := NewConfig(d.Name)
	if err := m.Configure(context.Background(), d, dir, cfg); err != nil {
		t.Fatal(err)
	}
	return dir, cfg
}

func countBackups(t *testing.T, m *Machine, d *Destination, root string, cfg *Config) int {
	t.Helper()
	snaps, err := d.Snapshots(context.Background(), m.Ref(root, cfg))
	if err != nil {
		t.Fatal(err)
	}
	return len(snaps)
}

func TestDeleteRemovesOnlyThatBackup(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root, cfg := configuredExample(t, m, d)
	other, otherCfg := otherFolder(t, m, d)
	backUp(t, m, d, root, &cfg)
	backUp(t, m, d, root, &cfg)
	backUp(t, m, d, other, &otherCfg)

	snaps, err := d.Snapshots(ctx, m.Ref(root, &cfg))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, snaps[0]); err != nil {
		t.Fatal(err)
	}
	left, err := d.Snapshots(ctx, m.Ref(root, &cfg))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].ID != snaps[1].ID {
		t.Errorf("after deleting %s, left %+v, want only %s", snaps[0].ShortID, left, snaps[1].ShortID)
	}
	if n := countBackups(t, m, d, other, &otherCfg); n != 1 {
		t.Errorf("the other folder has %d backups, want its 1 untouched", n)
	}
}

func TestDeleteAllRemovesOnlyThatFoldersBackups(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root, cfg := configuredExample(t, m, d)
	other, otherCfg := otherFolder(t, m, d)
	backUp(t, m, d, root, &cfg)
	backUp(t, m, d, root, &cfg)
	backUp(t, m, d, other, &otherCfg)

	n, err := d.deleteAll(ctx, m.Ref(root, &cfg))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("deleted %d backups, want 2", n)
	}
	if n := countBackups(t, m, d, root, &cfg); n != 0 {
		t.Errorf("the folder still has %d backups", n)
	}
	if n := countBackups(t, m, d, other, &otherCfg); n != 1 {
		t.Errorf("the other folder has %d backups, want its 1 untouched", n)
	}
}

// An unconfigured folder's backups are its one-offs, found by path and host.
func TestDeleteAllOfAnUnconfiguredFolderRemovesItsOneOffs(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root := exampleFolder(t, m)
	other, otherCfg := otherFolder(t, m, d)
	backUp(t, m, d, root, nil)
	backUp(t, m, d, other, &otherCfg)

	n, err := d.deleteAll(ctx, m.Ref(root, nil))
	if err != nil || n != 1 {
		t.Fatalf("deleteAll() = %d, %v; want 1 one-off deleted", n, err)
	}
	if n := countBackups(t, m, d, root, nil); n != 0 {
		t.Errorf("the folder still has %d one-offs", n)
	}
	if n := countBackups(t, m, d, other, &otherCfg); n != 1 {
		t.Errorf("the other folder has %d backups, want its 1 untouched", n)
	}
}

// Retention keeps the folder's newest backup per rule and forgets the rest of
// its backups only. Yearly keeps the test clear of hour and day boundaries.
func TestRetentionForgetsOnlyTheFoldersBackups(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root, cfg := configuredExample(t, m, d)
	other, otherCfg := otherFolder(t, m, d)
	backUp(t, m, d, other, &otherCfg)
	backUp(t, m, d, other, &otherCfg)
	backUp(t, m, d, root, &cfg)
	backUp(t, m, d, root, &cfg)
	newest := backUp(t, m, d, root, &cfg)

	cfg.Keep = Keep{Yearly: 1}
	if err := d.ApplyRetention(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	left, err := d.Snapshots(ctx, m.Ref(root, &cfg))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].ID != newest.ID {
		t.Errorf("kept %+v, want only the newest, %s", left, newest.ShortID)
	}
	if n := countBackups(t, m, d, other, &otherCfg); n != 2 {
		t.Errorf("the other folder has %d backups, want its 2 untouched", n)
	}
}

func TestRetentionWithNoRulesKeepsEverything(t *testing.T) {
	requireRestic(t)
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root, cfg := configuredExample(t, m, d)
	backUp(t, m, d, root, &cfg)
	backUp(t, m, d, root, &cfg)

	cfg.Keep = Keep{}
	if err := d.ApplyRetention(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if n := countBackups(t, m, d, root, &cfg); n != 2 {
		t.Errorf("%d backups left, want both", n)
	}
}

// Stopping takes the folder off this computer's list and removes its config
// file; its backups stay in the repository under the folder's ID.
func TestStopBackingUpKeepsTheBackups(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root, cfg := configuredExample(t, m, d)
	other, otherCfg := otherFolder(t, m, d)
	backUp(t, m, d, root, &cfg)

	if err := m.StopBackingUp(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ConfigFileName)); !os.IsNotExist(err) {
		t.Errorf("the config file is still there (%v)", err)
	}
	if on, err := m.scheduled(root, cfg.ID); err != nil || on {
		t.Errorf("still on this computer's list: %v, %v", on, err)
	}
	if on, err := m.scheduled(other, otherCfg.ID); err != nil || !on {
		t.Errorf("the other folder fell off the list: %v, %v", on, err)
	}

	st := m.Status(ctx, root)
	if st.ReadErr != nil {
		t.Fatal(st.ReadErr)
	}
	if st.Config != nil || st.Root != "" {
		t.Errorf("status still shows the folder as configured: %+v", st.Folder)
	}
	// Stop never rewrites tags: the backups stay the folder ID's, so the
	// unconfigured folder lists only one-offs, and it has none.
	if st.Backups != 0 {
		t.Errorf("status shows %d backups, want none: the kept ones keep the folder's ID", st.Backups)
	}
	if n := countBackups(t, m, d, root, &cfg); n != 1 {
		t.Errorf("%d backups with the folder's ID are in the repository, want the 1 kept", n)
	}
}

// sharedFolder gives the folder's ID backups from two computers: this one
// and an example desktop where a clone of the folder is backed up.
func sharedFolder(t *testing.T, m *Machine, d *Destination) (string, Config) {
	t.Helper()
	root, cfg := configuredExample(t, m, d)
	backUp(t, m, d, root, &cfg)
	desktop := testMachine(t, "example-desktop")
	clone := exampleFolder(t, desktop)
	writeConfig(t, clone, cfg)
	backUp(t, desktop, d, clone, &cfg)
	if n := countBackups(t, m, d, root, &cfg); n != 2 {
		t.Fatalf("the folder has %d backups, want one from each computer", n)
	}
	return root, cfg
}

func TestStopAndDeleteRemovesTheBackupsFromEveryComputer(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root, cfg := sharedFolder(t, m, d)
	other, otherCfg := otherFolder(t, m, d)
	backUp(t, m, d, other, &otherCfg)

	listed, err := d.Snapshots(ctx, m.Ref(root, &cfg))
	if err != nil || len(listed) != 2 {
		t.Fatalf("listed %d (%v), want both computers' backups", len(listed), err)
	}
	appeared, err := m.StopAndDelete(ctx, d, root, listed)
	if err != nil {
		t.Fatal(err)
	}
	if len(appeared) != 0 {
		t.Errorf("appeared = %+v, want none", appeared)
	}
	if left := countBackups(t, m, d, root, &cfg); left != 0 {
		t.Errorf("%d backups with the folder's ID are left", left)
	}
	if _, err := os.Stat(filepath.Join(root, ConfigFileName)); !os.IsNotExist(err) {
		t.Errorf("the config file is still there (%v)", err)
	}
	if on, err := m.scheduled(root, cfg.ID); err != nil || on {
		t.Errorf("still on this computer's list: %v, %v", on, err)
	}
	if n := countBackups(t, m, d, other, &otherCfg); n != 1 {
		t.Errorf("the other folder has %d backups, want its 1 untouched", n)
	}
}

// A backup that finishes between the listing and the delete, such as a
// scheduled one that was running, isn't in the list, so it's kept: Stop and
// delete lists again by the folder's ID afterwards and returns it, so the
// caller can say so.
func TestStopAndDeleteReturnsABackupThatAppearedMeanwhile(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root, cfg := configuredExample(t, m, d)
	backUp(t, m, d, root, &cfg)
	listed, err := d.Snapshots(ctx, m.Ref(root, &cfg))
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed %d (%v), want 1", len(listed), err)
	}
	late := backUp(t, m, d, root, &cfg)

	appeared, err := m.StopAndDelete(ctx, d, root, listed)
	if err != nil {
		t.Fatal(err)
	}
	if len(appeared) != 1 || appeared[0].ID != late.ID {
		t.Errorf("appeared = %+v, want the backup made after the listing, %s", appeared, late.ShortID)
	}
}

func TestStopWithoutDeletingKeepsEveryComputersBackups(t *testing.T) {
	requireRestic(t)
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root, cfg := sharedFolder(t, m, d)

	if err := m.StopBackingUp(root); err != nil {
		t.Fatal(err)
	}
	if n := countBackups(t, m, d, root, &cfg); n != 2 {
		t.Errorf("%d backups with the folder's ID are left, want both computers'", n)
	}
}

// A configured folder that moves takes its ID with it. A new, unconfigured
// folder later made at its old path must not claim the moved folder's
// backups, which carry that old path and this host: an ID-less reference
// matches only ID-less one-offs.
func TestANewFolderAtAMovedFoldersOldPathLeavesItsBackupsAlone(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root, cfg := configuredExample(t, m, d)
	backUp(t, m, d, root, &cfg)
	backUp(t, m, d, root, &cfg)

	moved := filepath.Join(filepath.Dir(root), "site")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "draft.txt"), []byte("example draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	newRef := m.Ref(root, nil)
	if snaps, err := d.Snapshots(ctx, newRef); err != nil || len(snaps) != 0 {
		t.Fatalf("the new folder lists %d backups (%v), want none of the moved folder's", len(snaps), err)
	}
	st := m.Status(ctx, root)
	if st.ReadErr != nil || st.Backups != 0 || st.Stored != 0 {
		t.Errorf("status of the new folder: %d backups, %d stored, %v; want none", st.Backups, st.Stored, st.ReadErr)
	}

	oneOff := backUp(t, m, d, root, nil)
	snaps, err := d.Snapshots(ctx, newRef)
	if err != nil || len(snaps) != 1 || snaps[0].ID != oneOff.ID {
		t.Fatalf("the new folder lists %v (%v), want only its one-off", snaps, err)
	}
	n, err := d.deleteAll(ctx, newRef)
	if err != nil || n != 1 {
		t.Fatalf("deleteAll() = %d, %v; want only the one-off", n, err)
	}
	if n := countBackups(t, m, d, moved, &cfg); n != 2 {
		t.Errorf("the moved folder has %d backups, want its 2 untouched", n)
	}
}

// zipNames lists the entries in the zip at path.
func zipNames(t *testing.T, path string) []string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var names []string
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	return names
}

// onlyBackup backs the folder up once and returns that backup as listed.
func onlyBackup(t *testing.T, m *Machine, d *Destination, root string, cfg *Config) Snapshot {
	t.Helper()
	backUp(t, m, d, root, cfg)
	snaps, err := d.Snapshots(context.Background(), m.Ref(root, cfg))
	if err != nil || len(snaps) != 1 {
		t.Fatalf("Snapshots() = %v, %v; want one backup", snaps, err)
	}
	return snaps[0]
}

func TestDownloadWritesAZipBesideTheFolder(t *testing.T) {
	requireRestic(t)
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root, cfg := configuredExample(t, m, d)
	snap := onlyBackup(t, m, d, root, &cfg)

	path, err := m.Download(context.Background(), d, root, snap)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(m.Home, "Projects", "website-"+snap.Time.UTC().Format("20060102T150405Z")+".zip")
	if path != want {
		t.Errorf("downloaded to %s, want %s", path, want)
	}
	if got := strings.Join(zipNames(t, path), ","); got != ".ctdev-backup.json,docs/,docs/readme.txt,index.html" {
		t.Errorf("zip holds %s, want the folder's files at their relative paths", got)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "website" && e.Name() != filepath.Base(path) {
			t.Errorf("left %s behind beside the zip", e.Name())
		}
	}
}

// A folder can hold a backed-up folder (CTD-87), so beside the inner one is
// inside the outer one; the zip goes beside the outermost instead.
func TestDownloadNeverLandsInsideABackedUpFolder(t *testing.T) {
	requireRestic(t)
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	root, cfg := configuredExample(t, m, d)
	writeConfig(t, filepath.Dir(root), NewConfig(d.Name)) // ~/Projects
	snap := onlyBackup(t, m, d, root, &cfg)

	path, err := m.Download(context.Background(), d, root, snap)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != m.Home {
		t.Errorf("downloaded to %s, want beside ~/Projects, in %s", path, m.Home)
	}
	if f, err := Find(filepath.Dir(path)); err != nil || f.Root != "" {
		t.Errorf("%s is inside backed-up folder %q (%v)", path, f.Root, err)
	}
}
