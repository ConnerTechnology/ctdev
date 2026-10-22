package backup

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// scheduledExample is m with a destination and ~/Projects/website
// configured on it, as of noon.
func scheduledExample(t *testing.T, m *Machine) (*Destination, string, Config) {
	t.Helper()
	m.Notifier = &fakeNotifier{}
	d, _ := createExample(t, m)
	at(m, noon)
	root, cfg := configuredExample(t, m, d)
	return d, root, cfg
}

// runAt runs the hourly runner at now and returns what it said about the
// destination's maintenance.
func runAt(t *testing.T, m *Machine, now time.Time) Maintenance {
	t.Helper()
	at(m, now)
	_, maint, err := m.RunScheduled(context.Background())
	if len(maint) != 1 {
		t.Fatalf("at %v: maintenance = %+v (err %v), want one destination's", now, maint, err)
	}
	if maint[0].Err != nil {
		t.Fatalf("at %v: maintenance failed: %v", now, maint[0].Err)
	}
	if err != nil {
		t.Fatalf("at %v: %v", now, err)
	}
	return maint[0]
}

// With no marker, the first hourly run maintains the destination and
// records the result in it. A week minus a minute later it isn't due; at a
// week it is, and reads the next slice.
func TestMaintenanceRunsWeeklyAndRotatesTheSlice(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, root, cfg := scheduledExample(t, m)

	got := runAt(t, m, noon)
	if !got.Ran || got.Check == nil || !got.Check.OK || got.Check.Slice != 1 || got.Check.Host != "example-laptop" ||
		!got.Check.Started.Equal(noon) {
		t.Fatalf("first run: %+v %+v, want slice 1 checked OK by example-laptop at noon", got, got.Check)
	}
	last, err := d.LastDataCheck(ctx)
	if err != nil || last == nil || !last.OK || last.Slice != 1 {
		t.Fatalf("LastDataCheck = %+v, %v; want slice 1 OK", last, err)
	}

	if got := runAt(t, m, noon.Add(maintenanceInterval-time.Minute)); got.Ran || got.Check == nil || got.Check.Slice != 1 {
		t.Errorf("before a week: %+v, want not run, showing slice 1", got)
	}
	if got := runAt(t, m, noon.Add(maintenanceInterval)); !got.Ran || !got.Check.OK || got.Check.Slice != 2 {
		t.Errorf("at a week: %+v %+v, want slice 2 checked OK", got, got.Check)
	}

	// The markers are never one of the folder's backups, and one is left.
	if n := countBackups(t, m, d, root, &cfg); n != 2 {
		t.Errorf("the folder has %d backups, want 2 (one per daily run)", n)
	}
	if n := countBackups(t, m, d, root, nil); n != 0 {
		t.Errorf("the folder has %d one-offs, want none", n)
	}
	ms, _, err := d.markers(ctx)
	if err != nil || len(ms) != 1 {
		t.Errorf("markers = %+v, %v; want only the latest result", ms, err)
	}

	// The folder's status shows the destination's data check, and counts
	// only the folder's backups.
	s := m.Status(ctx, root)
	if s.ReadErr != nil || !s.DataCheckRead || s.DataCheck == nil || !s.DataCheck.OK || s.DataCheck.Slice != 2 {
		t.Errorf("status data check = %+v (read %v, err %v), want slice 2 OK", s.DataCheck, s.DataCheckRead, s.ReadErr)
	}
	if s.Backups != 2 {
		t.Errorf("status counts %d backups, want 2", s.Backups)
	}
	// Nor is it a folder in ctdev backup --all.
	o, err := d.Overview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if o.FolderCount != 1 || len(o.Folders) != 1 || o.Folders[0].Backups != 2 || o.Folders[0].Ref.FolderID != cfg.ID {
		t.Errorf("--all overview = %+v, want just the folder's 2 backups", o)
	}
}

// Before any maintenance, the status says the data check hasn't run.
func TestStatusBeforeAnyDataCheck(t *testing.T) {
	requireRestic(t)
	m := testMachine(t, "example-laptop")
	_, root, _ := scheduledExample(t, m)
	s := m.Status(context.Background(), root)
	if s.ReadErr != nil || !s.DataCheckRead || s.DataCheck != nil {
		t.Errorf("status data check = %+v (read %v, err %v), want read and none yet", s.DataCheck, s.DataCheckRead, s.ReadErr)
	}
}

