package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/spf13/cobra"
)

// A backupProvider exports a service's version-controllable state to text files
// (to commit) and re-applies it. Today Pi-hole is the only one; the slice keeps
// the door open for more without a heavy framework.
type backupProvider struct {
	name    string
	export  func(ctx context.Context) error
	restore func(ctx context.Context) error
}

func backupProviders() []backupProvider {
	return []backupProvider{
		{
			name:    "pihole",
			export:  func(ctx context.Context) error { return exportPihole(ctx, flagBackupOut) },
			restore: func(ctx context.Context) error { return importPihole(ctx, flagRestoreFrom, flagDryRun) },
		},
	}
}

const (
	resticBackupScript  = "/usr/local/bin/restic-backup.sh"
	resticRestoreScript = "/usr/local/bin/restic-restore.sh"
)

var (
	flagBackupOut   string
	flagRestoreFrom string
)

var backupCmd = &cobra.Command{
	Use:   "backup [service...]",
	Short: "Back up service config to version control (and trigger data snapshots)",
	Long: "Export a service's version-controllable state to text files you can commit " +
		"(Pi-hole lists + SOPS-encrypted custom DNS). With no service, backs up every " +
		"backup-capable service.\n\n" +
		"Restore the exported config with `ctdev restore`. For restic data snapshots of " +
		"the stack dirs and Docker volumes, use `ctdev backup now` / `ctdev backup snapshots`.",
	RunE:              runBackup,
	ValidArgsFunction: completeBackupServices,
}

var backupNowCmd = &cobra.Command{
	Use:   "now",
	Short: "Run a restic data snapshot now",
	Long:  "Run the restic backup (" + resticBackupScript + "): snapshot the stack dirs and Docker volumes to the offsite B2 repo and the local USB repo when mounted. Requires the restic component.",
	RunE:  runBackupNow,
}

var backupSnapshotsCmd = &cobra.Command{
	Use:   "snapshots [b2|local]",
	Short: "List restic snapshots",
	Long:  "List restic snapshots (newest last) from the offsite B2 repo (default) or the local USB repo. Requires the restic component.",
	RunE:  runBackupSnapshots,
}

var restoreCmd = &cobra.Command{
	Use:   "restore [service...]",
	Short: "Restore service config from version control",
	Long: "Apply a service's version-controlled config back to it (the inverse of `ctdev backup`). " +
		"For Pi-hole this applies the lists and rebuilds gravity (additive). With no service, " +
		"restores every backup-capable service.\n\n" +
		"This restores service config, not restic data snapshots — for those see RECOVERY.md / restic-restore.sh.",
	RunE:              runRestore,
	ValidArgsFunction: completeBackupServices,
}

func init() {
	backupCmd.Flags().StringVar(&flagBackupOut, "out", defaultPiholeOut, "directory to write exported config to")
	restoreCmd.Flags().StringVar(&flagRestoreFrom, "from", "", "directory to read config from (default: built-in)")
	backupCmd.AddCommand(backupNowCmd, backupSnapshotsCmd)
	rootCmd.AddCommand(backupCmd, restoreCmd)
}

// resolveBackupServices returns the providers named in args, or all of them when
// args is empty. Unknown names are an error listing the valid services.
func resolveBackupServices(args []string) ([]backupProvider, error) {
	all := backupProviders()
	if len(args) == 0 {
		return all, nil
	}
	byName := map[string]backupProvider{}
	var names []string
	for _, p := range all {
		byName[p.name] = p
		names = append(names, p.name)
	}
	var out []backupProvider
	for _, a := range args {
		p, ok := byName[a]
		if !ok {
			return nil, fmt.Errorf("unknown backup service %q (available: %s)", a, strings.Join(names, ", "))
		}
		out = append(out, p)
	}
	return out, nil
}

func runBackup(cmd *cobra.Command, args []string) error {
	services, err := resolveBackupServices(args)
	if err != nil {
		return err
	}
	ctx := cmdContext(cmd)
	for _, p := range services {
		if err := p.export(ctx); err != nil {
			return fmt.Errorf("backup %s: %w", p.name, err)
		}
	}
	return nil
}

func runRestore(cmd *cobra.Command, args []string) error {
	services, err := resolveBackupServices(args)
	if err != nil {
		return err
	}
	ctx := cmdContext(cmd)
	for _, p := range services {
		if err := p.restore(ctx); err != nil {
			return fmt.Errorf("restore %s: %w", p.name, err)
		}
	}
	return nil
}

func runBackupNow(cmd *cobra.Command, args []string) error {
	if err := requireResticScript(resticBackupScript); err != nil {
		return err
	}
	if !flagDryRun {
		if err := ensureSudo(); err != nil {
			return fmt.Errorf("sudo required: %w", err)
		}
	}
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	fmt.Println("Running restic backup...")
	return sysutil.SudoRun(cmdContext(cmd), o, resticBackupScript)
}

func runBackupSnapshots(cmd *cobra.Command, args []string) error {
	if err := requireResticScript(resticRestoreScript); err != nil {
		return err
	}
	if err := ensureSudo(); err != nil {
		return fmt.Errorf("sudo required: %w", err)
	}
	o := sysutil.Opts{Stdout: os.Stdout}
	return sysutil.SudoRun(cmdContext(cmd), o, resticRestoreScript, append([]string{"snapshots"}, args...)...)
}

// requireResticScript fails with an actionable message when the restic component
// hasn't been installed on this node.
func requireResticScript(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("restic is not installed on this node (run: ctdev install restic)")
	}
	return nil
}

func completeBackupServices(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	var names []string
	for _, p := range backupProviders() {
		names = append(names, p.name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
