package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/spf13/cobra"
)

const (
	resticBackupScript  = "/usr/local/bin/restic-backup.sh"
	resticRestoreScript = "/usr/local/bin/restic-restore.sh"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Back up this machine with restic",
	Long: "Snapshot this machine to its configured restic repository.\n\n" +
		"  ctdev backup now              run a snapshot now\n" +
		"  ctdev backup snapshots        list this machine's snapshots\n" +
		"  ctdev backup paths            pick what to back up in a local web UI\n\n" +
		"Configure the repository and credentials first with 'ctdev configure restic'. " +
		"Restore with 'ctdev restore'.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("ctdev backup no longer takes a service name; use 'ctdev backup now' or 'ctdev backup snapshots'")
		}
		return cmd.Help()
	},
}

var backupNowCmd = &cobra.Command{
	Use:   "now",
	Short: "Run a restic snapshot now",
	Long:  "Snapshot this machine's configured paths (" + component.ResticPathsFile + ") to its restic repository, then prune. Requires 'ctdev install restic' and 'ctdev configure restic'.",
	RunE:  runBackupNow,
}

var backupSnapshotsCmd = &cobra.Command{
	Use:   "snapshots [primary|local]",
	Short: "List this machine's restic snapshots",
	Long:  "List restic snapshots for this machine (newest last) from the primary repo (default) or the optional second repo. Requires 'ctdev configure restic'.",
	Args:  cobra.MatchAll(cobra.MaximumNArgs(1), repoArgAt(0)),
	RunE:  runBackupSnapshots,
}

// repoArgAt validates the optional trailing [primary|local] argument at idx, so
// a typo errors in ctdev instead of two layers down in the shell script.
func repoArgAt(idx int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > idx && args[idx] != "primary" && args[idx] != "local" {
			return fmt.Errorf("unknown repo %q (use primary or local)", args[idx])
		}
		return nil
	}
}

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore this machine from restic",
	Long: "Inspect and restore from this machine's restic repository.\n\n" +
		"  ctdev restore ls <snap|latest> [primary|local]            list files in a snapshot\n" +
		"  ctdev restore files <snap|latest> <dir> [primary|local]   restore a snapshot into <dir> (safe)\n" +
		"  ctdev restore in-place <snap|latest> [primary|local]      restore to original paths (DANGER)\n" +
		"  ctdev restore check [primary|local]                       verify repository integrity\n\n" +
		"List snapshots with 'ctdev backup snapshots'. See RECOVERY.md for the full runbook.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Unknown verbs fall through to this parent RunE; error instead of
		// printing help with exit 0.
		if len(args) > 0 {
			return fmt.Errorf("unknown restore subcommand %q (use ls, files, in-place, check)", args[0])
		}
		return cmd.Help()
	},
}

var restoreLsCmd = &cobra.Command{
	Use:   "ls <snapshot|latest> [primary|local]",
	Short: "List files in a snapshot",
	Args:  cobra.MatchAll(cobra.RangeArgs(1, 2), repoArgAt(1)),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runResticRestore(cmd, append([]string{"ls"}, args...))
	},
}

var restoreFilesCmd = &cobra.Command{
	Use:   "files <snapshot|latest> <dir> [primary|local]",
	Short: "Restore a snapshot into a directory (safe)",
	Args:  cobra.MatchAll(cobra.RangeArgs(2, 3), repoArgAt(2)),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runResticRestore(cmd, append([]string{"restore"}, args...))
	},
}

var restoreInPlaceCmd = &cobra.Command{
	Use:   "in-place <snapshot|latest> [primary|local]",
	Short: "Restore to original paths (DANGER — overwrites live files)",
	Args:  cobra.MatchAll(cobra.RangeArgs(1, 2), repoArgAt(1)),
	RunE: func(cmd *cobra.Command, args []string) error {
		// The overwrite confirmation must live HERE: sysutil runs children
		// without stdin, so the script's own "Type YES" prompt reads EOF and
		// aborts unconditionally. ctdev confirms, then passes --yes.
		if !flagDryRun {
			fmt.Println("Restoring to ORIGINAL paths — this overwrites live files at their")
			fmt.Println("absolute locations. Stop any affected services/stacks first.")
			fmt.Print("Type YES to continue: ")
			line, ok := readLineCtx(cmdContext(cmd))
			if !ok {
				return cancelToClean(errPromptCancelled)
			}
			if line != "YES" {
				fmt.Println("aborted — nothing restored")
				return nil
			}
		}
		return runResticRestore(cmd, append([]string{"restore-in-place", "--yes"}, args...))
	},
}

