package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/setup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var (
	flagGitName       string
	flagGitEmail      string
	flagGitLocal      bool
	flagGitSigningKey string
	flagGitGlobal     bool
	flagConfigShow    bool

	flagAWSProfile string

	// flagSwapfileSize is `configure storage --swapfile-size`, in GiB.
	flagSwapfileSize int
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure system, git, AWS, and other settings",
	Long:  "Interactive system configuration wizard. Run without arguments to walk through all categories, or specify a category.",
	RunE:  runConfigureAll,
}

var configureAWSCmd = &cobra.Command{
	Use:   "aws",
	Short: "Configure AWS profile",
	Long: "Set the default AWS_PROFILE in your shell environment. Installs the AWS CLI first " +
		"when it's missing, and offers 'aws configure sso' when ~/.aws/config has no profiles.",
	RunE: runConfigureAWS,
}

var configureGitCmd = &cobra.Command{
	Use:   "git",
	Short: "Configure git user settings",
	Long:  "Set git user name and email globally or locally. Installs git first when it's missing.",
	RunE:  runConfigureGit,
}

func init() {
	configureCmd.PersistentFlags().BoolVar(&flagConfigShow, "show", false, "show current values without changing")

	configureGitCmd.Flags().StringVar(&flagGitName, "name", "", "git user name")
	configureGitCmd.Flags().StringVar(&flagGitEmail, "email", "", "git user email")
	configureGitCmd.Flags().BoolVar(&flagGitLocal, "local", false, "apply to current repo only")
	configureGitCmd.Flags().BoolVar(&flagGitGlobal, "global", false, "force global scope")
	configureGitCmd.Flags().StringVar(&flagGitSigningKey, "signing-key", "", "SSH signing key path")
	configureCmd.AddCommand(configureGitCmd)
	configureAWSCmd.Flags().StringVar(&flagAWSProfile, "profile", "", "AWS profile name")
	configureCmd.AddCommand(configureAWSCmd)

	// Register system configuration category subcommands. gpu is handled by a
	// dedicated subcommand (configure_gpu.go) that also runs driver signing, so
	// skip the generic registration for it here. pihole has no subcommand
	// (CTD-171): `ctdev install pihole` and a bare `ctdev configure` reach it.
	for _, slug := range slugOrder {
		if slug == "gpu" || slug == "pihole" {
			continue
		}
		slug := slug // capture
		cmd := &cobra.Command{
			Use:   slug,
			Short: fmt.Sprintf("Configure %s settings", slugDescription(slug)),
			RunE: func(cmd *cobra.Command, args []string) error {
				var adjust func([]setup.SettingState) error
				if cmd.Flags().Changed("swapfile-size") {
					// The wizard asks for the size itself, and --show changes
					// nothing. It needs --batch itself, not just piped stdin, so
					// a size applies only when the caller asked for batch.
					if flagConfigShow || !flagBatch {
						return fmt.Errorf("--swapfile-size only works with --batch")
					}
					gib := flagSwapfileSize
					adjust = func(states []setup.SettingState) error {
						return setup.RequestSwapfileSize(states, gib)
					}
				}
				if flagConfigShow {
					return runCategoryWizard(cmdContext(cmd), slug, true)
				}
				// Interactive ssh is a menu over your keys and the server
				// settings (CTD-177); it asks for sudo when it needs it.
				if slug == "ssh" && !isBatchMode() {
					return cancelToClean(runSSHMenu(cmdContext(cmd)))
				}
				if !flagDryRun {
					if err := ensureSudo(cmdContext(cmd)); err != nil {
						return fmt.Errorf("sudo required: %w", err)
					}
				}
				// Non-interactive (--batch or piped stdin): apply defaults without prompting.
				if isBatchMode() {
					return runCategoryBatchAdjusted(cmdContext(cmd), slug, adjust)
				}
				return cancelToClean(runCategoryWizard(cmdContext(cmd), slug, false))
			},
		}
		if slug == "storage" {
			cmd.Flags().IntVar(&flagSwapfileSize, "swapfile-size", 0,
				"with --batch: the swapfile's size in GiB, which also resizes ctdev's own swapfile")
		}
		configureCmd.AddCommand(cmd)
	}

	rootCmd.AddCommand(configureCmd)
}

