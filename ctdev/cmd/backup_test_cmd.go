package cmd

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var backupTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Check backups are set up correctly (config, connection, paths)",
	Long: "Run a full preflight of this machine's restic backup setup: the tool is installed, " +
		"the repository is configured and reachable (credentials + network verified against the " +
		"live repo), the selected paths exist, and the nightly timer is enabled. Nothing is " +
		"written. Exits non-zero if anything is wrong, so it works as a health check.",
	RunE: runBackupTest,
}

func init() {
	backupCmd.AddCommand(backupTestCmd)
}

func runBackupTest(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)

	fmt.Println(styles.Title.Render("Backup preflight"))
	fmt.Println()

	failed := 0
	report := func(name string, ok bool, detail string) bool {
		mark := styles.Success.Render("✓")
		if !ok {
			mark = styles.Error.Render("✗")
			failed++
		}
		line := "  " + mark + " " + name
		if detail != "" {
			line += "  " + styles.Dimmed.Render(detail)
		}
		fmt.Println(line)
		return ok
	}
	stop := func(msg string) error {
		fmt.Println()
		return fmt.Errorf("%s", msg)
	}

	// 1. restic installed.
	if !report("restic installed", sysutil.CommandExists("restic"), firstLine(runForOutput("restic", "version"))) {
		return stop("restic is not installed — run: ctdev install restic")
	}
	// 2. backup script deployed.
	if !report("backup script deployed", pathExists(resticBackupScript), resticBackupScript) {
		return stop("backup script missing — run: ctdev install restic")
	}

	if err := ensureSudo(); err != nil {
		return fmt.Errorf("sudo required to read /etc/restic: %w", err)
	}

	// 3. repository configured (RESTIC_REPOSITORY actually set).
	env := component.ResticReadEnv(ctx)
	if !report("repository configured", env["RESTIC_REPOSITORY"] != "", env["RESTIC_REPOSITORY"]) {
		return stop("not configured — run: ctdev configure restic")
	}

	// 4. selected paths exist on disk.
	paths := component.ResticReadLines(ctx, component.ResticPathsFile)
	if len(paths) == 0 {
		report("paths selected", false, "none — run: ctdev backup paths")
	} else {
		missing := missingPaths(ctx, paths)
		detail := fmt.Sprintf("%d path(s)", len(paths))
		if len(missing) > 0 {
			detail = fmt.Sprintf("%d of %d missing: %s", len(missing), len(paths), strings.Join(missing, ", "))
		}
		// Missing paths are skipped at backup time, not fatal — but surface them.
		report("selected paths exist", len(missing) == 0, detail)
	}

	// 5. repository reachable — `restic cat config` proves credentials, network,
	// and password against the live repo without modifying it.
	reach := resticRepoReachable(ctx)
	report("repository reachable", reach == "", firstNonEmpty(reach, "credentials + network OK"))

	// 6. nightly timer enabled.
	report("daily timer enabled", unitEnabled("restic-backup.timer"), unitEnabledState("restic-backup.timer"))

	// 7. healthcheck (informational — absence is a choice, not a failure).
	if env["HEALTHCHECK_URL"] != "" {
		report("healthcheck configured", true, "")
	} else {
		fmt.Println("  " + styles.Dimmed.Render("○ no healthcheck URL (optional — alerts you if backups stop)"))
	}

	fmt.Println()
	if failed > 0 {
		return fmt.Errorf("%d backup check(s) failed", failed)
	}
	fmt.Println(styles.Success.Render("Backups are set up correctly."))
	return nil
}

// missingPaths returns the selected paths that don't exist on disk (checked via
// sudo, since some — Docker volumes — are root-only).
func missingPaths(ctx context.Context, paths []string) []string {
	quiet := sysutil.Opts{Stdout: io.Discard}
	var missing []string
	for _, p := range paths {
		if sysutil.SudoRun(ctx, quiet, "test", "-e", p) != nil {
			missing = append(missing, p)
		}
	}
	return missing
}

// resticRepoReachable runs `restic cat config` (read-only) with the backend
// credentials sourced from restic.env, the same way the backup script does.
// Returns "" on success or a short error detail.
func resticRepoReachable(ctx context.Context) string {
	script := fmt.Sprintf("set -a; . %s; set +a; restic cat config >/dev/null", component.ResticEnvPath)
	out, err := exec.CommandContext(ctx, "sudo", "bash", "-c", script).CombinedOutput()
	if err != nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return firstLine(s)
		}
		return err.Error()
	}
	return ""
}
