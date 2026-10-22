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
	markerList := filepath.Join(dir, "markers.json")
	code := func(cmd string) string {
		if c, ok := fail[cmd]; ok {
			return `echo "Fatal: example failure" >&2; exit ` + c
		}
		return ":"
	}
	m.Restic = filepath.Join(dir, "restic")
	script := `#!/bin/sh
echo "$*" >> "` + log + `"
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
backup)
  for a in "$@"; do case "$a" in ctdev-maintenance-data:*) tag="$a" ;; esac; done
  echo '[{"id":"c1a1c1a1c1a1c1a1","tags":["ctdev-maintenance","'"$tag"'"]}]' > "` + markerList + `"
  echo '{"message_type":"summary","snapshot_id":"c1a1c1a1c1a1c1a1"}' ;;
snapshots)
  case "$*" in
  *ctdev-maintenance*) if [ -e "` + markerList + `" ]; then cat "` + markerList + `"; else echo '[]'; fi ;;
  *) echo '[]' ;;
  esac ;;
forget)
  case "$*" in
  *ctdev-folder:*) ` + code("forget-folder") + ` ;;
  *) rm -f "` + markerList + `" ;;
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
			if checks, _ := m.destinationStates(); len(checks) != 0 {
				t.Errorf("state.json keeps %+v, want nothing", checks)
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

// A marker that can't be read is skipped and counted, not a failure.
func TestAnUnreadableMarkerIsSkipped(t *testing.T) {
	m := testMachine(t, "example-laptop")
	at(m, noon)
	d, _ := maintenanceStandin(t, m, nil)
	if _, err := d.writeMarker(context.Background(), DataCheck{Host: "example-desktop", Started: noon, Finished: noon, Slice: 1, OK: true}); err != nil {
		t.Fatal(err)
	}
	// Replace the record with one that isn't base64 JSON.
	list := filepath.Join(filepath.Dir(m.Restic), "markers.json")
	if err := os.WriteFile(list, []byte(`[{"id":"c1a1c1a1c1a1c1a1","tags":["ctdev-maintenance","ctdev-maintenance-data:not*base64"]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	ms, skipped, err := d.markers(context.Background())
	if err != nil || len(ms) != 0 || skipped != 1 {
		t.Errorf("markers = %+v, skipped %d, err %v; want one skipped", ms, skipped, err)
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
	if err != nil || skipped != 0 || len(ms) != 1 {
		t.Fatalf("markers = %+v, skipped %d, err %v", ms, skipped, err)
	}
	if want := strings.Repeat("a", maxMarkerError-1) + "…"; ms[0].Error != want {
		t.Errorf("reason = %q, want it cut before the é", ms[0].Error)
	}
}
