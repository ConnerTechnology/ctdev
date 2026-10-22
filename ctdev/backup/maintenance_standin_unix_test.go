//go:build unix

package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// maintenanceStandin gives m a destination whose stand-in restic keeps one
// marker: backup --stdin saves the claim, snapshots --tag ctdev-maintenance
// lists it, and forget drops it. Other snapshots lists are empty. fail maps
// a command (prune, check, or "forget-folder" for a folder's keep rules) to
// the exit code it fails with. It returns the log of restic's arguments.
func maintenanceStandin(t *testing.T, m *Machine, fail map[string]string) (*Destination, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "restic.log")
	markers := filepath.Join(dir, "markers")
	if err := os.Mkdir(markers, 0o755); err != nil {
		t.Fatal(err)
	}
	code := func(cmd string) string {
		if c, ok := fail[cmd]; ok {
			return `echo "Fatal: example failure" >&2; exit ` + c
		}
		return ":"
	}
	m.Restic = filepath.Join(dir, "restic")
	script := `#!/bin/sh
echo "$*" >> "` + log + `"
markers="` + markers + `"
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
backup)
  for a in "$@"; do case "$a" in ctdev-maintenance-data:*) tag="$a" ;; esac; done
  id="c1a1c1a1c1a1c1a$(ls "$markers" | wc -l | tr -d ' ')"
  echo '{"id":"'"$id"'","tags":["ctdev-maintenance","'"$tag"'"]}' > "$markers/$id"
  echo '{"message_type":"summary","snapshot_id":"'"$id"'"}' ;;
snapshots)
  case "$*" in
  *ctdev-maintenance*)
    printf '['; sep=""
    for f in "$markers"/*; do [ -e "$f" ] || continue; printf '%s' "$sep"; cat "$f"; sep=","; done
    echo ']' ;;
  *) echo '[]' ;;
  esac ;;
forget)
  case "$*" in
  *ctdev-folder:*) ` + code("forget-folder") + ` ;;
  *) ` + code("forget-marker") + `; for a in "$@"; do rm -f "$markers/$a"; done ;;
  esac ;;
prune) ` + code("prune") + ` ;;
check) ` + code("check") + ` ;;
esac
`
	if err := os.WriteFile(m.Restic, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Destination{Name: "example-backups", Address: filepath.Join(dir, "repo"), m: m}, log
}

// A prune or check that can't lock the destination within --retry-lock
// (restic's exit 11) isn't a failed data check: the claim is removed, no
// result is written or kept, and the next hour tries again.
func TestABusyDestinationLeavesTheMaintenanceForLater(t *testing.T) {
	for _, step := range []string{"prune", "check"} {
		t.Run(step, func(t *testing.T) {
			m := testMachine(t, "example-laptop")
			at(m, noon)
			d, log := maintenanceStandin(t, m, map[string]string{step: "11"})

			res := m.maintainIfDue(context.Background(), d, nil)
			if !res.Busy || res.Ran || res.Check != nil || res.Err != nil {
				t.Errorf("result = %+v, want busy with nothing recorded", res)
			}
			ms, _, err := d.markers(context.Background())
			if err != nil || len(ms) != 0 {
				t.Errorf("markers = %+v, %v; want the claim removed", ms, err)
			}
			data, _ := os.ReadFile(log)
			if n := strings.Count(string(data), "backup --stdin"); n != 1 {
				t.Errorf("wrote %d markers, want only the claim:\n%s", n, data)
			}
			if states, _ := m.destinationStates(); states[d.Name].DataCheck != nil || len(states[d.Name].Claims) != 0 {
				t.Errorf("state.json keeps %+v, want no data check and no claim", states[d.Name])
			}
		})
	}
}

// A folder whose keep rules fail at the maintenance's forget step gets the
// failure as its own retention error. The maintenance goes on, and its data
// check passes on prune, check and test download alone.
func TestAForgetFailureIsTheFoldersNotTheDataChecks(t *testing.T) {
	m := testMachine(t, "example-laptop")
	at(m, noon)
	d, log := maintenanceStandin(t, m, map[string]string{"forget-folder": "1"})
	root := exampleFolder(t, m)

	res := m.maintainIfDue(context.Background(), d, []listedFolder{{Root: root, Config: NewConfig(d.Name)}})
	if res.Err != nil || !res.Ran || res.Check == nil || !res.Check.OK {
		t.Errorf("result = %+v %+v, want the data check OK", res, res.Check)
	}
	if r, _, _ := m.lastRun(root); !strings.Contains(r.RetentionError, "example failure") {
		t.Errorf("folder state = %+v, want its retention error", r)
	}
	data, _ := os.ReadFile(log)
	for _, cmd := range []string{"\nprune", "\ncheck"} {
		if !strings.Contains(string(data), cmd) {
			t.Errorf("restic%s didn't run after the forget failed:\n%s", cmd, data)
		}
	}
}

