package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/profile"
	"github.com/ConnerTechnology/dotfiles/ctdev/state"
	tuiinfo "github.com/ConnerTechnology/dotfiles/ctdev/tui/info"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
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
		if _, err := os.Stat(filepath.Join(candidate, "CLAUDE.md")); err == nil {
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

	width, isTTY := infoTerminalSize()
	noColor := os.Getenv("NO_COLOR") != ""
	// info renders directly (not via Bubble Tea), so adapt the palette to the
	// terminal background here. Only query a real terminal when we'll actually use
	// color; the query has a 2s timeout and falls back to the dark default.
	if isTTY && !noColor {
		styles.SetDarkBackground(lipgloss.HasDarkBackground(os.Stdin, os.Stdout))
	}
	output := tuiinfo.Render(sysInfo, version, components, machineProfile(), width)
	// Strip color when piped/redirected, or when NO_COLOR is set (no-color.org).
	if !isTTY || noColor {
		output = ansi.Strip(output)
	}
	fmt.Print(output)
	return nil
}

// inferredProfileFloor is how much of a profile must already be installed
// before we're willing to guess a machine was built from it. Below that, the
// "closest match" is noise — a Pi profile would otherwise claim a desktop.
const inferredProfileFloor = 0.5

// machineProfile names the profile this host was built from: the one `ctdev
// apply` recorded, or — for a machine composed by hand — the closest match by
// components installed. Returns nil when neither applies.
func machineProfile() *tuiinfo.ProfileInfo {
	installed := component.InstalledSet()

	if name := state.AppliedProfile(); name != "" {
		// A recorded profile is authoritative even at 0 components installed:
		// that's drift worth showing, not a reason to guess something else.
		if p, err := profile.Load(name); err == nil {
			return profileStats(p, installed, false)
		}
	}

	var best *tuiinfo.ProfileInfo
	for _, p := range profile.List() {
		st := profileStats(&p, installed, true)
		if st.Total == 0 || float64(st.Installed)/float64(st.Total) < inferredProfileFloor {
			continue
		}
		if best == nil || st.Installed > best.Installed {
			best = st
		}
	}
	return best
}

func profileStats(p *profile.Profile, installed map[string]bool, inferred bool) *tuiinfo.ProfileInfo {
	st := &tuiinfo.ProfileInfo{Name: p.Name, Inferred: inferred, Total: len(p.Components)}
	for _, c := range p.Components {
		if installed[c] {
			st.Installed++
		}
	}
	return st
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
