package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
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
	Long:  "Set the default AWS_PROFILE in your shell environment.",
	RunE:  runConfigureAWS,
}

var configureGitCmd = &cobra.Command{
	Use:   "git",
	Short: "Configure git user settings",
	Long:  "Set git user name and email globally or locally.",
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
	// skip the generic registration for it here.
	for _, slug := range slugOrder {
		if slug == "gpu" {
			continue
		}
		slug := slug // capture
		cmd := &cobra.Command{
			Use:   slug,
			Short: fmt.Sprintf("Configure %s settings", slugDescription(slug)),
			RunE: func(cmd *cobra.Command, args []string) error {
				if !flagDryRun && !flagConfigShow {
					if err := ensureSudo(); err != nil {
						return fmt.Errorf("sudo required: %w", err)
					}
				}
				if flagConfigShow {
					return runCategoryWizard(cmdContext(cmd), slug, true)
				}
				// Non-interactive (--batch or piped stdin): apply defaults without prompting.
				if isBatchMode() {
					return runCategoryBatch(cmdContext(cmd), slug)
				}
				return runCategoryWizard(cmdContext(cmd), slug, false)
			},
		}
		configureCmd.AddCommand(cmd)
	}

	rootCmd.AddCommand(configureCmd)
}

func runConfigureAll(cmd *cobra.Command, args []string) error {
	if !flagDryRun && !flagConfigShow {
		if err := ensureSudo(); err != nil {
			return fmt.Errorf("sudo required: %w", err)
		}
	}

	ctx := cmdContext(cmd)
	for _, slug := range slugOrder {
		if err := runCategoryWizard(ctx, slug, flagConfigShow); err != nil {
			return err
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

var stdinScanner = bufio.NewScanner(os.Stdin)

// promptLine reads a single line of input, returning empty string on EOF.
func promptLine() string {
	if stdinScanner.Scan() {
		return strings.TrimSpace(stdinScanner.Text())
	}
	return ""
}

// promptChoice shows a numbered prompt and returns the 1-based selection.
// Returns defaultVal if input is empty.
func promptChoice(defaultVal int) int {
	input := promptLine()
	if input == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(input)
	if err != nil {
		return defaultVal
	}
	return n
}

func runConfigureGit(cmd *cobra.Command, args []string) error {
	if flagConfigShow {
		return showGitConfig()
	}

	// If flags provided, set directly (non-interactive)
	if flagGitName != "" || flagGitEmail != "" {
		local := flagGitLocal
		if err := setGitConfig(flagGitName, flagGitEmail, local); err != nil {
			return err
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
			choice := promptChoice(1)
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
	currentName := getGitConfig("user.name", local)
	fmt.Printf("  %s ", styles.Dimmed.Render(fmt.Sprintf("Name [%s]:", currentName)))
	name := promptLine()
	if name == "" {
		name = currentName
	}

	// Step 3: Email prompt
	currentEmail := getGitConfig("user.email", local)
	fmt.Printf("  %s ", styles.Dimmed.Render(fmt.Sprintf("Email [%s]:", currentEmail)))
	email := promptLine()
	if email == "" {
		email = currentEmail
	}

	fmt.Println()

	// Step 4: SSH signing key picker
	keyPath, err := promptSSHSigningKey(email)
	if err != nil {
		return err
	}

	// Step 5: GitHub upload (if key selected)
	if keyPath != "" {
		promptGitHubKeyUpload(keyPath)
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

func promptSSHSigningKey(email string) (string, error) {
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
	choice := promptChoice(defaultChoice)

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
		return generateSSHKey(email)
	}

	// Custom path
	if choice == customIdx {
		fmt.Printf("  Key path: ")
		path := promptLine()
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

func generateSSHKey(email string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	keyPath := filepath.Join(home, ".ssh", "id_ed25519")

	fmt.Println("  Generating new ed25519 key...")
	fmt.Println()

	genCmd := exec.Command("ssh-keygen", "-t", "ed25519", "-C", email)
	genCmd.Stdin = os.Stdin
	genCmd.Stdout = os.Stdout
	genCmd.Stderr = os.Stderr
	if err := genCmd.Run(); err != nil {
		return "", fmt.Errorf("ssh-keygen failed: %w", err)
	}

	pubKeyPath := keyPath + ".pub"
	if _, err := os.Stat(pubKeyPath); err != nil {
		// Try to find any newly created .pub key
		newKeys := sysutil.FindSSHPublicKeys()
		if len(newKeys) > 0 {
			pubKeyPath = newKeys[len(newKeys)-1].Path
		} else {
			return "", fmt.Errorf("could not find generated public key")
		}
	}

	fmt.Println()
	if data, err := os.ReadFile(pubKeyPath); err == nil {
		fmt.Printf("  %s\n", styles.Dimmed.Render(strings.TrimSpace(string(data))))
	}
	fmt.Println()

	return pubKeyPath, nil
}

func promptGitHubKeyUpload(keyPath string) {
	// Check if gh is available and authenticated
	if err := exec.Command("gh", "auth", "status").Run(); err != nil {
		printManualGitHubInstructions(keyPath)
		return
	}

	fmt.Printf("  Add this key to GitHub? [Y/n]: ")
	input := promptLine()
	fmt.Println()

	if input != "" && strings.ToLower(input) != "y" && strings.ToLower(input) != "yes" {
		return
	}

	hostname, _ := os.Hostname()
	title := "ctdev-" + hostname

	// Add as authentication key
	ghCmd := exec.Command("gh", "ssh-key", "add", keyPath, "--title", title)
	if output, err := ghCmd.CombinedOutput(); err != nil {
		fmt.Println(styles.Warning.Render(fmt.Sprintf("  Failed to add key to GitHub: %s", strings.TrimSpace(string(output)))))
		printManualGitHubInstructions(keyPath)
	} else {
		fmt.Println(styles.Success.Render(fmt.Sprintf("  Added SSH auth key to GitHub as %q", title)))
	}

	// Also add as signing key for commit verification
	ghSignCmd := exec.Command("gh", "ssh-key", "add", keyPath, "--title", title+"-signing", "--type", "signing")
	if output, err := ghSignCmd.CombinedOutput(); err != nil {
		fmt.Println(styles.Warning.Render(fmt.Sprintf("  Failed to add signing key to GitHub: %s", strings.TrimSpace(string(output)))))
	} else {
		fmt.Println(styles.Success.Render(fmt.Sprintf("  Added SSH signing key to GitHub as %q", title+"-signing")))
	}
	fmt.Println()
}

func printManualGitHubInstructions(keyPath string) {
	hostname, _ := os.Hostname()
	fmt.Println()
	fmt.Println("  Add this key to GitHub:")
	fmt.Println("    1. Go to https://github.com/settings/ssh/new")
	fmt.Printf("    2. Title: ctdev-%s\n", hostname)
	fmt.Println("    3. Key type: Authentication and Signing")
	fmt.Println("    4. Paste your public key (shown above)")
	fmt.Println()
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
	selected := flagAWSProfile

	if selected == "" {
		profiles, err := sysutil.ReadAWSProfiles()
		if err != nil || len(profiles) == 0 {
			fmt.Println("No AWS profiles found in ~/.aws/config. Set up AWS CLI first: aws configure sso")
			return nil
		}

		fmt.Println("AWS Profiles:")
		for i, p := range profiles {
			fmt.Printf("  %d) %s\n", i+1, p)
		}
		fmt.Printf("Select: ")
		choice := promptChoice(1)

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
