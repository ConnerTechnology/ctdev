package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var (
	flagGitName  string
	flagGitEmail string
	flagGitLocal bool
	flagGitShow  bool
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure system settings",
}

var configureGitCmd = &cobra.Command{
	Use:   "git",
	Short: "Configure git user settings",
	Long:  "Set git user name and email globally or locally.",
	RunE:  runConfigureGit,
}

func init() {
	configureGitCmd.Flags().StringVar(&flagGitName, "name", "", "git user name")
	configureGitCmd.Flags().StringVar(&flagGitEmail, "email", "", "git user email")
	configureGitCmd.Flags().BoolVar(&flagGitLocal, "local", false, "apply to current repo only")
	configureGitCmd.Flags().BoolVar(&flagGitShow, "show", false, "show current git config")
	configureCmd.AddCommand(configureGitCmd)
	rootCmd.AddCommand(configureCmd)
}

func runConfigureGit(cmd *cobra.Command, args []string) error {
	if flagGitShow {
		return showGitConfig()
	}

	// If flags provided, set directly
	if flagGitName != "" || flagGitEmail != "" {
		return setGitConfig(flagGitName, flagGitEmail, flagGitLocal)
	}

	// Interactive mode
	if isBatchMode() {
		return fmt.Errorf("--name and --email required in batch mode")
	}

	// Get current values
	currentName := getGitConfig("user.name", flagGitLocal)
	currentEmail := getGitConfig("user.email", flagGitLocal)

	labelStyle := lipgloss.NewStyle().Foreground(styles.Subtle).Width(20)
	valueStyle := lipgloss.NewStyle().Foreground(styles.Bright)

	fmt.Println(styles.Title.Render("Git Configuration"))
	fmt.Println()

	scope := "global"
	if flagGitLocal {
		scope = "local"
	}
	fmt.Printf("  %s %s\n\n", labelStyle.Render("Scope:"), valueStyle.Render(scope))

	// Simple prompt-based input
	fmt.Printf("  %s ", styles.Dimmed.Render(fmt.Sprintf("Name [%s]:", currentName)))
	var name string
	fmt.Scanln(&name)
	if name == "" {
		name = currentName
	}

	fmt.Printf("  %s ", styles.Dimmed.Render(fmt.Sprintf("Email [%s]:", currentEmail)))
	var email string
	fmt.Scanln(&email)
	if email == "" {
		email = currentEmail
	}

	return setGitConfig(name, email, flagGitLocal)
}

func showGitConfig() error {
	labelStyle := lipgloss.NewStyle().Foreground(styles.Subtle).Width(20)
	valueStyle := lipgloss.NewStyle().Foreground(styles.Bright)
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.Orange)

	name := getGitConfig("user.name", false)
	email := getGitConfig("user.email", false)
	localName := getGitConfig("user.name", true)
	localEmail := getGitConfig("user.email", true)

	fmt.Println(styles.Title.Render("Git Configuration"))
	fmt.Println()

	fmt.Println(headerStyle.Render("Global"))
	fmt.Printf("  %s %s\n", labelStyle.Render("Name"), valueStyle.Render(name))
	fmt.Printf("  %s %s\n", labelStyle.Render("Email"), valueStyle.Render(email))

	hasLocal := localName != "" || localEmail != ""
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
		fmt.Printf("  %s %s\n", labelStyle.Render("Name"), valueStyle.Render(localNameDisplay))
		fmt.Printf("  %s %s\n", labelStyle.Render("Email"), valueStyle.Render(localEmailDisplay))
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