// Two computers share a destination: after one maintains it, the other sees
// the result in the destination, keeps a copy for ctdev doctor, and doesn't
// run it again that week. A week on, whichever runs first takes the next
// slice, and the first sees that.
func TestTwoComputersShareTheWeeklyMaintenance(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	laptop := testMachine(t, "example-laptop")
	laptop.Notifier = &fakeNotifier{}
	d, setup := createExample(t, laptop)
	at(laptop, noon)
	configuredExample(t, laptop, d)

	desktop := testMachine(t, "example-desktop")
	desktop.Notifier = &fakeNotifier{}
	d2, err := desktop.ConnectDestination(ctx, setup.Kit, exampleMaster)
	if err != nil {
		t.Fatal(err)
	}
	at(desktop, noon)
	configuredExample(t, desktop, d2)

	if got := runAt(t, laptop, noon); !got.Ran || got.Check.Slice != 1 {
		t.Fatalf("laptop: %+v, want it to run slice 1", got)
	}
	got := runAt(t, desktop, noon.Add(time.Hour))
	if got.Ran || got.Check == nil || got.Check.Host != "example-laptop" || got.Check.Slice != 1 || !got.Check.OK {
		t.Errorf("desktop an hour later: %+v %+v, want the laptop's result and no run", got, got.Check)
	}
	checks, err := desktop.DataChecks()
	if err != nil || len(checks) != 1 || checks[0].Destination != d.Name || checks[0].Check.Host != "example-laptop" {
		t.Errorf("desktop's DataChecks = %+v, %v; want the laptop's result", checks, err)
	}

	if got := runAt(t, desktop, noon.Add(maintenanceInterval)); !got.Ran || got.Check.Slice != 2 {
		t.Errorf("desktop a week later: %+v, want it to run slice 2", got)
	}
	got = runAt(t, laptop, noon.Add(maintenanceInterval+time.Hour))
	if got.Ran || got.Check.Host != "example-desktop" || got.Check.Slice != 2 {
		t.Errorf("laptop after that: %+v %+v, want the desktop's slice 2 and no run", got, got.Check)
	}
}

// The losing side of a race: another computer claims between this one's
// claim and its second look. This one backs off, whichever claim is
// earlier: it removes its claim, runs nothing, and waits an hour plus its
// jitter before claiming again.
func TestARaceForTheMaintenanceBacksOff(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _, _ := scheduledExample(t, m)
	m.jitterFn = func() time.Duration { return 2 * time.Hour }
	var other string
	m.afterClaim = func() {
		// Later than this computer's claim, so it isn't the earliest.
		id, err := d.writeMarker(ctx, DataCheck{Host: "example-desktop", Started: noon.Add(time.Minute), Slice: 1})
		if err != nil {
			t.Error(err)
		}
		other = id
	}

	if got := runAt(t, m, noon); got.Ran || got.Check != nil {
		t.Errorf("ran despite example-desktop's claim: %+v", got)
	}
	ms, _, err := d.markers(ctx)
	if err != nil || len(ms) != 1 || ms[0].ID != other {
		t.Errorf("markers = %+v, %v; want only example-desktop's claim", ms, err)
	}
	states, err := m.destinationStates()
	if err != nil || !states[d.Name].RetryAfter.Equal(noon.Add(2*time.Hour)) {
		t.Errorf("retry after = %v (%v), want noon plus the 2h jitter", states[d.Name].RetryAfter, err)
	}

	// example-desktop backed off too and removed its claim. This computer
	// waits out its jitter, then runs.
	if err := d.forget(ctx, other); err != nil {
		t.Fatal(err)
	}
	m.afterClaim = nil
	if got := runAt(t, m, noon.Add(time.Hour)); got.Ran {
		t.Errorf("claimed again before the jitter ran out: %+v", got)
	}
	if got := runAt(t, m, noon.Add(2*time.Hour)); !got.Ran {
		t.Errorf("didn't run after the jitter: %+v", got)
	}
}

// The slice after 20 is 1 again.
func TestTheSliceWrapsAfter20(t *testing.T) {
	requireRestic(t)
	m := testMachine(t, "example-laptop")
	d, _, _ := scheduledExample(t, m)
	week := noon.Add(-maintenanceInterval)
	if _, err := d.writeMarker(context.Background(), DataCheck{Host: "example-desktop", Started: week, Finished: week, Slice: Slices, OK: true}); err != nil {
		t.Fatal(err)
	}
	if got := runAt(t, m, noon); !got.Ran || got.Check.Slice != 1 {
		t.Errorf("after slice 20: %+v %+v, want slice 1", got, got.Check)
	}
}

