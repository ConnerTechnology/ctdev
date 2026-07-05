package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var (
	flagResticRepo      string
	flagResticRepoLocal string
)

var configureResticCmd = &cobra.Command{
	Use:   "restic",
	Short: "Configure restic backups for this machine",
	Long: "Walk through setting up restic backups for this machine: pick a backend " +
		"(Backblaze B2, S3, SFTP, or a local/USB path), enter its credentials, and set a " +
		"repository password. The config is written to /etc/restic/restic.env, default " +
		"exclude patterns are seeded, the repository is initialized, and the daily timer is " +
		"enabled. Backups are opt-in: pick what to include with 'ctdev backup paths'. " +
		"Credentials are stored only on this host — if lost, just run this again.\n\n" +
		"Run a snapshot with 'ctdev backup now' and list snapshots with 'ctdev backup snapshots'.",
	RunE: runConfigureRestic,
}

func init() {
	configureResticCmd.Flags().StringVar(&flagResticRepo, "repo", "", "restic repository, set directly (e.g. b2:bucket:host) — skips the backend wizard")
	configureResticCmd.Flags().StringVar(&flagResticRepoLocal, "repo-local", "", "optional second repository for a 3-2-1 copy")
	configureCmd.AddCommand(configureResticCmd)
}

func runConfigureRestic(cmd *cobra.Command, args []string) error {
	return cancelToClean(configureRestic(cmdContext(cmd)))
}

// configureRestic walks the user through choosing a backend and credentials,
// writes /etc/restic/restic.env (0600), seeds the backup-paths/excludes files,
// runs `restic init`, and enables the daily timer. Nothing secret is stored
// anywhere but this host.
func configureRestic(ctx context.Context) error {
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}

	if !flagDryRun {
		if err := ensureSudo(); err != nil {
			return fmt.Errorf("sudo required: %w", err)
		}
	}

	current := component.ResticReadEnv(ctx)

	if flagConfigShow {
		return showResticConfig(current)
	}

	if isBatchMode() {
		return fmt.Errorf("ctdev configure restic needs interactive input (backend + credentials); run it without --batch")
	}

	fmt.Println(styles.Title.Render("restic backups"))
	fmt.Println(styles.Dimmed.Render("  Press Ctrl-C any time to cancel."))
	fmt.Println()

	repo, backendEnv, err := promptResticRepository(ctx, current)
	if err != nil {
		return err
	}
	env := map[string]string{"RESTIC_REPOSITORY": repo}
	for k, v := range backendEnv {
		if v != "" {
			env[k] = v
		}
	}
	fmt.Printf("\n  Repository: %s\n", styles.Value.Render(repo))

	pw, err := resolveResticPassword(ctx, current["RESTIC_PASSWORD"])
	if err != nil {
		return err
	}
	env["RESTIC_PASSWORD"] = pw

	local, err := promptWithDefaultCtx(ctx, "Optional second repo for a 3-2-1 copy (blank for none)", firstNonEmpty(flagResticRepoLocal, current["RESTIC_REPOSITORY_LOCAL"]))
	if err != nil {
		return err
	}
	if local != "" {
		env["RESTIC_REPOSITORY_LOCAL"] = local
	}

	if err := component.ResticWriteEnv(ctx, o, env); err != nil {
		return fmt.Errorf("write %s: %w", component.ResticEnvPath, err)
	}
	fmt.Printf("\n  Wrote %s (0600)\n", component.ResticEnvPath)

	// Backups are opt-in: nothing is included until you pick it. Seed only the
	// exclude patterns (harmless carve-outs applied once you include something).
	excludes := strings.Join(component.DefaultBackupExcludes(), "\n") + "\n"
	if wrote, err := component.ResticSeedFile(ctx, o, component.ResticExcludesFile, excludes); err != nil {
		return err
	} else if wrote {
		fmt.Printf("  Seeded default excludes in %s\n", component.ResticExcludesFile)
	}

	if flagDryRun {
		fmt.Println("\n  [dry-run] would run `restic init` (if new) and enable restic-backup.timer")
		return nil
	}

	if !sysutil.CommandExists("restic") {
		fmt.Println("\n  restic isn't installed yet — config saved. Finish with: ctdev install restic")
		return nil
	}

	fmt.Println("\n  Initializing repository (if new)...")
	if err := component.ResticInitIfNeeded(ctx, o); err != nil {
		return fmt.Errorf("restic init: %w", err)
	}

	if component.ResticFileExists(ctx, o, "/etc/systemd/system/restic-backup.timer") {
		if err := component.ResticEnableTimer(ctx, o); err != nil {
			return fmt.Errorf("enable timer: %w", err)
		}
		fmt.Println("  Daily backup timer enabled.")
	} else {
		fmt.Println("  Run 'ctdev install restic' to deploy the daily backup timer and helper scripts.")
	}

	fmt.Println("\n  Done. Backups are opt-in — nothing is saved until you choose what to back up:")
	fmt.Println("    ctdev backup paths      pick folders/files (web UI)")
	fmt.Println("    ctdev backup now        snapshot your selection")
	return nil
}