func runConfigureAll(cmd *cobra.Command, args []string) error {
	// An unknown category (e.g. `ctdev configure nosuch`) is not a subcommand, so
	// cobra falls through to this parent RunE. Reject it instead of silently
	// walking every category — only a bare `ctdev configure` runs the full sweep.
	if len(args) > 0 {
		var names []string
		if cmd != nil {
			for _, c := range cmd.Commands() {
				if c.Name() != "help" {
					names = append(names, c.Name())
				}
			}
		}
		sort.Strings(names)
		return fmt.Errorf("unknown configure category %q (available: %s)", args[0], strings.Join(names, ", "))
	}

	ctx := cmdContext(cmd)

	// Interactive full sweep → the full-screen settings browser. The line
	// wizard remains for per-category runs (`ctdev configure ssh`), --show,
	// and non-TTY/ACCESSIBLE sessions.
	if !flagConfigShow && !isBatchMode() {
		// Sudo up front: detection reads some root-only state (pihole), and
		// the apply that follows the browser needs it anyway.
		if !flagDryRun {
			if err := ensureSudo(cmdContext(cmd)); err != nil {
				return fmt.Errorf("sudo required: %w", err)
			}
		}
		return cancelToClean(runSettingsBrowser(ctx))
	}

	if !flagDryRun && !flagConfigShow {
		if err := ensureSudo(cmdContext(cmd)); err != nil {
			return fmt.Errorf("sudo required: %w", err)
		}
	}

	for _, slug := range slugOrder {
		// The GPU category's driver-signing apply launches the interactive
		// Secure Boot/MOK enrollment flow (one-time password, reboot steps) —
		// never spring that mid-sweep. It stays behind an explicit
		// `ctdev configure gpu`; --show remains safe to include.
		if slug == "gpu" && !flagConfigShow {
			if len(setup.FilterByHardware(setup.FilterBySlug(setup.Registry, slug))) > 0 {
				fmt.Println(styles.Dimmed.Render("  Skipping GPU & NVIDIA — run 'ctdev configure gpu' (includes the Secure Boot driver-signing flow)."))
				fmt.Println()
			}
			continue
		}
		if flagConfigShow {
			if err := runCategoryWizard(ctx, slug, true); err != nil {
				return cancelToClean(err)
			}
			continue
		}
		if err := runCategoryBatch(ctx, slug); err != nil {
			return cancelToClean(err)
		}
	}
	return nil
}

// cmdContext returns cmd.Context() or context.Background() if cobra hasn't
// seeded one yet — keeps helpers safe to call from both normal and test paths.
func cmdContext(cmd *cobra.Command) context.Context {
	if cmd != nil {
		if c := cmd.Context(); c != nil {
			return c
		}
	}
	return context.Background()
}

func runConfigureGit(cmd *cobra.Command, args []string) error {
	return cancelToClean(configureGit(cmdContext(cmd)))
}

