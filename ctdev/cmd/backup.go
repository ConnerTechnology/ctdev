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
	RunE:  runBackupSnapshots,
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
	RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
}

var restoreLsCmd = &cobra.Command{
	Use:   "ls <snapshot|latest> [primary|local]",
	Short: "List files in a snapshot",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runResticRestore(cmd, append([]string{"ls"}, args...))
	},
}

var restoreFilesCmd = &cobra.Command{
	Use:   "files <snapshot|latest> <dir> [primary|local]",
	Short: "Restore a snapshot into a directory (safe)",
	Args:  cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runResticRestore(cmd, append([]string{"restore"}, args...))
	},
}

var restoreInPlaceCmd = &cobra.Command{
	Use:   "in-place <snapshot|latest> [primary|local]",
	Short: "Restore to original paths (DANGER — overwrites live files)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runResticRestore(cmd, append([]string{"restore-in-place"}, args...))
	},
}

var restoreCheckCmd = &cobra.Command{
	Use:   "check [primary|local]",
	Short: "Verify repository integrity",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runResticRestore(cmd, append([]string{"check"}, args...))
	},
}

func init() {
	backupCmd.AddCommand(backupNowCmd, backupSnapshotsCmd, backupPathsCmd)
	restoreCmd.AddCommand(restoreLsCmd, restoreFilesCmd, restoreInPlaceCmd, restoreCheckCmd)
	rootCmd.AddCommand(backupCmd, restoreCmd)
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
	o := sysutil.Opts{Stdout: os.Stdout}
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
	if err := ensureSudo(); err != nil {
		return fmt.Errorf("sudo required: %w", err)
	}
	if !flagDryRun && !component.ResticConfigured(ctx, o) {
		return fmt.Errorf("restic is not configured on this machine (run: ctdev configure restic)")
	}
	return nil
}
