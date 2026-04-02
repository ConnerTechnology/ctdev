package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	tuiinfo "github.com/ConnerTechnology/dotfiles/ctdev/tui/info"
	"github.com/spf13/cobra"
)

var dotfilesOnce sync.Once
var cachedDotfilesRoot string

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show system information",
	RunE:  runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)
}

func dotfilesRoot() string {
	dotfilesOnce.Do(func() {
		if root := os.Getenv("DOTFILES_ROOT"); root != "" {
			cachedDotfilesRoot = root
			return
		}
		if dotfilesPath != "" {
			cachedDotfilesRoot = dotfilesPath
			return
		}
		// Try relative to executable (works when running from repo: ./ctdev)
		exe, _ := os.Executable()
		candidate := filepath.Dir(filepath.Dir(exe))
		if _, err := os.Stat(filepath.Join(candidate, "lib", "utils.sh")); err == nil {
			cachedDotfilesRoot = candidate
			return
		}
		home, _ := os.UserHomeDir()
		cachedDotfilesRoot = filepath.Join(home, "Repos", "github.com", "ConnerTechnology", "dotfiles")
	})
	return cachedDotfilesRoot
}

func runInfo(cmd *cobra.Command, args []string) error {
	sysInfo := platform.GatherSystemInfo(dotfilesRoot())

	osType := component.OS(sysInfo.Platform.OS)
	filtered := component.FilterByOS(component.Registry, osType)

	var components []tuiinfo.ComponentInfo
	for i := range filtered {
		components = append(components, tuiinfo.ComponentInfo{
			Name:      filtered[i].Name,
			Category:  string(filtered[i].Category),
			Installed: filtered[i].IsInstalled(),
		})
	}

	width := 80
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if w, err := strconv.Atoi(cols); err == nil {
			width = w
		}
	}
	output := tuiinfo.Render(sysInfo, version, components, width)
	fmt.Print(output)
	return nil
}
