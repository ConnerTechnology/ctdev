package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/state"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show system information",
	RunE:  runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)
}

func dotfilesRoot() string {
	exe, _ := os.Executable()
	return filepath.Dir(filepath.Dir(exe))
}

func runInfo(cmd *cobra.Command, args []string) error {
	info := platform.GatherSystemInfo(dotfilesRoot())
	markers := state.DefaultMarkerStore()
	installed, _ := markers.List()

	fmt.Printf("System Information\n\n")
	fmt.Printf("  %-20s %s\n", "OS", formatOS(info.Platform))
	fmt.Printf("  %-20s %s\n", "Architecture", info.Platform.Arch)
	fmt.Printf("  %-20s %s\n", "Package Manager", info.Platform.PackageManager)
	fmt.Printf("  %-20s %s\n", "Shell", info.Shell)
	fmt.Printf("  %-20s %s\n", "Dotfiles", info.DotfilesDir)
	fmt.Printf("  %-20s %s\n", "ctdev", version)
	fmt.Println()
	fmt.Printf("  %-20s %s (%d threads)\n", "CPU", info.CPUModel, info.CPUThreads)
	fmt.Printf("  %-20s %d GB\n", "Memory", info.MemoryGB)
	fmt.Println()
	fmt.Printf("  Components: %d of %d installed\n", len(installed), len(component.Registry))
	if len(installed) > 0 {
		fmt.Printf("  Installed: %s\n", strings.Join(installed, ", "))
	}

	return nil
}

func formatOS(p platform.Info) string {
	if p.Distro != "" {
		return fmt.Sprintf("%s (%s)", p.Distro, p.OS)
	}
	return string(p.OS)
}