func configureGit(ctx context.Context) error {
	if flagConfigShow {
		return showGitConfig()
	}

	if ok, err := ensureTool(ctx, "git", "Git", isBatchMode()); !ok || err != nil {
		return err
	}

	// If flags provided, set directly (non-interactive). --signing-key counts
	// too — it used to be silently ignored unless --name/--email came along.
	if flagGitName != "" || flagGitEmail != "" || flagGitSigningKey != "" {
		local := flagGitLocal
		if flagGitName != "" || flagGitEmail != "" {
			if err := setGitConfig(flagGitName, flagGitEmail, local); err != nil {
				return err
			}
		}
		if flagGitSigningKey != "" {
			return setGitSigningKey(flagGitSigningKey, local)
		}
		return nil
	}

	// Interactive mode
	if isBatchMode() {
		return fmt.Errorf("--name and --email required in batch mode")
	}

	fmt.Println(styles.Title.Render("Git Configuration"))
	fmt.Println()

	// Step 1: Scope detection
	local := flagGitLocal
	if flagGitGlobal {
		local = false
	} else if !flagGitLocal {
		if _, err := os.Stat(".git"); err == nil {
			fmt.Println("  Git repository detected.")
			fmt.Println("    1) Global (all repos)")
			fmt.Println("    2) This repo only")
			fmt.Printf("  Select [1]: ")
			choice, err := promptChoiceCtx(ctx, 1)
			if err != nil {
				return err
			}
			if choice == 2 {
				local = true
			}
			fmt.Println()
		}
	}

	labelStyle := styles.Label(20)
	valueStyle := styles.Value

	scope := "global"
	if local {
		scope = "local"
	}
	fmt.Printf("  %s %s\n\n", labelStyle.Render("Scope:"), valueStyle.Render(scope))

	// Step 2: Name prompt
	name, err := promptWithDefaultCtx(ctx, "Name", getGitConfig("user.name", local))
	if err != nil {
		return err
	}

	// Step 3: Email prompt
	email, err := promptWithDefaultCtx(ctx, "Email", getGitConfig("user.email", local))
	if err != nil {
		return err
	}

	fmt.Println()

	// Step 4: SSH signing key picker
	keyPath, err := promptSSHSigningKey(ctx, email)
	if err != nil {
		return err
	}

	// Step 5: GitHub upload (if key selected)
	if keyPath != "" {
		if err := promptGitHubKeyUpload(ctx, keyPath); err != nil {
			return err
		}
	}

	// Step 6: Apply settings
	if err := setGitConfig(name, email, local); err != nil {
		return err
	}
	if keyPath != "" {
		if err := setGitSigningKey(keyPath, local); err != nil {
			return err
		}
	}

	return nil
}

func promptSSHSigningKey(ctx context.Context, email string) (string, error) {
	keys := sysutil.FindSSHPublicKeys()

	fmt.Println("  SSH Signing Key:")

	optionNum := 1
	for _, k := range keys {
		home, _ := os.UserHomeDir()
		displayPath := k.Path
		if home != "" {
			displayPath = strings.Replace(k.Path, home, "~", 1)
		}
		fmt.Printf("    %d) %s (%s)\n", optionNum, displayPath, k.KeyType)
		optionNum++
	}

	generateIdx := optionNum
	fmt.Printf("    %d) Generate new ed25519 key\n", optionNum)
	optionNum++

	customIdx := optionNum
	fmt.Printf("    %d) Enter custom path\n", optionNum)
	optionNum++

	fmt.Printf("    %d) Skip (no signing)\n", optionNum)

	defaultChoice := 1
	if len(keys) == 0 {
		defaultChoice = generateIdx
	}
	fmt.Printf("  Select [%d]: ", defaultChoice)
	choice, err := promptChoiceCtx(ctx, defaultChoice)
	if err != nil {
		return "", err
	}

	fmt.Println()

	// Existing key selected
	if choice >= 1 && choice <= len(keys) {
		selected := keys[choice-1]
		fmt.Println(styles.Success.Render(fmt.Sprintf("  Using key: %s", selected.Path)))
		// Display the public key
		if data, err := os.ReadFile(selected.Path); err == nil {
			fmt.Printf("  %s\n", styles.Dimmed.Render(strings.TrimSpace(string(data))))
		}
		fmt.Println()
		return selected.Path, nil
	}

	// Generate new key
	if choice == generateIdx {
		return generateSSHKey(ctx, email)
	}

	// Custom path
	if choice == customIdx {
		path, err := promptWithDefaultCtx(ctx, "Key path", "")
		if err != nil {
			return "", err
		}
		if path == "" {
			return "", nil
		}
		// Expand ~ to home dir
		if strings.HasPrefix(path, "~/") {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, path[2:])
		}
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("key file not found: %s", path)
		}
		fmt.Println()
		return path, nil
	}

	// Skip
	return "", nil
}

// generateSSHKey makes a new ed25519 key for git signing, with the same
// file-name prompt as `configure ssh`'s Create a key, and returns its public
// key's path ("" under --dry-run).
func generateSSHKey(ctx context.Context, email string) (string, error) {
	dir, err := sshDir()
	if err != nil {
		return "", err
	}
	path, err := promptKeyPath(ctx, dir, keyTypes[0])
	if err != nil {
		return "", err
	}
	pair, ok, err := createKey(ctx, newKey{typ: keyTypes[0], path: path, comment: email})
	if err != nil || !ok {
		return "", err
	}
	return pair.Public, nil
}