var restoreCheckCmd = &cobra.Command{
	Use:   "check [primary|local]",
	Short: "Verify repository integrity",
	Args:  cobra.MatchAll(cobra.MaximumNArgs(1), repoArgAt(0)),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runResticRestore(cmd, append([]string{"check"}, args...))
	},
}

var backupDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Pause scheduled backups (config + snapshots kept)",
	Long: "Stop and disable the nightly backup timer (and the monthly integrity check), so no " +
		"scheduled runs happen. Your configuration and all existing snapshots are kept — you can " +
		"still run a one-off with 'ctdev backup now', and re-enable scheduling with 'ctdev backup " +
		"enable'.",
	RunE: runBackupDisable,
}

var backupEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Resume scheduled backups",
	Long:  "Enable and start the nightly backup timer and the monthly integrity check. Requires restic to be installed and configured.",
	RunE:  runBackupEnable,
}

func init() {
	backupCmd.AddCommand(backupNowCmd, backupSnapshotsCmd, backupPathsCmd, backupDisableCmd, backupEnableCmd)
	restoreCmd.AddCommand(restoreLsCmd, restoreFilesCmd, restoreInPlaceCmd, restoreCheckCmd)
	rootCmd.AddCommand(backupCmd, restoreCmd)
}

func runBackupDisable(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	if !pathExists(resticBackupScript) {
		return fmt.Errorf("restic is not installed on this machine (nothing to disable)")
	}
	if flagDryRun {
		fmt.Println("[dry-run] would disable restic-backup.timer and restic-check.timer")
		return nil
	}
	if err := ensureSudo(cmdContext(cmd)); err != nil {
		return fmt.Errorf("sudo required: %w", err)
	}
	if err := component.ResticDisableTimer(ctx, o); err != nil {
		return fmt.Errorf("disable backup timer: %w", err)
	}
	fmt.Println("Scheduled backups paused — no nightly runs.")
	fmt.Println("  Config and snapshots are kept. Run a one-off any time: ctdev backup now")
	fmt.Println("  Resume scheduled backups: ctdev backup enable")
	return nil
}

func runBackupEnable(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	// Installed + configured (else the timer would just fail every night).
	if err := ensureResticReady(ctx, o, resticBackupScript); err != nil {
		return err
	}
	if flagDryRun {
		fmt.Println("[dry-run] would enable restic-backup.timer and restic-check.timer")
		return nil
	}
	if err := component.ResticEnableTimer(ctx, o); err != nil {
		return fmt.Errorf("enable backup timer: %w", err)
	}
	fmt.Println("Scheduled backups enabled — nightly snapshot + monthly integrity check.")
	return nil
}

func runBackupNow(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	if err := ensureResticReady(ctx, o, resticBackupScript); err != nil {
		return err
	}
	fmt.Println("Running restic backup...")
	return sysutil.SudoRun(ctx, o, resticBackupScript)
}

func runBackupSnapshots(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)
	o := sysutil.Opts{Stdout: os.Stdout}
	if err := ensureResticReady(ctx, o, resticRestoreScript); err != nil {
		return err
	}
	return sysutil.SudoRun(ctx, o, resticRestoreScript, append([]string{"snapshots"}, args...)...)
}

// runResticRestore runs the restore helper with the given verb/args after the
// install + config prechecks.
func runResticRestore(cmd *cobra.Command, scriptArgs []string) error {
	ctx := cmdContext(cmd)
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	if err := ensureResticReady(ctx, o, resticRestoreScript); err != nil {
		return err
	}
	return sysutil.SudoRun(ctx, o, resticRestoreScript, scriptArgs...)
}

// ensureResticReady verifies restic is installed and configured on this node and
// that sudo is available, returning an actionable error otherwise — so the user
// gets a clean "run X" message instead of a failure from inside the shell script.
func ensureResticReady(ctx context.Context, o sysutil.Opts, scriptPath string) error {
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("restic is not installed on this machine (run: ctdev install restic)")
	}
	if flagDryRun {
		// Nothing will execute — don't prompt for a sudo password to preview.
		return nil
	}
	if err := ensureSudo(ctx); err != nil {
		return fmt.Errorf("sudo required: %w", err)
	}
	if !component.ResticConfigured(ctx, o) {
		return fmt.Errorf("restic is not configured on this machine (run: ctdev configure restic)")
	}
	return nil
}
