package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/component"
	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	tuiinfo "github.com/ConnerTechnology/ctdev/ctdev/tui/info"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

var repoOnce sync.Once
var cachedRepoRoot string

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show system information",
	RunE:  runInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)
}

func repoRoot() string {
	repoOnce.Do(func() {
		if root := os.Getenv("DOTFILES_ROOT"); root != "" {
			cachedRepoRoot = root
			return
		}
		if repoPath != "" {
			cachedRepoRoot = repoPath
			return
		}
		// Try relative to executable (works when running from repo: ./ctdev)
		exe, _ := os.Executable()
		candidate := filepath.Dir(filepath.Dir(exe))
		if _, err := os.Stat(filepath.Join(candidate, "CLAUDE.md")); err == nil {
			cachedRepoRoot = candidate
			return
		}
		home, _ := os.UserHomeDir()
		cachedRepoRoot = checkoutUnder(filepath.Join(home, "Repos", "github.com", "ConnerTechnology"))
	})
	return cachedRepoRoot
}

// checkoutUnder returns the checkout inside the org folder. The checkout was
// called "dotfiles" before the repo was renamed; machines that have not moved
// the folder yet still have it there.
func checkoutUnder(org string) string {
	root := filepath.Join(org, "ctdev")
	if _, err := os.Stat(root); err != nil {
		return filepath.Join(org, "dotfiles")
	}
	return root
}

func runInfo(cmd *cobra.Command, args []string) error {
	sysInfo := platform.GatherSystemInfo(repoRoot())

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

	width, isTTY := infoTerminalSize()
	noColor := os.Getenv("NO_COLOR") != ""
	// info renders directly (not via Bubble Tea), so adapt the palette to the
	// terminal background here. Only query a real terminal when we'll actually use
	// color; the query has a 2s timeout and falls back to the dark default.
	if isTTY && !noColor {
		styles.SetDarkBackground(lipgloss.HasDarkBackground(os.Stdin, os.Stdout))
	}
	output := tuiinfo.Render(sysInfo, version, components, width)
	// Strip color when piped/redirected, or when NO_COLOR is set (no-color.org).
	if !isTTY || noColor {
		output = ansi.Strip(output)
	}
	fmt.Print(output)
	return nil
}

// infoTerminalSize returns the usable column width and whether stdout is a TTY.
// For a real terminal it queries the actual size; when piped it honors $COLUMNS
// if set, otherwise defaults to 80.
func infoTerminalSize() (width int, isTTY bool) {
	fd := os.Stdout.Fd()
	if term.IsTerminal(fd) {
		if w, _, err := term.GetSize(fd); err == nil && w > 0 {
			return w, true
		}
		return 80, true
	}
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if w, err := strconv.Atoi(cols); err == nil && w > 0 {
			return w, false
		}
	}
	return 80, false
}