func promptGitHubKeyUpload(ctx context.Context, keyPath string) error {
	// Uploading a key is an outward-facing write — never do it in a preview.
	if flagDryRun {
		fmt.Println("  [dry-run] would offer to add the key to GitHub (gh ssh-key add)")
		return nil
	}
	hostname, _ := os.Hostname()
	title := "ctdev-" + hostname
	if !githubReady(ctx) {
		printManualGitHubInstructions(title, githubBoth)
		return nil
	}

	yes, err := promptYesNoCtx(ctx, "Add this key to GitHub?", true)
	if err != nil {
		return err
	}
	fmt.Println()
	if !yes {
		return nil
	}
	uploadKeyToGitHub(ctx, keyPath, title, githubBoth)
	fmt.Println()
	return nil
}

func showGitConfig() error {
	labelStyle := styles.Label(20)
	valueStyle := styles.Value
	headerStyle := styles.Header

	name := getGitConfig("user.name", false)
	email := getGitConfig("user.email", false)
	signingKey := getGitConfig("user.signingKey", false)
	gpgSign := getGitConfig("commit.gpgsign", false)
	localName := getGitConfig("user.name", true)
	localEmail := getGitConfig("user.email", true)
	localSigningKey := getGitConfig("user.signingKey", true)
	localGpgSign := getGitConfig("commit.gpgsign", true)

	fmt.Println(styles.Title.Render("Git Configuration"))
	fmt.Println()

	fmt.Println(headerStyle.Render("Global"))
	fmt.Printf("  %s %s\n", labelStyle.Render("Name"), valueStyle.Render(name))
	fmt.Printf("  %s %s\n", labelStyle.Render("Email"), valueStyle.Render(email))
	signingKeyDisplay := signingKey
	if signingKeyDisplay == "" {
		signingKeyDisplay = styles.Dimmed.Render("(not set)")
	}
	fmt.Printf("  %s %s\n", labelStyle.Render("Signing Key"), valueStyle.Render(signingKeyDisplay))
	gpgSignDisplay := gpgSign
	if gpgSignDisplay == "" {
		gpgSignDisplay = styles.Dimmed.Render("(not set)")
	}
	fmt.Printf("  %s %s\n", labelStyle.Render("GPG Sign"), valueStyle.Render(gpgSignDisplay))

	hasLocal := localName != "" || localEmail != "" || localSigningKey != "" || localGpgSign != ""
	if hasLocal {
		fmt.Println()
		fmt.Println(headerStyle.Render("Local (this repo)"))
		localNameDisplay := localName
		if localNameDisplay == "" {
			localNameDisplay = styles.Dimmed.Render("(not set)")
		}
		localEmailDisplay := localEmail
		if localEmailDisplay == "" {
			localEmailDisplay = styles.Dimmed.Render("(not set)")
		}
		localSigningKeyDisplay := localSigningKey
		if localSigningKeyDisplay == "" {
			localSigningKeyDisplay = styles.Dimmed.Render("(not set)")
		}
		localGpgSignDisplay := localGpgSign
		if localGpgSignDisplay == "" {
			localGpgSignDisplay = styles.Dimmed.Render("(not set)")
		}
		fmt.Printf("  %s %s\n", labelStyle.Render("Name"), valueStyle.Render(localNameDisplay))
		fmt.Printf("  %s %s\n", labelStyle.Render("Email"), valueStyle.Render(localEmailDisplay))
		fmt.Printf("  %s %s\n", labelStyle.Render("Signing Key"), valueStyle.Render(localSigningKeyDisplay))
		fmt.Printf("  %s %s\n", labelStyle.Render("GPG Sign"), valueStyle.Render(localGpgSignDisplay))
	}

	return nil
}

