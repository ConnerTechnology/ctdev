package cmd

import (
	"fmt"
	"os/exec"
	"strings"

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

	fmt.Println("Git Configuration")
	fmt.Println()

	scope := "global"
	if flagGitLocal {
		scope = "local"
	}
	fmt.Printf("  Scope: %s\n\n", scope)

	// Simple prompt-based input
	fmt.Printf("  Name [%s]: ", currentName)
	var name string
	fmt.Scanln(&name)
	if name == "" {
		name = currentName
	}

	fmt.Printf("  Email [%s]: ", currentEmail)
	var email string
	fmt.Scanln(&email)
	if email == "" {
		email = currentEmail
	}

	return setGitConfig(name, email, flagGitLocal)
}

func showGitConfig() error {
	name := getGitConfig("user.name", false)
	email := getGitConfig("user.email", false)
	localName := getGitConfig("user.name", true)
	localEmail := getGitConfig("user.email", true)

	fmt.Println("Git Configuration")
	fmt.Println()
	fmt.Printf("  %-15s %s\n", "Global Name:", name)
	fmt.Printf("  %-15s %s\n", "Global Email:", email)
	if localName != "" && localName != name {
		fmt.Printf("  %-15s %s\n", "Local Name:", localName)
	}
	if localEmail != "" && localEmail != email {
		fmt.Printf("  %-15s %s\n", "Local Email:", localEmail)
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
			fmt.Printf("  Set user.name = %s\n", name)
		}
	}

	if email != "" {
		if flagDryRun {
			fmt.Printf("  [dry-run] git config %s user.email %q\n", scope, email)
		} else {
			if err := exec.Command("git", "config", scope, "user.email", email).Run(); err != nil {
				return fmt.Errorf("failed to set user.email: %w", err)
			}
			fmt.Printf("  Set user.email = %s\n", email)
		}
	}

	return nil
}
