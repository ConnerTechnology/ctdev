//go:build unix

package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