func getGitConfig(key string, local bool) string {
	args := []string{"config"}
	if local {
		args = append(args, "--local")
	} else {
		args = append(args, "--global")
	}
	args = append(args, key)
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func setGitConfig(name, email string, local bool) error {
	scope := "--global"
	if local {
		scope = "--local"
	}

	if name != "" {
		if flagDryRun {
			fmt.Printf("  [dry-run] git config %s user.name %q\n", scope, name)
		} else {
			if err := exec.Command("git", "config", scope, "user.name", name).Run(); err != nil {
				return fmt.Errorf("failed to set user.name: %w", err)
			}
			fmt.Println(styles.Success.Render(fmt.Sprintf("  Set user.name = %s", name)))
		}
	}

	if email != "" {
		if flagDryRun {
			fmt.Printf("  [dry-run] git config %s user.email %q\n", scope, email)
		} else {
			if err := exec.Command("git", "config", scope, "user.email", email).Run(); err != nil {
				return fmt.Errorf("failed to set user.email: %w", err)
			}
			fmt.Println(styles.Success.Render(fmt.Sprintf("  Set user.email = %s", email)))
		}
	}

	return nil
}

func runConfigureAWS(cmd *cobra.Command, args []string) error {
	return cancelToClean(configureAWS(cmdContext(cmd)))
}

func configureAWS(ctx context.Context) error {
	if flagConfigShow {
		return showAWSConfig()
	}

	if ok, err := ensureTool(ctx, "aws", "AWS CLI", isBatchMode()); !ok || err != nil {
		return err
	}

	selected := flagAWSProfile

	if selected == "" {
		profiles, _ := sysutil.ReadAWSProfiles()
		if len(profiles) == 0 {
			var err error
			if profiles, err = offerAWSConfigureSSO(ctx, isBatchMode()); err != nil || len(profiles) == 0 {
				return err
			}
		}

		fmt.Println("AWS Profiles:")
		for i, p := range profiles {
			fmt.Printf("  %d) %s\n", i+1, p)
		}
		fmt.Printf("Select: ")
		choice, err := promptChoiceCtx(ctx, 1)
		if err != nil {
			return err
		}

		if choice < 1 || choice > len(profiles) {
			return fmt.Errorf("invalid selection: %d", choice)
		}
		selected = profiles[choice-1]
	}

	if flagDryRun {
		fmt.Printf("[dry-run] would set AWS_PROFILE=%s in %s\n", selected, sysutil.ExportsLocalPath())
		return nil
	}

	if err := sysutil.SetLineInFile(sysutil.ExportsLocalPath(), "AWS_PROFILE", "export AWS_PROFILE="+selected); err != nil {
		return fmt.Errorf("failed to set AWS_PROFILE: %w", err)
	}

	fmt.Println(styles.Success.Render(fmt.Sprintf("AWS_PROFILE set to %s", selected)))
	fmt.Println(styles.Dimmed.Render("Restart your shell or run: source " + sysutil.ExportsLocalPath()))
	return nil
}

// showAWSConfig prints the AWS_PROFILE currently exported by the shell config.
func showAWSConfig() error {
	profile := "(not set)"
	if b, err := os.ReadFile(sysutil.ExportsLocalPath()); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(line), "export AWS_PROFILE="); ok {
				profile = v
			}
		}
	}
	fmt.Println(styles.Title.Render("AWS"))
	fmt.Println()
	fmt.Printf("  %s %s\n", styles.Label(20).Render("AWS_PROFILE:"), styles.Value.Render(profile))
	return nil
}

func setGitSigningKey(keyPath string, local bool) error {
	scope := "--global"
	if local {
		scope = "--local"
	}

	if flagDryRun {
		fmt.Printf("  [dry-run] git config %s gpg.format ssh\n", scope)
		fmt.Printf("  [dry-run] git config %s user.signingKey %q\n", scope, keyPath)
		fmt.Printf("  [dry-run] git config %s commit.gpgsign true\n", scope)
		return nil
	}

	if err := exec.Command("git", "config", scope, "gpg.format", "ssh").Run(); err != nil {
		return fmt.Errorf("failed to set gpg.format: %w", err)
	}
	fmt.Println(styles.Success.Render("  Set gpg.format = ssh"))

	if err := exec.Command("git", "config", scope, "user.signingKey", keyPath).Run(); err != nil {
		return fmt.Errorf("failed to set user.signingKey: %w", err)
	}
	fmt.Println(styles.Success.Render(fmt.Sprintf("  Set user.signingKey = %s", keyPath)))

	if err := exec.Command("git", "config", scope, "commit.gpgsign", "true").Run(); err != nil {
		return fmt.Errorf("failed to set commit.gpgsign: %w", err)
	}
	fmt.Println(styles.Success.Render("  Set commit.gpgsign = true"))

	return nil
}
