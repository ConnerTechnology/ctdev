package diagnose

import (
	"context"
	"time"
)

// GroupBackups is the folders ctdev backs up on a schedule on this machine.
const GroupBackups = "Backups"

// BackupState is how a scheduled folder's backups are doing on this machine.
type BackupState string

const (
	BackupOK      BackupState = "ok"
	BackupFailed  BackupState = "failed"
	BackupOverdue BackupState = "overdue"
	BackupMissing BackupState = "missing"
)

// BackupFolder is one folder on this machine's backup schedule, as ctdev
// doctor's caller judged it from ~/.config/ctdev/backup. This package never
// reads that itself and never imports package backup: the facts arrive filled.
type BackupFolder struct {
	Path        string // as the user would type it, ~/…
	Schedule    string // hourly, daily or weekly
	State       BackupState
	LastSuccess time.Time // zero when it has never backed up
	Error       string    // why it failed, or why it's missing
}

// backupChecks is a row per scheduled folder. Every one is local: the facts
// already hold the answer, so nothing here touches the network or needs root.
func backupChecks(f Facts) []Check {
	if f.BackupsErr != "" {
		return []Check{{
			ID: "backup.state", Name: "Backup state", Group: GroupBackups,
			Run: func(context.Context, Facts) Result {
				return skipf("couldn't read ctdev's backup state: %s", f.BackupsErr)
			},
		}}
	}
	checks := make([]Check, 0, len(f.Backups))
	for _, b := range f.Backups {
		checks = append(checks, Check{
			ID: "backup." + b.Path, Name: b.Path, Group: GroupBackups,
			Run: func(context.Context, Facts) Result { return backupResult(b) },
		})
	}
	return checks
}

func backupResult(b BackupFolder) Result {
	const look = "Run 'ctdev backup' in that folder to see why, and choose Back up now."
	switch b.State {
	case BackupFailed:
		return failf(look, "failed: %s", b.Error)
	case BackupOverdue:
		since := "never backed up"
		if !b.LastSuccess.IsZero() {
			since = "no backup since " + backupTime(b.LastSuccess)
		}
		return failf("Run 'ctdev backup' in that folder: it says when the schedule isn't installed or the last backup failed.",
			"overdue: %s, %s", b.Schedule, since)
	case BackupMissing:
		return failf("Move the folder back, or run 'ctdev backup' in its new place and choose Configure.",
			"missing: %s", b.Error)
	}
	if b.LastSuccess.IsZero() {
		return okf("%s · no backup yet", b.Schedule)
	}
	return okf("%s · last backup %s", b.Schedule, backupTime(b.LastSuccess))
}

func backupTime(t time.Time) string { return t.Local().Format("Jan 2, 3:04 PM") }