// Another computer's run in progress keeps this one from starting a second;
// a claim that's a day old was abandoned and doesn't.
func TestAClaimInProgressIsLeftToFinish(t *testing.T) {
	requireRestic(t)
	m := testMachine(t, "example-laptop")
	d, _, _ := scheduledExample(t, m)
	if _, err := d.writeMarker(context.Background(), DataCheck{Host: "example-desktop", Started: noon.Add(-time.Hour), Slice: 1}); err != nil {
		t.Fatal(err)
	}
	if got := runAt(t, m, noon); got.Ran {
		t.Errorf("ran while example-desktop's run was in progress: %+v", got)
	}
	if got := runAt(t, m, noon.Add(claimTTL)); !got.Ran || got.Check.Slice != 1 {
		t.Errorf("after the claim was abandoned: %+v, want this computer to run slice 1", got)
	}
}

// Prune runs with the maintenance, so a deleted backup's space is freed.
func TestMaintenanceFreesTheSpaceOfADeletedBackup(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, root, cfg := scheduledExample(t, m)
	big := make([]byte, 4<<20)
	if _, err := rand.Read(big); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "example.bin"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	runAt(t, m, noon) // backs up, and maintains with nothing to free

	snaps, err := d.Snapshots(ctx, m.Ref(root, &cfg))
	if err != nil || len(snaps) != 1 {
		t.Fatalf("snapshots = %d, %v", len(snaps), err)
	}
	if err := d.DeleteBackups(ctx, snaps); err != nil {
		t.Fatal(err)
	}
	// So the week's backup doesn't hold the data again.
	if err := os.Remove(filepath.Join(root, "example.bin")); err != nil {
		t.Fatal(err)
	}
	before := dirSize(t, d.Address)
	if got := runAt(t, m, noon.Add(maintenanceInterval)); !got.Ran || !got.Check.OK {
		t.Fatalf("maintenance: %+v %+v", got, got.Check)
	}
	if after := dirSize(t, d.Address); before-after < 3<<20 {
		t.Errorf("the destination went from %d to %d bytes, want the 4 MB backup's space freed", before, after)
	}
}