// lockedStandin gives the failingMachine's destination a stand-in restic
// that finds it locked (exit 11) for everything but version, logging each
// command.
func lockedStandin(t *testing.T, m *Machine) (log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "restic.log")
	m.Restic = filepath.Join(dir, "restic")
	script := `#!/bin/sh
echo "$*" >> "` + log + `"
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
*) echo "Fatal: unable to create lock in backend: repository is already locked exclusively" >&2; exit 11 ;;
esac
`
	if err := os.WriteFile(m.Restic, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	env := m.destinationFile("example-backups")
	if err := os.MkdirAll(filepath.Dir(env), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("RESTIC_REPOSITORY="+filepath.Join(dir, "repo")+"\nCTDEV_COMPUTER_KEY=example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return log
}

// A scheduled backup that finds the destination locked is deferred: no
// error, no notification, and its last attempt and success stand. Only the
// overdue rule turns it red.
func TestALockedDestinationDefersTheScheduledBackup(t *testing.T) {
	m, root, n := failingMachine(t)
	lockedStandin(t, m)
	if err := m.recordRun(root, noon.Add(-25*time.Hour), nil); err != nil {
		t.Fatal(err)
	}

	results, maint, err := m.RunScheduled(context.Background())
	if err != nil {
		t.Errorf("err = %v, want none", err)
	}
	if len(results) != 1 || results[0].Outcome != Deferred || results[0].Err != nil {
		t.Errorf("results = %+v, want the folder deferred", results)
	}
	if len(maint) != 1 || !maint[0].Busy || maint[0].Err != nil {
		t.Errorf("maintenance = %+v, want busy", maint)
	}
	r, _, _ := m.lastRun(root)
	if r.Failed() || r.Deferred == "" || !r.DeferredAt.Equal(noon) || !r.LastAttempt.Equal(noon.Add(-25*time.Hour)) {
		t.Errorf("state = %+v, want deferred at noon, the last attempt unchanged", r)
	}
	if len(n.sent) != 0 {
		t.Errorf("notifications = %q, want none", n.sent)
	}
	if h := health(t, m); h[0].State != HealthOK {
		t.Errorf("health = %q, want ok", h[0].State)
	}
	// Still locked a day later: the last success is 49 hours old, which
	// is overdue for a daily folder.
	at(m, noon.Add(24*time.Hour))
	if _, _, err := m.RunScheduled(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h := health(t, m); h[0].State != HealthOverdue {
		t.Errorf("health = %q, want overdue", h[0].State)
	}
	// A backup that runs clears it.
	if err := m.recordRun(root, noon.Add(25*time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	if r, _, _ := m.lastRun(root); r.Deferred != "" {
		t.Errorf("state = %+v, want deferred cleared", r)
	}
}

// Back up now waits a minute for a locked destination, not half an hour,
// then returns ErrDestinationBusy, recorded as deferred.
func TestBackupNowWaitsAMinuteForALockedDestination(t *testing.T) {
	m, root, _ := failingMachine(t)
	log := lockedStandin(t, m)
	d, err := m.Destination("example-backups")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.BackupNow(context.Background(), d, root, &cfg); !errors.Is(err, ErrDestinationBusy) {
		t.Errorf("err = %v, want ErrDestinationBusy", err)
	}
	data, _ := os.ReadFile(log)
	if !strings.Contains(string(data), "backup . --json --retry-lock 1m ") {
		t.Errorf("restic ran:\n%s\nwant a backup waiting 1m for the lock", data)
	}
	if r, _, _ := m.lastRun(root); r.Failed() || r.Deferred == "" {
		t.Errorf("state = %+v, want deferred", r)
	}
}

// The next maintenance is a week after the last one started, or now when
// that's passed or none has run.
func TestNextMaintenance(t *testing.T) {
	m := testMachine(t, "example-laptop")
	at(m, noon)
	d, _ := maintenanceStandin(t, m, nil)
	ctx := context.Background()
	if next, err := d.NextMaintenance(ctx); err != nil || !next.Equal(noon) {
		t.Errorf("with none run: %v, %v; want now", next, err)
	}
	last := noon.Add(-48 * time.Hour)
	if _, err := d.writeMarker(ctx, DataCheck{Host: "example-desktop", Started: last, Finished: last, Slice: 1, OK: true}); err != nil {
		t.Fatal(err)
	}
	if next, err := d.NextMaintenance(ctx); err != nil || !next.Equal(last.Add(maintenanceInterval)) {
		t.Errorf("two days after one: %v, %v; want five days on", next, err)
	}
	at(m, noon.Add(maintenanceInterval))
	if next, err := d.NextMaintenance(ctx); err != nil || !next.Equal(noon.Add(maintenanceInterval)) {
		t.Errorf("overdue: %v, %v; want now", next, err)
	}
}

// A marker that can't be read is skipped, not a failure, and a finished
// run's cleanup removes it and counts it once.
func TestAnUnreadableMarkerIsSkippedThenRemoved(t *testing.T) {
	m := testMachine(t, "example-laptop")
	at(m, noon)
	d, _ := maintenanceStandin(t, m, nil)
	bad := filepath.Join(filepath.Dir(m.Restic), "markers", "badbadbadbadbad0")
	if err := os.WriteFile(bad, []byte(`{"id":"badbadbadbadbad0","tags":["ctdev-maintenance","ctdev-maintenance-data:not*base64"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ms, unreadable, err := d.markers(ctx)
	if err != nil || len(ms) != 0 || len(unreadable) != 1 || unreadable[0] != "badbadbadbadbad0" {
		t.Fatalf("markers = %+v, unreadable %v, err %v; want one unreadable", ms, unreadable, err)
	}
	res := m.maintainIfDue(ctx, d, nil)
	if !res.Ran || res.Err != nil || res.RemovedUnreadable != 1 {
		t.Errorf("result = %+v, want a run that removed the unreadable marker", res)
	}
	if _, unreadable, _ := d.markers(ctx); len(unreadable) != 0 {
		t.Errorf("unreadable markers left: %v", unreadable)
	}
}

// A data check more than two weeks old, or none two weeks after the first
// folder here went on the destination, is overdue, in doctor's facts and
// in the status.
func TestADataCheckTwoWeeksOldIsOverdue(t *testing.T) {
	m := testMachine(t, "example-laptop")
	m.Notifier = &fakeNotifier{}
	at(m, noon)
	d, _ := maintenanceStandin(t, m, nil)
	d.computerKey = "example-computer-key"
	if err := m.save(d); err != nil {
		t.Fatal(err)
	}
	root := exampleFolder(t, m)
	if err := m.Configure(context.Background(), d, root, NewConfig(d.Name)); err != nil {
		t.Fatal(err)
	}
	overdue := func(when time.Time) (doctor, status bool, checks []DestinationCheck) {
		t.Helper()
		at(m, when)
		checks, err := m.DataChecks()
		if err != nil {
			t.Fatal(err)
		}
		s := m.Status(context.Background(), root)
		if s.ReadErr != nil || !s.DataCheckRead {
			t.Fatalf("status: read %v, err %v", s.DataCheckRead, s.ReadErr)
		}
		return len(checks) == 1 && checks[0].Overdue, s.DataCheckOverdue, checks
	}
	if doc, st, checks := overdue(noon.Add(dataCheckStale)); doc || st || len(checks) != 0 {
		t.Errorf("two weeks after it was added, none run: doctor %v %+v, status %v; want not yet overdue, no row", doc, checks, st)
	}
	if doc, st, checks := overdue(noon.Add(dataCheckStale + time.Hour)); !doc || !st || checks[0].Check != nil {
		t.Errorf("past two weeks, none run: doctor %v, status %v; want overdue, never run", doc, st)
	}

	ran := noon.Add(20 * 24 * time.Hour)
	if _, err := d.writeMarker(context.Background(), DataCheck{Host: "example-desktop", Started: ran, Finished: ran, Slice: 1, OK: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.recordDataCheck(d.Name, DataCheck{Host: "example-desktop", Started: ran, Finished: ran, Slice: 1, OK: true}); err != nil {
		t.Fatal(err)
	}
	if doc, st, _ := overdue(ran.Add(dataCheckStale)); doc || st {
		t.Errorf("two weeks after a check: doctor %v, status %v; want not overdue", doc, st)
	}
	if doc, st, _ := overdue(ran.Add(dataCheckStale + time.Hour)); !doc || !st {
		t.Errorf("past two weeks after a check: doctor %v, status %v; want overdue", doc, st)
	}
}

// A claim by another computer with the same hostname is still another
// computer's: this one knows its own claims by ID.
func TestAClaimUnderTheSameHostnameIsAnotherComputers(t *testing.T) {
	m := testMachine(t, "example-laptop")
	at(m, noon)
	d, _ := maintenanceStandin(t, m, nil)
	if _, err := d.writeMarker(context.Background(), DataCheck{Host: m.Hostname, Started: noon.Add(-time.Hour), Slice: 1}); err != nil {
		t.Fatal(err)
	}
	if res := m.maintainIfDue(context.Background(), d, nil); res.Ran || res.Err != nil {
		t.Errorf("result = %+v, want it left to the other computer", res)
	}
}

// folders.json records each folder's destination, so doctor knows which
// data checks to show: Configure writes it, and a scheduled run fills it in
// for an entry written without it, as v17.0.0 and CTD-121 wrote them.
func TestTheScheduleListRecordsEachFoldersDestination(t *testing.T) {
	m := testMachine(t, "example-laptop")
	m.Notifier = &fakeNotifier{}
	at(m, noon)
	d, _ := maintenanceStandin(t, m, nil)
	root := exampleFolder(t, m)
	if err := m.Configure(context.Background(), d, root, NewConfig(d.Name)); err != nil {
		t.Fatal(err)
	}
	destinationOf := func(root string) string {
		t.Helper()
		folders, err := m.scheduledFolders()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range folders {
			if e.Path == root {
				return e.Destination
			}
		}
		t.Fatalf("%s isn't listed", root)
		return ""
	}
	if got := destinationOf(root); got != d.Name {
		t.Errorf("after Configure: destination %q, want %q", got, d.Name)
	}

	old := filepath.Join(m.Home, "Documents", "notes")
	listed(t, m, old, NewConfig("example-other")) // as the old ctdev listed it
	if got := destinationOf(old); got != "" {
		t.Fatalf("an old entry already has destination %q", got)
	}
	_, _, _ = m.RunScheduled(context.Background())
	if got := destinationOf(old); got != "example-other" {
		t.Errorf("after a scheduled run: destination %q, want example-other", got)
	}
}

// Once the maintenance has run, its result stands: a failed check is
// reported as failed even when removing the old markers then finds the
// destination locked, not as waiting for the next hour.
func TestAFinishedRunIsntReportedAsBusy(t *testing.T) {
	m := testMachine(t, "example-laptop")
	at(m, noon)
	d, _ := maintenanceStandin(t, m, map[string]string{"check": "1", "forget-marker": "11"})

	res := m.maintainIfDue(context.Background(), d, nil)
	if !res.Ran || res.Busy || res.Check == nil || res.Check.Step != StepCheck {
		t.Fatalf("result = %+v, want the check's failure", res)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "data check failed at check") ||
		!errors.Is(res.Err, ErrDestinationBusy) {
		t.Errorf("err = %v, want the failed check and the locked cleanup", res.Err)
	}
}

// A long reason is cut on a rune boundary, so the record stays valid.
func TestALongReasonIsCutOnARuneBoundary(t *testing.T) {
	m := testMachine(t, "example-laptop")
	d, _ := maintenanceStandin(t, m, nil)
	reason := strings.Repeat("a", maxMarkerError-1) + "é and more"
	if _, err := d.writeMarker(context.Background(), DataCheck{Host: "example-laptop", Started: noon, Finished: noon, Slice: 1, Step: StepCheck, Error: reason}); err != nil {
		t.Fatal(err)
	}
	ms, skipped, err := d.markers(context.Background())
	if err != nil || len(skipped) != 0 || len(ms) != 1 {
		t.Fatalf("markers = %+v, skipped %v, err %v", ms, skipped, err)
	}
	if want := strings.Repeat("a", maxMarkerError-1) + "…"; ms[0].Error != want {
		t.Errorf("reason = %q, want it cut before the é", ms[0].Error)
	}
}
