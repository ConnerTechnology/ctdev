package backup

import (
	"errors"
	"testing"
	"time"
)

// at is a fixed clock for m.
func at(m *Machine, t time.Time) { m.Now = func() time.Time { return t } }

var noon = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

func TestDueAfterOneIntervalSinceTheLastSuccess(t *testing.T) {
	for name, tc := range map[string]struct {
		schedule Schedule
		since    time.Duration // since the last success; 0 means never
		want     bool
	}{
		"never backed up":         {Daily, 0, true},
		"hourly, 59 minutes ago":  {Hourly, 59 * time.Minute, false},
		"hourly, an hour ago":     {Hourly, time.Hour, true},
		"daily, 23 hours ago":     {Daily, 23 * time.Hour, false},
		"daily, exactly 24h ago":  {Daily, 24 * time.Hour, true},
		"weekly, 6 days ago":      {Weekly, 6 * 24 * time.Hour, false},
		"weekly, 7 days ago":      {Weekly, 7 * 24 * time.Hour, true},
		"daily, three days ago":   {Daily, 72 * time.Hour, true},
		"weekly, a day ago":       {Weekly, 24 * time.Hour, false},
		"hourly, two minutes ago": {Hourly, 2 * time.Minute, false},
	} {
		var last Run
		if tc.since > 0 {
			last.LastSuccess = noon.Add(-tc.since)
		}
		if got := Due(tc.schedule, last, noon); got != tc.want {
			t.Errorf("%s: Due = %v, want %v", name, got, tc.want)
		}
	}
}

// health writes a folders.json and state.json for m and returns its Health.
func health(t *testing.T, m *Machine) []FolderHealth {
	t.Helper()
	h, err := m.Health()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestHealthJudgesEachScheduledFolder(t *testing.T) {
	m := testMachine(t, "example-laptop")
	added := noon.Add(-30 * 24 * time.Hour)
	at(m, added)
	folders := map[string]Schedule{
		"/example/ok":             Daily,
		"/example/failed":         Daily,
		"/example/overdue":        Daily,
		"/example/just-in-time":   Daily,
		"/example/barely-late":    Hourly,
		"/example/missing":        Weekly,
		"/example/never-new":      Daily,
		"/example/never-old":      Weekly,
		"/example/failed-and-old": Hourly,
	}
	for path, s := range folders {
		if err := m.addFolder(path, newUUID(), s); err != nil {
			t.Fatal(err)
		}
	}
	record := func(path string, when time.Time, err error) {
		t.Helper()
		if rerr := m.recordRun(path, when, err); rerr != nil {
			t.Fatal(rerr)
		}
	}
	record("/example/ok", noon.Add(-30*time.Hour), nil)    // daily, inside twice the interval
	record("/example/failed", noon.Add(-2*time.Hour), nil) // succeeded, then...
	record("/example/failed", noon.Add(-time.Hour), errors.New("example failure"))
	record("/example/overdue", noon.Add(-49*time.Hour), nil)      // daily, no success in 48h
	record("/example/just-in-time", noon.Add(-48*time.Hour), nil) // daily, success exactly 48h ago
	record("/example/barely-late", noon.Add(-(2*time.Hour + time.Second)), nil)
	record("/example/failed-and-old", noon.Add(-72*time.Hour), errors.New("example failure"))
	if err := m.recordMissing("/example/missing", noon.Add(-time.Hour), "the folder is gone"); err != nil {
		t.Fatal(err)
	}
	// never-new was added a day ago; never-old 30 days ago, weekly.
	at(m, noon.Add(-24*time.Hour))
	if err := m.addFolder("/example/never-new", newUUID(), Daily); err != nil {
		t.Fatal(err)
	}
	at(m, noon)

	want := map[string]HealthState{
		"/example/ok":             HealthOK,
		"/example/failed":         HealthFailed,
		"/example/overdue":        HealthOverdue,
		"/example/just-in-time":   HealthOK,
		"/example/barely-late":    HealthOverdue,
		"/example/missing":        HealthMissing,
		"/example/never-new":      HealthOK,
		"/example/never-old":      HealthOverdue,
		"/example/failed-and-old": HealthFailed,
	}
	got := map[string]HealthState{}
	for _, f := range health(t, m) {
		got[f.Path] = f.State
		if f.State == HealthFailed && f.Run.Error != "example failure" {
			t.Errorf("%s: error %q, want the run's error", f.Path, f.Run.Error)
		}
	}
	for path, w := range want {
		if got[path] != w {
			t.Errorf("%s: %q, want %q", path, got[path], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("health lists %d folders, want %d", len(got), len(want))
	}
}

// Re-adding a folder (reconfiguring it) keeps when it was first added, so a
// reconfigure can't hide a folder that has never backed up.
func TestReconfigureKeepsWhenTheFolderWasAdded(t *testing.T) {
	m := testMachine(t, "example-laptop")
	id := "00000000-0000-4000-8000-000000000000"
	at(m, noon.Add(-10*24*time.Hour))
	if err := m.addFolder("/example/a", id, Daily); err != nil {
		t.Fatal(err)
	}
	at(m, noon)
	if err := m.addFolder("/example/a", id, Weekly); err != nil {
		t.Fatal(err)
	}
	h := health(t, m)
	if len(h) != 1 || h[0].Schedule != Weekly || h[0].State != HealthOK {
		t.Fatalf("health = %+v, want weekly and OK (added 10 days ago)", h)
	}
	at(m, noon.Add(5*24*time.Hour))
	if h := health(t, m); h[0].State != HealthOverdue {
		t.Errorf("15 days after it was added, never backed up: %q, want overdue", h[0].State)
	}
}

func TestHealthWithNothingScheduled(t *testing.T) {
	m := testMachine(t, "example-laptop")
	if h := health(t, m); len(h) != 0 {
		t.Errorf("health = %+v, want none", h)
	}
}
