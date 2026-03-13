package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/state"
	tuiinfo "github.com/ConnerTechnology/dotfiles/ctdev/tui/info"
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
	sysInfo := platform.GatherSystemInfo(dotfilesRoot())
	markers := state.DefaultMarkerStore()
	installed, _ := markers.List()

	output := tuiinfo.Render(sysInfo, version, len(installed), len(component.Registry), installed)
	fmt.Print(output)
	return nil
}