// promptResticRepository runs the backend wizard and returns the restic
// repository string plus any backend credential env vars. With --repo set it
// skips the menu and only prompts for credentials inferred from the scheme.
func promptResticRepository(ctx context.Context, current map[string]string) (string, map[string]string, error) {
	backendEnv := map[string]string{}
	host, _ := os.Hostname()
	if host == "" {
		host = "host"
	}

	if flagResticRepo != "" {
		if err := promptBackendCreds(ctx, flagResticRepo, current, backendEnv); err != nil {
			return "", nil, err
		}
		return flagResticRepo, backendEnv, nil
	}

	fmt.Println("  Where should backups go?")
	fmt.Println("    1) Backblaze B2          (recommended)")
	fmt.Println("    2) Amazon S3 / S3-compatible")
	fmt.Println("    3) SFTP (another server over SSH)")
	fmt.Println("    4) Local path or USB drive")
	fmt.Printf("  %s ", styles.Dimmed.Render("Choice [1]:"))
	choice, err := promptChoiceCtx(ctx, 1)
	if err != nil {
		return "", nil, err
	}
	fmt.Println()

	switch choice {
	case 1:
		fmt.Println(styles.Dimmed.Render("  Create a bucket and an Application Key at"))
		fmt.Println(styles.Dimmed.Render("  https://secure.backblaze.com → B2 Cloud Storage → Buckets / Application Keys."))
		bucket, err := promptRequiredCtx(ctx, "B2 bucket name", "")
		if err != nil {
			return "", nil, err
		}
		path, err := promptWithDefaultCtx(ctx, "Path/prefix inside the bucket", host)
		if err != nil {
			return "", nil, err
		}
		backendEnv["B2_ACCOUNT_ID"], err = promptSecretRequiredCtx(ctx, "B2 keyID (B2_ACCOUNT_ID)", current["B2_ACCOUNT_ID"])
		if err != nil {
			return "", nil, err
		}
		backendEnv["B2_ACCOUNT_KEY"], err = promptSecretRequiredCtx(ctx, "B2 applicationKey (B2_ACCOUNT_KEY)", current["B2_ACCOUNT_KEY"])
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("b2:%s:%s", bucket, path), backendEnv, nil

	case 2:
		fmt.Println(styles.Dimmed.Render("  Needs a bucket, endpoint host, and an access key/secret."))
		bucket, err := promptRequiredCtx(ctx, "S3 bucket name", "")
		if err != nil {
			return "", nil, err
		}
		endpoint, err := promptWithDefaultCtx(ctx, "Endpoint host", "s3.amazonaws.com")
		if err != nil {
			return "", nil, err
		}
		backendEnv["AWS_ACCESS_KEY_ID"], err = promptSecretRequiredCtx(ctx, "AWS access key ID", current["AWS_ACCESS_KEY_ID"])
		if err != nil {
			return "", nil, err
		}
		backendEnv["AWS_SECRET_ACCESS_KEY"], err = promptSecretRequiredCtx(ctx, "AWS secret access key", current["AWS_SECRET_ACCESS_KEY"])
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("s3:%s/%s", endpoint, bucket), backendEnv, nil

	case 3:
		fmt.Println(styles.Dimmed.Render("  restic authenticates with your existing SSH keys/config."))
		dest, err := promptRequiredCtx(ctx, "SFTP user@host", "")
		if err != nil {
			return "", nil, err
		}
		path, err := promptWithDefaultCtx(ctx, "Path on the server", "/srv/restic/"+host)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("sftp:%s:%s", dest, path), backendEnv, nil

	case 4:
		fmt.Println(styles.Dimmed.Render("  A directory on this machine or a mounted external/USB drive."))
		path, err := promptRequiredCtx(ctx, "Repository path", "/mnt/backup/restic")
		if err != nil {
			return "", nil, err
		}
		return path, backendEnv, nil

	default:
		return "", nil, fmt.Errorf("invalid choice %d (pick 1–4)", choice)
	}
}

