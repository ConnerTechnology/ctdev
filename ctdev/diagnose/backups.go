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
	// RetentionError is why its keep rules didn't apply after its last
	// successful backup. The backup still counts as succeeded.
	RetentionError string
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
	checks := make([]Check, 0, len(f.Backups)+len(f.BackupDataChecks))
	for _, b := range f.Backups {
		checks = append(checks, Check{
			ID: "backup." + b.Path, Name: b.Path, Group: GroupBackups,
			Run: func(context.Context, Facts) Result { return backupResult(b) },
		})
	}
	for _, c := range f.BackupDataChecks {
		checks = append(checks, Check{
			ID: "backup.datacheck." + c.Destination, Name: "Data check: " + c.Destination, Group: GroupBackups,
			Run: func(context.Context, Facts) Result { return dataCheckResult(c) },
		})
	}
	return checks
}

// BackupDataCheck is a backup destination's last weekly data check, which
// whichever computer found it due ran and recorded in the destination.
type BackupDataCheck struct {
	Destination string
	Finished    time.Time
	Host        string // the computer that ran it
	Slice       int    // the part of the stored data it read
	Slices      int    // out of how many
	OK          bool
	Step        string // the step that failed
	Error       string // why
}

func dataCheckResult(c BackupDataCheck) Result {
	day := c.Finished.Local().Format("Jan 2")
	if !c.OK {
		var advice string
		switch c.Step {
		case "check":
			advice = "Some stored data in " + c.Destination + " is damaged, so backups that use it may not restore. restic's troubleshooting guide covers repairing damaged packs; the weekly maintenance checks again a week after this one."
		case "test download":
			advice = "A file from a backup in " + c.Destination + " didn't read back whole, so that backup may not restore. Download it from 'ctdev backup' → List backups to see; the weekly maintenance tries again a week after this one."
		default:
			advice = "Freeing deleted backups' space in " + c.Destination + " failed, so the destination keeps it for now. The weekly maintenance tries again a week after this one."
		}
		return failf(advice, "%s · failed at %s: %s", day, c.Step, c.Error)
	}
	return okf("%s · OK · read slice %d of %d · by %s", day, c.Slice, c.Slices, c.Host)
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
	detail := b.Schedule + " · no backup yet"
	if !b.LastSuccess.IsZero() {
		detail = b.Schedule + " · last backup " + backupTime(b.LastSuccess)
	}
	if b.RetentionError != "" {
		return warnf("Older backups weren't thinned. Run 'ctdev backup' in that folder and choose Back up now to apply the keep rules again.",
			"%s · retention failed: %s", detail, b.RetentionError)
	}
	return okf("%s", detail)
}

func backupTime(t time.Time) string { return t.Local().Format("Jan 2, 3:04 PM") }
