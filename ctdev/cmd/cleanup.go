package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

type duplicateSource struct {
	Line  string
	Files []string
}

func findDuplicateSourceLines(fileContents map[string]string) []duplicateSource {
	seen := make(map[string][]string)
	for filename, content := range fileContents {
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			seen[line] = append(seen[line], filename)
		}
	}
	var dups []duplicateSource
	for line, files := range seen {
		if len(files) > 1 {
			dups = append(dups, duplicateSource{Line: line, Files: files})
		}
	}
	return dups
}

func readAPTSourceFiles() map[string]string {
	dir := "/etc/apt/sources.list.d"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	files := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".list") && !strings.HasSuffix(e.Name(), ".sources")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		files[e.Name()] = string(data)
	}
	return files
}

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
				files := readAPTSourceFiles()
				dups := findDuplicateSourceLines(files)
				if len(dups) == 0 {
					fmt.Println("  No duplicate repositories found.")
					return nil
				}
				for _, d := range dups {
					fmt.Printf("  Duplicate: %s\n    In: %s\n", d.Line, strings.Join(d.Files, ", "))
				}
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

	labelStyle := lipgloss.NewStyle().Foreground(styles.Subtle).Width(30)
	valueStyle := lipgloss.NewStyle().Foreground(styles.Bright)

	for _, task := range tasks {
		info := task.check()
		fmt.Printf("  %s %s\n", labelStyle.Render(task.name), valueStyle.Render(info))
	}

	if flagDryRun {
		fmt.Println(styles.Dimmed.Render("\n  [dry-run] No changes made."))
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
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("Running: %s...", task.name)))
		if err := task.execute(); err != nil {
			fmt.Printf("  %s\n", styles.Warning.Render(fmt.Sprintf("Warning: %v", err)))
		}
	}

	fmt.Println(styles.Success.Render("Cleanup complete."))
	return nil
}
