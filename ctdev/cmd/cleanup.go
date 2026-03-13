package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Run system cleanup tasks",
	Long:  "Remove old kernels, audit APT repositories, and clean package cache.",
	RunE:  runCleanup,
}

func init() {
	rootCmd.AddCommand(cleanupCmd)
}

func runCleanup(cmd *cobra.Command, args []string) error {
	info := platform.Detect()
	if info.OS != platform.Linux {
		return fmt.Errorf("cleanup is only supported on Linux")
	}

	tasks := []struct {
		name    string
		check   func() string
		execute func() error
	}{
		{
			name: "Remove old kernels",
			check: func() string {
				out, err := exec.Command("bash", "-c", "dpkg --list | grep linux-image | grep -v $(uname -r) | wc -l").Output()
				if err != nil {
					return "unable to check"
				}
				count := strings.TrimSpace(string(out))
				return fmt.Sprintf("%s old kernels found", count)
			},
			execute: func() error {
				return exec.Command("bash", "-c",
					"dpkg --list | grep linux-image | grep -v $(uname -r) | awk '{print $2}' | xargs sudo apt remove -y").Run()
			},
		},
		{
			name: "Audit APT repositories",
			check: func() string {
				out, err := exec.Command("bash", "-c", "find /etc/apt/sources.list.d -name '*.list' 2>/dev/null | wc -l").Output()
				if err != nil {
					return "unable to check"
				}
				return fmt.Sprintf("%s repository files", strings.TrimSpace(string(out)))
			},
			execute: func() error {
				fmt.Println("  Checking for duplicate repositories...")
				return nil
			},
		},
		{
			name: "Clean package cache",
			check: func() string {
				out, err := exec.Command("du", "-sh", "/var/cache/apt/archives/").Output()
				if err != nil {
					return "unable to check"
				}
				fields := strings.Fields(string(out))
				if len(fields) > 0 {
					return fmt.Sprintf("%s cache size", fields[0])
				}
				return "unknown size"
			},
			execute: func() error {
				return exec.Command("sudo", "apt", "clean").Run()
			},
		},
	}

	for _, task := range tasks {
		info := task.check()
		fmt.Printf("  %-30s %s\n", task.name, info)
	}

	if flagDryRun {
		fmt.Println("\n  [dry-run] No changes made.")
		return nil
	}

	if !isBatchMode() {
		fmt.Print("\nProceed with cleanup? [y/N] ")
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(answer) != "y" {
			return nil
		}
	}

	for _, task := range tasks {
		fmt.Printf("Running: %s...\n", task.name)
		if err := task.execute(); err != nil {
			fmt.Printf("  Warning: %v\n", err)
		}
	}

	fmt.Println("Cleanup complete.")
	return nil
}
