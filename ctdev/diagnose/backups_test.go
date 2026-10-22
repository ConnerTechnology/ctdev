package diagnose

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

func backupRows(t *testing.T, f Facts) map[string]Result {
	t.Helper()
	rows := map[string]Result{}
	for _, c := range Catalog(f.Platform, f) {
		if c.Group != GroupBackups {
			continue
		}
		if c.Network || c.Deep {
			t.Errorf("%s: a backup row must be local and quick", c.ID)
		}
		rows[c.Name] = c.Run(context.Background(), f)
	}
	return rows
}

func TestBackupsSectionShowsEachScheduledFolder(t *testing.T) {
	last := time.Date(2026, 10, 21, 21, 12, 0, 0, time.Local)
	f := Facts{Platform: platform.Info{OS: platform.Linux}, Backups: []BackupFolder{
		{Path: "~/Projects/website", Schedule: "daily", State: BackupOK, LastSuccess: last},
		{Path: "~/Documents/new", Schedule: "hourly", State: BackupOK},
		{Path: "~/Documents/taxes", Schedule: "daily", State: BackupFailed, LastSuccess: last, Error: "example failure"},
		{Path: "~/Photos", Schedule: "weekly", State: BackupOverdue, LastSuccess: last},
		{Path: "~/Old", Schedule: "daily", State: BackupMissing, Error: "the folder isn't there any more"},
		{Path: "~/Music", Schedule: "daily", State: BackupOK, LastSuccess: last, RetentionError: "example failure"},
	}}
	rows := backupRows(t, f)
	for name, want := range map[string]struct {
		severity Severity
		detail   string
	}{
		"~/Projects/website": {OK, "daily · last backup Oct 21, 9:12 PM"},
		"~/Documents/new":    {OK, "hourly · no backup yet"},
		"~/Documents/taxes":  {Fail, "failed: example failure"},
		"~/Photos":           {Fail, "overdue: weekly, no backup since Oct 21, 9:12 PM"},
		"~/Old":              {Fail, "missing: the folder isn't there any more"},
		"~/Music":            {Warn, "daily · last backup Oct 21, 9:12 PM · retention failed: example failure"},
	} {
		got, ok := rows[name]
		if !ok {
			t.Errorf("no row for %s", name)
			continue
		}
		if got.Severity != want.severity || got.Detail != want.detail {
			t.Errorf("%s: %v %q, want %v %q", name, got.Severity, got.Detail, want.severity, want.detail)
		}
		if got.Severity != OK && !strings.Contains(got.Advice, "ctdev backup") {
			t.Errorf("%s: advice %q doesn't say what to run", name, got.Advice)
		}
	}
	if len(rows) != 6 {
		t.Errorf("%d backup rows, want 6", len(rows))
	}
}

// Each destination's weekly data check is a row: OK with its date and
// slice, or failed with the step and reason.
func TestBackupsSectionShowsEachDestinationsDataCheck(t *testing.T) {
	at := time.Date(2026, 10, 20, 3, 0, 0, 0, time.Local)
	f := Facts{Platform: platform.Info{OS: platform.Linux}, BackupDataChecks: []BackupDataCheck{
		{Destination: "example-backups", Finished: at, Host: "example-desktop", Slice: 3, Slices: 20, OK: true},
		{Destination: "example-usb", Finished: at, Host: "example-laptop", Slice: 7, Slices: 20, Step: "check", Error: "damaged pack 0123abcd"},
	}}
	rows := backupRows(t, f)
	for name, want := range map[string]struct {
		severity Severity
		detail   string
	}{
		"Data check: example-backups": {OK, "Oct 20 · OK · read slice 3 of 20 · by example-desktop"},
		"Data check: example-usb":     {Fail, "Oct 20 · failed at check: damaged pack 0123abcd"},
	} {
		got, ok := rows[name]
		if !ok {
			t.Errorf("no row for %s in %v", name, rows)
			continue
		}
		if got.Severity != want.severity || got.Detail != want.detail {
			t.Errorf("%s: %v %q, want %v %q", name, got.Severity, got.Detail, want.severity, want.detail)
		}
		if got.Severity == Fail && got.Advice == "" {
			t.Errorf("%s: no advice", name)
		}
	}
}

// A failed data check's advice fits the step that failed: only a failed
// check talks about damaged packs.
func TestDataCheckAdviceFitsTheStep(t *testing.T) {
	at := time.Date(2026, 10, 20, 3, 0, 0, 0, time.Local)
	for step, want := range map[string]string{
		"check":         "damaged packs",
		"test download": "didn't read back",
		"prune":         "space",
	} {
		r := dataCheckResult(BackupDataCheck{Destination: "example-backups", Finished: at, Slice: 3, Slices: 20, Step: step, Error: "example failure"})
		if r.Severity != Fail || !strings.Contains(r.Advice, want) {
			t.Errorf("%s: %v %q, want advice about %q", step, r.Severity, r.Advice, want)
		}
		if step != "check" && strings.Contains(r.Advice, "damaged") {
			t.Errorf("%s: advice %q talks about damage", step, r.Advice)
		}
	}
}

// A machine with nothing scheduled has no Backups section at all.
func TestNoBackupsSectionWithNothingScheduled(t *testing.T) {
	if rows := backupRows(t, Facts{Platform: platform.Info{OS: platform.MacOS}}); len(rows) != 0 {
		t.Errorf("rows = %v, want none", rows)
	}
}

// State that can't be read is one skipped row, not a missing section.
func TestBackupStateThatCantBeReadIsSkipped(t *testing.T) {
	rows := backupRows(t, Facts{Platform: platform.Info{OS: platform.Linux}, BackupsErr: "state.json: example error"})
	got, ok := rows["Backup state"]
	if len(rows) != 1 || !ok || got.Severity != Skipped || !strings.Contains(got.Detail, "example error") {
		t.Errorf("rows = %v, want one skipped Backup state row", rows)
	}
}

func TestBackupsGroupRendersBeforeTools(t *testing.T) {
	var backups, tools int
	for i, g := range GroupOrder {
		switch g {
		case GroupBackups:
			backups = i
		case GroupTools:
			tools = i
		}
	}
	if backups == 0 || backups > tools {
		t.Errorf("GroupOrder = %v, want Backups listed, before Tools", GroupOrder)
	}
}
