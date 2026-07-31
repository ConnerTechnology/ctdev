package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	comp "github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify this machine's ctdev-managed setup",
	Long:  "Checks that installed components, homelab containers, backup timers, dotfiles, and remote-access settings are in place. Exits non-zero if anything is missing.",
	RunE:  runVerify,
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}

type verifyCheck struct {
	name   string
	ok     bool
	detail string
	// soft marks an informational check: it's rendered but a failure never
	// fails the run. Used for system settings (ufw/suspend) that come from
	// `configure` categories a given machine may legitimately opt out of.
	soft bool
}

// compInstalled reports whether a registered component is present on this
// machine, using the registry's own detection. verify gates its checks on this
// so a headless/homelab node isn't penalized for desktop tooling it was never
// meant to have.
func compInstalled(name string) bool {
	c := comp.FindByName(name)
	return c != nil && c.IsInstalled()
}

func runVerify(cmd *cobra.Command, args []string) error {
	var checks []verifyCheck
	add := func(name string, ok bool, detail string) {
		checks = append(checks, verifyCheck{name: name, ok: ok, detail: detail})
	}
	info := func(name string, ok bool, detail string) {
		checks = append(checks, verifyCheck{name: name, ok: ok, detail: detail, soft: true})
	}

	// Binaries + versions. `owner` gates the check on a ctdev component being
	// installed; an empty owner means the tool isn't ctdev-managed, so it's
	// reported informationally (present → ✓, absent → skipped, never a failure).
	bins := []struct {
		name  string
		args  []string
		owner string
	}{
		{"git", []string{"--version"}, "git"},
		{"curl", []string{"--version"}, ""},
		{"zsh", []string{"--version"}, "zsh"},
		{"tmux", []string{"-V"}, "tmux"},
		{"mosh", []string{"--version"}, ""},
		{"jq", []string{"--version"}, "jq"},
		{"rg", []string{"--version"}, ""},
		{"node", []string{"--version"}, "node"},
		{"docker", []string{"--version"}, "docker"},
		{"code", []string{"--version"}, "vscode"},
		{"tailscale", []string{"version"}, "tailscale"},
		{"claude", []string{"--version"}, "claude-code"},
		{"gh", []string{"--version"}, "gh"},
		{"devcontainer", []string{"--version"}, "devcontainer"},
	}
	for _, b := range bins {
		// ctdev-managed but not installed here: skip silently.
		if b.owner != "" && !compInstalled(b.owner) {
			continue
		}
		path, err := exec.LookPath(b.name)
		if err != nil {
			if b.owner == "" {
				continue // unmanaged extra that's simply absent — not a failure
			}
			add(b.name, false, "not on PATH")
			continue
		}
		add(b.name, true, firstLine(runForOutput(path, b.args...)))
	}

	// fd ships as `fdfind` on Debian/Ubuntu (fd-find package); unmanaged, so
	// informational only.
	if p := firstOnPath("fd", "fdfind"); p != "" {
		add("fd", true, filepath.Base(p))
	}

	// go may live at /usr/local/go/bin even when not yet on PATH.
	if compInstalled("go") {
		if gp := goBinaryPath(); gp != "" {
			add("go", true, firstLine(runForOutput(gp, "version")))
		} else {
			add("go", false, "not found")
		}
	}

	// Services + system state (Linux only).
	if runtime.GOOS == "linux" {
		svcs := []struct{ unit, owner string }{
			{"ssh", ""}, // remote-access lifeline; checked only if the unit exists
			{"tailscaled", "tailscale"},
			{"docker", "docker"},
		}
		for _, s := range svcs {
			if s.owner != "" && !compInstalled(s.owner) {
				continue
			}
			if s.owner == "" && !serviceExists(s.unit) {
				continue
			}
			add("service: "+s.unit, serviceActive(s.unit), serviceState(s.unit))
		}

		// Homelab compose stacks install as containers — no host binary, no
		// systemd unit — so "container running" is the real health check.
		for _, name := range []string{"pihole", "caddy", "portainer", "beszel"} {
			if !compInstalled(name) {
				continue
			}
			state := "running"
			ok := sysutil.ContainerRunning(name)
			if !ok {
				state = "not running"
			}
			add("container: "+name, ok, state)
		}

		// restic backups: the timer being enabled is what makes them nightly.
		if compInstalled("restic") {
			add("restic binary", sysutil.CommandExists("restic"), "")
			add("restic-backup.timer", unitEnabled("restic-backup.timer"), unitEnabledState("restic-backup.timer"))
		}

		// ufw / never-suspend are applied by `configure`, not a component, and a
		// DNS/proxy node intentionally skips them — report, don't fail.
		if _, err := exec.LookPath("ufw"); err == nil {
			info("ufw active", serviceActive("ufw"), "")
		}
		info("suspend masked", allTargetsMasked(), "")
	}

	// Shell + dotfiles (only when the zsh component is installed).
	if compInstalled("zsh") {
		home, _ := os.UserHomeDir()
		shell := sysutil.LoginShell(cmd.Context())
		add("shell is zsh", strings.HasSuffix(shell, "zsh"), shell)
		add("oh-my-zsh", pathIsDir(filepath.Join(home, ".oh-my-zsh")), "")
		add("pure prompt", pathExists(filepath.Join(home, ".zsh", "functions", "prompt_pure_setup")), "")
		add(".zshrc deployed", pathExists(filepath.Join(home, ".zshrc")), "")
	}
	if compInstalled("devcontainer") {
		home, _ := os.UserHomeDir()
		add("dx wrapper", pathExists(filepath.Join(home, ".local", "bin", "dx")), "")
	}

	// Render
	fmt.Println(styles.Title.Render("ctdev verify"))
	fmt.Println()
	failed := 0
	for _, c := range checks {
		mark := styles.Success.Render("✓")
		if !c.ok {
			if c.soft {
				mark = styles.Dimmed.Render("○")
			} else {
				mark = styles.Error.Render("✗")
				failed++
			}
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

// serviceExists reports whether a systemd unit of the given name is known to the
// system, so verify can skip checking services that were never installed.
func serviceExists(name string) bool {
	out, _ := exec.Command("systemctl", "list-unit-files", name+".service", "--no-legend").Output()
	return strings.Contains(string(out), name+".service")
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

func unitEnabled(name string) bool {
	return unitEnabledState(name) == "enabled"
}

func unitEnabledState(name string) string {
	out, _ := exec.Command("systemctl", "is-enabled", name).Output()
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
