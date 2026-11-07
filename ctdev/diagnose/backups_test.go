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
	} {
		got, ok := rows[name]
		if !ok {
			t.Errorf("no row for %s", name)
			continue
		}
		if got.Severity != want.severity || got.Detail != want.detail {
			t.Errorf("%s: %v %q, want %v %q", name, got.Severity, got.Detail, want.severity, want.detail)
		}
		if got.Severity == Fail && !strings.Contains(got.Advice, "ctdev backup") {
			t.Errorf("%s: advice %q doesn't say what to run", name, got.Advice)
		}
	}
	if len(rows) != 5 {
		t.Errorf("%d backup rows, want 5", len(rows))
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