func dirSize(t *testing.T, dir string) int64 {
	t.Helper()
	var n int64
	err := filepath.Walk(dir, func(_ string, fi os.FileInfo, err error) error {
		if err == nil && fi.Mode().IsRegular() {
			n += fi.Size()
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// biggestPack is the largest pack file in a local destination: the one
// holding a folder's file data, not its trees or a marker.
func biggestPack(t *testing.T, repo string) string {
	t.Helper()
	var path string
	var size int64
	err := filepath.Walk(filepath.Join(repo, "data"), func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi.Mode().IsRegular() && fi.Size() > size {
			path, size = p, fi.Size()
		}
		return err
	})
	if err != nil || path == "" {
		t.Fatalf("no pack in %s: %v", repo, err)
	}
	return path
}

// corruptExample backs up a folder with a 1 MB file at noon, maintaining the
// destination, then corrupts the data pack on disk: zeroes all but its last
// 4 KB, which hold the pack's header. The 1 MB file's blob is damaged; a
// small file's blob near the end of the pack may not be. It returns the pack
// and the slice that reads it.
func corruptExample(t *testing.T, m *Machine) (d *Destination, pack string, slice int) {
	t.Helper()
	d, root, _ := scheduledExample(t, m)
	big := make([]byte, 1<<20)
	if _, err := rand.Read(big); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "example.bin"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	runAt(t, m, noon)

	pack = biggestPack(t, d.Address)
	// restic's subset n/t reads the packs whose ID's first byte is n-1 mod t.
	first, err := strconv.ParseUint(filepath.Base(pack)[:2], 16, 8)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(pack, 0o600); err != nil { // restic leaves packs read-only
		t.Fatal(err)
	}
	fi, err := os.Stat(pack)
	if err != nil {
		t.Fatal(err)
	}
	// The pack's header, which says where each blob is, is at its end.
	f, err := os.OpenFile(pack, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(make([]byte, fi.Size()-4096), 0); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return d, pack, int(first)%Slices + 1
}

// dueWithSlice makes maintenance due at a week past noon, with slice next.
func dueWithSlice(t *testing.T, d *Destination, next int) {
	t.Helper()
	prev := next - 1
	if prev == 0 {
		prev = Slices
	}
	at := noon.Add(time.Minute)
	if _, err := d.writeMarker(context.Background(), DataCheck{Host: "example-desktop", Started: at, Finished: at, Slice: prev, OK: true}); err != nil {
		t.Fatal(err)
	}
}

// failedRun runs the hourly runner a week past noon and returns its failed
// maintenance and the run's error.
func failedRun(t *testing.T, m *Machine) (DataCheck, error) {
	t.Helper()
	at(m, noon.Add(maintenanceInterval+time.Minute))
	_, maint, err := m.RunScheduled(context.Background())
	if err == nil || len(maint) != 1 || !maint[0].Ran || maint[0].Check == nil || maint[0].Check.OK {
		t.Fatalf("maintenance = %+v, err %v; want it run and failed", maint, err)
	}
	return *maint[0].Check, err
}

// A pack corrupted on disk fails the data check in the week its slice comes
// up, and the failure is recorded in the marker for every computer.
func TestACorruptPackFailsTheCheckWhenItsSliceComesUp(t *testing.T) {
	requireRestic(t)
	m := testMachine(t, "example-laptop")
	d, pack, slice := corruptExample(t, m)
	dueWithSlice(t, d, slice)

	c, runErr := failedRun(t, m)
	if c.Step != StepCheck || c.Slice != slice {
		t.Fatalf("result = %+v, want slice %d failed at check", c, slice)
	}
	if want := "damaged pack " + filepath.Base(pack)[:8]; !strings.HasPrefix(c.Error, want) {
		t.Errorf("the check's failure = %q, want it to start %q", c.Error, want)
	}
	last, err := d.LastDataCheck(context.Background())
	if err != nil || last == nil || last.OK || last.Step != StepCheck || last.Slice != slice {
		t.Errorf("the marker says %+v (%v), want the failed check", last, err)
	}
	if !strings.Contains(runErr.Error(), "data check failed at check") {
		t.Errorf("err = %v, want it to name the check", runErr)
	}
	if sent := m.Notifier.(*fakeNotifier).sent; len(sent) != 1 || !strings.HasPrefix(sent[0], "Backup data check failed: example-backups: failed at check: ") {
		t.Errorf("notifications = %q, want one data check failed", sent)
	}
}

// In a week whose slice doesn't hold the damaged pack, the check passes, and
// the test download of a file from the folder's latest backup catches it.
func TestTheTestDownloadCatchesDamageTheSliceMissed(t *testing.T) {
	requireRestic(t)
	m := testMachine(t, "example-laptop")
	d, _, slice := corruptExample(t, m)
	dueWithSlice(t, d, slice%Slices+1)
	// The large file: a small file's blob can sit in the pack's last 4 KB,
	// which corruptExample leaves whole.
	m.pickFn = func(paths []string) int { return slices.Index(paths, "/example.bin") }

	c, _ := failedRun(t, m)
	if c.Step != StepDownload || !strings.Contains(c.Error, "from the backup of ~/Projects/website") {
		t.Errorf("result = %+v, want a failed test download from ~/Projects/website", c)
	}
}

// A failed data check notifies once, not every hour, and once more when a
// later check passes.
func TestADataCheckFailureNotifiesOnceAndOnRecovery(t *testing.T) {
	m, _, n := failingMachine(t)
	failed := DataCheck{Host: "example-desktop", Started: noon, Finished: noon, Slice: 3, Step: StepCheck, Error: "example failure"}
	if err := m.recordDataCheck("example-backups", failed); err != nil {
		t.Fatal(err)
	}
	dataCheck := func() []string {
		var got []string
		for _, s := range n.sent {
			if strings.HasPrefix(s, "Backup data check") {
				got = append(got, s)
			}
		}
		return got
	}
	for range 2 {
		_, _, _ = m.RunScheduled(context.Background())
	}
	if got := dataCheck(); len(got) != 1 || got[0] != "Backup data check failed: example-backups: failed at check: example failure" {
		t.Fatalf("after two runs: %q, want one failed notification", got)
	}
	ok := DataCheck{Host: "example-desktop", Started: noon, Finished: noon, Slice: 4, OK: true}
	if err := m.recordDataCheck("example-backups", ok); err != nil {
		t.Fatal(err)
	}
	_, _, _ = m.RunScheduled(context.Background())
	if got := dataCheck(); len(got) != 2 || got[1] != "Backup data check passing again: example-backups: the data check passed." {
		t.Errorf("after it passed: %q, want a passing-again notification", got)
	}
}
