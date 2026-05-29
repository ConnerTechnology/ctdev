package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify the bootstrap installation",
	Long:  "Checks that expected tools, services, dotfiles, and remote-access settings are in place. Exits non-zero if anything is missing.",
	RunE:  runVerify,
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}

type verifyCheck struct {
	name   string
	ok     bool
	detail string
}

func runVerify(cmd *cobra.Command, args []string) error {
	var checks []verifyCheck
	add := func(name string, ok bool, detail string) {
		checks = append(checks, verifyCheck{name, ok, detail})
	}

	// Binaries + versions
	bins := []struct {
		name string
		args []string
	}{
		{"git", []string{"--version"}},
		{"curl", []string{"--version"}},
		{"zsh", []string{"--version"}},
		{"tmux", []string{"-V"}},
		{"mosh", []string{"--version"}},
		{"jq", []string{"--version"}},
		{"rg", []string{"--version"}},
		{"node", []string{"--version"}},
		{"docker", []string{"--version"}},
		{"code", []string{"--version"}},
		{"tailscale", []string{"version"}},
		{"claude", []string{"--version"}},
		{"gh", []string{"--version"}},
		{"devcontainer", []string{"--version"}},
	}
	for _, b := range bins {
		path, err := exec.LookPath(b.name)
		if err != nil {
			add(b.name, false, "not on PATH")
			continue
		}
		add(b.name, true, firstLine(runForOutput(path, b.args...)))
	}

	// fd ships as `fdfind` on Debian/Ubuntu (fd-find package).
	if p := firstOnPath("fd", "fdfind"); p != "" {
		add("fd", true, filepath.Base(p))
	} else {
		add("fd", false, "not on PATH (fd-find)")
	}

	// go may live at /usr/local/go/bin even when not yet on PATH.
	if gp := goBinaryPath(); gp != "" {
		add("go", true, firstLine(runForOutput(gp, "version")))
	} else {
		add("go", false, "not found")
	}

	// Services + system state (Linux only)
	if runtime.GOOS == "linux" {
		for _, svc := range []string{"ssh", "tailscaled", "docker"} {
			add("service: "+svc, serviceActive(svc), serviceState(svc))
		}
		add("ufw active", serviceActive("ufw"), "")
		add("suspend masked", allTargetsMasked(), "")
	}

	// Shell + dotfiles
	home, _ := os.UserHomeDir()
	shell := os.Getenv("SHELL")
	add("shell is zsh", strings.HasSuffix(shell, "zsh"), shell)
	add("oh-my-zsh", pathIsDir(filepath.Join(home, ".oh-my-zsh")), "")
	add("pure prompt", pathExists(filepath.Join(home, ".zsh", "functions", "prompt_pure_setup")), "")
	add(".zshrc deployed", pathExists(filepath.Join(home, ".zshrc")), "")
	add("dx wrapper", pathExists(filepath.Join(home, ".local", "bin", "dx")), "")

	// Render
	fmt.Println(styles.Title.Render("ctdev verify"))
	fmt.Println()
	failed := 0
	for _, c := range checks {
		mark := styles.Success.Render("✓")
		if !c.ok {
			mark = styles.Error.Render("✗")
			failed++
		}
		line := fmt.Sprintf("  %s %s", mark, c.name)
		if c.detail != "" {
			line += "  " + styles.Dimmed.Render(c.detail)
		}
		fmt.Println(line)
	}
	fmt.Println()

	if failed > 0 {
		return fmt.Errorf("%d of %d checks failed", failed, len(checks))
	}
	fmt.Println(styles.Success.Render(fmt.Sprintf("All %d checks passed.", len(checks))))
	return nil
}

// runForOutput runs a command and returns combined output, tolerating tools
// that print their version to stderr or exit non-zero.
func runForOutput(path string, args ...string) string {
	out, _ := exec.Command(path, args...).CombinedOutput()
	return string(out)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func firstOnPath(names ...string) string {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

func goBinaryPath() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	if p := "/usr/local/go/bin/go"; pathExists(p) {
		return p
	}
	return ""
}

func serviceActive(name string) bool {
	return serviceState(name) == "active"
}

func serviceState(name string) string {
	out, _ := exec.Command("systemctl", "is-active", name).Output()
	return strings.TrimSpace(string(out))
}

// allTargetsMasked reports whether sleep.target is masked as a proxy for the
// whole never-suspend set.
func allTargetsMasked() bool {
	out, _ := exec.Command("systemctl", "is-enabled", "sleep.target").Output()
	return strings.TrimSpace(string(out)) == "masked"
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func pathIsDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