// promptBackendCreds prompts for the credentials a --repo string implies.
func promptBackendCreds(ctx context.Context, repo string, current, backendEnv map[string]string) error {
	var err error
	switch {
	case strings.HasPrefix(repo, "b2:"):
		if backendEnv["B2_ACCOUNT_ID"], err = promptSecretRequiredCtx(ctx, "B2 keyID (B2_ACCOUNT_ID)", current["B2_ACCOUNT_ID"]); err != nil {
			return err
		}
		backendEnv["B2_ACCOUNT_KEY"], err = promptSecretRequiredCtx(ctx, "B2 applicationKey (B2_ACCOUNT_KEY)", current["B2_ACCOUNT_KEY"])
	case strings.HasPrefix(repo, "s3:"):
		if backendEnv["AWS_ACCESS_KEY_ID"], err = promptSecretRequiredCtx(ctx, "AWS access key ID", current["AWS_ACCESS_KEY_ID"]); err != nil {
			return err
		}
		backendEnv["AWS_SECRET_ACCESS_KEY"], err = promptSecretRequiredCtx(ctx, "AWS secret access key", current["AWS_SECRET_ACCESS_KEY"])
	}
	return err
}

// resolveResticPassword keeps the existing password, lets the user paste a new
// one, or generates a strong one when there's none and the reply is blank.
func resolveResticPassword(ctx context.Context, existing string) (string, error) {
	if existing != "" {
		entered, err := promptSecretCtx(ctx, "Repository password", existing)
		if err != nil {
			return "", err
		}
		if entered != "" {
			return entered, nil
		}
		return existing, nil
	}
	fmt.Println()
	fmt.Println(styles.Dimmed.Render("  The repository password encrypts your backups. Without it they're"))
	fmt.Println(styles.Dimmed.Render("  unrecoverable, and ctdev stores it only on this host — save a copy."))
	for {
		entered, err := promptSecretCtx(ctx, "Repository password (blank = generate one)", "")
		if err != nil {
			return "", err
		}
		if entered == "" {
			break
		}
		// Input is masked, so a typo would be invisible — confirm new passwords.
		confirm, err := promptSecretCtx(ctx, "Repeat to confirm", "")
		if err != nil {
			return "", err
		}
		if confirm == entered {
			return entered, nil
		}
		fmt.Println(styles.Warning.Render("  Passwords don't match — try again."))
	}
	gen, err := generatePassword(32)
	if err != nil {
		return "", err
	}
	fmt.Println()
	fmt.Println("  Generated a repository password — copy it into your password manager NOW:")
	fmt.Printf("    %s\n", styles.Value.Render(gen))
	return gen, nil
}

// generatePassword returns a URL-safe random string with nBytes of entropy.
func generatePassword(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// backupPathsContent renders the seeded /etc/restic/backup-paths file.
func backupPathsContent(paths []string) string {
	var b strings.Builder
	b.WriteString("# Paths backed up by restic-backup.sh — one per line, '#' for comments.\n")
	b.WriteString("# Edit freely; missing paths are skipped at backup time.\n")
	for _, p := range paths {
		b.WriteString(p)
		b.WriteString("\n")
	}
	return b.String()
}

func showResticConfig(env map[string]string) error {
	label := styles.Label(22)
	set := func(v string) string {
		if v != "" {
			return "(set)"
		}
		return "(unset)"
	}
	fmt.Println(styles.Title.Render("restic backups"))
	fmt.Println()
	if len(env) == 0 {
		fmt.Println("  Not configured. Run: ctdev configure restic")
		return nil
	}
	fmt.Printf("  %s %s\n", label.Render("Repository:"), styles.Value.Render(orDash(env["RESTIC_REPOSITORY"])))
	fmt.Printf("  %s %s\n", label.Render("Second repo:"), styles.Value.Render(orDash(env["RESTIC_REPOSITORY_LOCAL"])))
	fmt.Printf("  %s %s\n", label.Render("Password:"), styles.Value.Render(set(env["RESTIC_PASSWORD"])))
	for _, k := range []string{"B2_ACCOUNT_ID", "B2_ACCOUNT_KEY", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"} {
		if env[k] != "" {
			fmt.Printf("  %s %s\n", label.Render(k+":"), styles.Value.Render("(set)"))
		}
	}
	return nil
}
