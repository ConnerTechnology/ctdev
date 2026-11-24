package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupapp"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

// backupCmd is per-folder backup (CTD-87): the full-screen program in
// tui/backupapp in a terminal, the status alone without one.
var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Back up the folder you're in",
	Long: "Show whether the folder you're in is backed up, on a full-screen dashboard with its actions. " +
		"ctdev looks up from the current directory for the nearest .ctdev-backup.json, the way " +
		"git finds its repository. Without a terminal, or with --batch, it prints the status " +
		"only, and exits non-zero when the folder isn't backed up or scheduled, or its last backup failed " +
		"or is overdue.\n\n" +
		"With --all, from any directory: every backed-up folder in the destination, from every " +
		"computer, grouped by computer, to list and download; the computers with access, to remove " +
		"one or recover from a lost one; and the Emergency Kit and master password. On a computer with no destination, " +
		"it first connects to one with its Emergency Kit and master password, or creates one.",
	Args: cobra.NoArgs,
	RunE: runBackup,
}

// backupRunScheduledCmd is what the hourly systemd user timer or launchd
// agent runs. It's hidden: people back up with ctdev backup.
var backupRunScheduledCmd = &cobra.Command{
	Use:    "run-scheduled",
	Short:  "Back up the scheduled folders that are due (run hourly by the scheduler)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE:   runBackupScheduled,
}

func init() {
	rootCmd.AddCommand(backupCmd)
	backupCmd.AddCommand(backupRunScheduledCmd)
}

// scheduledNotifier is run-scheduled's notifier; nil is the desktop's.
// Tests set it so they never pop up a real notification.
var scheduledNotifier backup.Notifier

func runBackupScheduled(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	m, err := backup.ThisMachine(sysutil.Opts{Stdout: out, DryRun: flagDryRun})
	if err != nil {
		return err
	}
	m.Notifier = scheduledNotifier
	results, maint, err := m.RunScheduled(cmdContext(cmd))
	for _, r := range results {
		line := backup.DisplayPath(m.Home, r.Path) + ": " + string(r.Outcome)
		switch {
		case r.Err != nil:
			line += ": " + r.Err.Error()
		case r.Outcome == backup.BackedUp && r.Snapshot.ShortID != "":
			line += " as " + r.Snapshot.ShortID
		case r.Outcome == backup.Deferred:
			line += ": the destination is busy, trying again next hour"
		}
		if r.RetentionErr != nil {
			line += ", but retention failed: " + r.RetentionErr.Error()
		}
		fmt.Fprintln(out, line)
	}
	for _, r := range maint {
		if r.RemovedUnreadable > 0 {
			fmt.Fprintf(out, "%s: removed %d maintenance markers that couldn't be read\n", r.Destination, r.RemovedUnreadable)
		}
		// Only a run worth a line in the journal: one this computer made,
		// or one that couldn't be checked for.
		switch {
		case r.Busy:
			fmt.Fprintf(out, "%s: weekly maintenance waiting: the destination is busy, trying again next hour\n", r.Destination)
		case r.Ran && r.Check != nil && r.Check.OK:
			fmt.Fprintf(out, "%s: weekly maintenance done, data check %d/%d OK\n", r.Destination, r.Check.Slice, backup.Slices)
		case r.Ran && r.Check != nil:
			fmt.Fprintf(out, "%s: weekly maintenance %s\n", r.Destination, r.Check.Reason())
		case r.Err != nil:
			fmt.Fprintf(out, "%s: weekly maintenance: %s\n", r.Destination, r.Err)
		}
	}
	return err
}

func runBackup(cmd *cobra.Command, _ []string) error {
	ctx := cmdContext(cmd)
	m, err := backup.ThisMachine(sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun})
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if flagBackupAll {
		return runBackupAll(ctx, m, cwd, cmd.OutOrStdout())
	}
	if isBatchMode() {
		return backupStatusOnly(ctx, m, cwd, cmd.OutOrStdout())
	}
	return backupapp.RunFolder(ctx, backupapp.Deps{Machine: m, Cwd: cwd, Scheduler: machineScheduler{m}})
}

// machineScheduler is this computer's hourly runner for the backup screens.
type machineScheduler struct{ m *backup.Machine }

func (s machineScheduler) Install(ctx context.Context) error { return s.m.InstallScheduler(ctx) }

func (s machineScheduler) RemoveIfIdle(ctx context.Context) (bool, error) {
	return s.m.RemoveSchedulerIfIdle(ctx)
}

// backupStatusOnly prints the status for a pipe or --batch, and fails when
// the folder isn't backed up, its last backup failed or is overdue, or its
// destination can't be reached or read, so it works as a check. This
// computer's own record of a failure or an overdue backup is the more useful
// error, so it comes before the destination's.
func backupStatusOnly(ctx context.Context, m *backup.Machine, cwd string, out io.Writer) error {
	s := m.Status(ctx, cwd)
	text := backupscreen.Status(s, m.Home)
	if f, ok := out.(*os.File); !ok || !term.IsTerminal(f.Fd()) {
		// The plain theme keeps bold for NO_COLOR, but a pipe or a log
		// should get no escape codes at all.
		text = ansi.Strip(text)
	}
	fmt.Fprint(out, text)
	where := backup.DisplayPath(m.Home, s.Target)
	switch {
	case s.ConfigErr != nil:
		return s.ConfigErr
	case s.Config == nil:
		return fmt.Errorf("%s isn't backed up: run ctdev backup in a terminal and choose Configure", where)
	case s.Failed():
		return fmt.Errorf("the last backup of %s failed", where)
	case s.Overdue:
		return fmt.Errorf("the backup of %s is overdue: no success in twice its %s interval", where, s.Config.Schedule)
	case s.DestinationMissing:
		return fmt.Errorf("destination %s isn't set up on this computer: run ctdev backup in a terminal and choose Configure", s.Destination)
	case s.ReadErr != nil:
		return fmt.Errorf("can't read the backups of %s: %w", where, s.ReadErr)
	case !s.Scheduled || !s.SchedulerInstalled:
		return fmt.Errorf("backups for %s are not scheduled on this computer: run ctdev backup in a terminal and choose Configure", where)
	case !s.HasBackups():
		return fmt.Errorf("no backups of %s are stored in %s yet", where, s.Destination)
	}
	return nil
}
