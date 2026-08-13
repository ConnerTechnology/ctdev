package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version      string
	dotfilesPath string // set via SetDotfilesPath at startup

	flagVerbose bool
	flagDryRun  bool
	flagForce   bool
	flagBatch   bool
)

func SetVersion(v string) {
	version = v
}

func SetDotfilesPath(p string) {
	dotfilesPath = p
}

var rootCmd = &cobra.Command{
	Use:     "ctdev",
	Short:   "Development environment manager",
	Long:    "ctdev manages your development environment — install components, update packages, and configure your system.",
	Version: "",
	// Runtime failures ("2 components failed") aren't usage errors — don't
	// follow them with a page of flags, and let main print the error exactly
	// once. Cobra still prints usage for genuine arg/flag parse errors.
	SilenceUsage:      true,
	SilenceErrors:     true,
	PersistentPreRunE: guardWindows,
}

// windowsCommands are the only commands that run on Windows. ctdev manages
// Linux and macOS machines; the Windows build exists so `ctdev doctor` can
// look at someone else's laptop. Everything that installs or configures refuses
// up front here rather than failing halfway through with a confusing error
// about a missing package manager.
var windowsCommands = map[string]bool{
	"ctdev":      true, // bare `ctdev`, which prints help
	"doctor":     true,
	"help":       true,
	"completion": true,
	"__complete": true,
}

func guardWindows(cmd *cobra.Command, _ []string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	name := topLevelName(cmd)
	if windowsCommands[name] {
		return nil
	}
	return fmt.Errorf("ctdev %s is not supported on Windows — only 'ctdev doctor' runs here", name)
}

// topLevelName returns the name of cmd's outermost ancestor below the root, so
// `ctdev backup test` reports "backup". The root itself reports "ctdev".
func topLevelName(cmd *cobra.Command) string {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Parent() == nil || c.Parent().Parent() == nil {
			return c.Name()
		}
	}
	return ""
}

func Execute() error {
	rootCmd.Version = version
	// Strip ANSI styling globally when the user asks (NO_COLOR, no-color.org)
	// or when stdout isn't a terminal — logs and pipes shouldn't collect
	// escape codes.
	if os.Getenv("NO_COLOR") != "" || !term.IsTerminal(os.Stdout.Fd()) {
		styles.Disable()
	}
	defer setupAskpass()()
	setupHomebrewEnv()
	// Bind SIGINT/SIGTERM to a context so Ctrl-C cancels in-flight installs,
	// updates, and other long-running shell-outs via the ctx threaded through
	// sysutil. A second signal aborts hard (signal.NotifyContext stops trapping
	// once the context is canceled).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return rootCmd.ExecuteContext(ctx)
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&flagDryRun, "dry-run", "n", false, "preview changes without applying")
	rootCmd.PersistentFlags().BoolVarP(&flagForce, "force", "f", false, "force re-run install scripts")
	rootCmd.PersistentFlags().BoolVar(&flagBatch, "batch", false, "non-interactive mode")

}

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(os.ExpandEnv("$HOME/.config/ctdev"))
	viper.AutomaticEnv()
	_ = viper.ReadInConfig()
}

// sudoPlan decides what ensureSudo should do for a given access level. Split out
// as a pure function because ensureSudo itself shells out and can't be tested.
// An empty warn with needPrompt false means "nothing to do, we already have
// root".
func sudoPlan(access sysutil.SudoAccess, batch bool) (needPrompt bool, warn string) {
	switch access {
	case sysutil.AlreadyRoot, sysutil.SudoCached:
		return false, ""
	case sysutil.SudoUnavailable:
		return false, "sudo is unavailable here"
	}
	// SudoNeedsPassword: only a terminal can supply one.
	if batch {
		return false, "no terminal to enter a sudo password on"
	}
	return true, ""
}

// ensureSudo caches sudo credentials before starting a TUI: commands that shell
// out through sudo would otherwise prompt on /dev/tty while Bubble Tea holds the
// terminal in raw mode, and hang. Callers gate it on work that will actually use
// root — see component.InstallNeedsRoot and updateNeedsRoot.
//
// This matters just as much on macOS as on Linux: Homebrew shells out to sudo
// itself for casks that ship a pkg payload or a system extension, so a cask
// install with no cached credential hangs exactly the same way.
//
// Reaching root is best-effort. When we already are root, when there is no sudo
// to run, when the environment forbids escalation (a container started with
// no-new-privileges), or when nothing can type a password, it warns and returns
// nil: whatever genuinely needs root then fails with its own error, instead of
// ctdev refusing to do the part of the run that never needed it. Only a prompt
// the user fails or cancels is an error.
func ensureSudo(ctx context.Context) error {
	access := sysutil.CheckSudoAccess(ctx)
	needPrompt, warn := sudoPlan(access, isBatchMode())
	if warn != "" {
		warnNoRoot(warn)
		return nil
	}
	if !needPrompt {
		// Already cached — but it can still expire mid-run, so keep it warm.
		if access == sysutil.SudoCached {
			startSudoKeepalive(ctx)
		}
		return nil
	}

	fmt.Println("Some steps require sudo. Please enter your password:")
	cmd := exec.Command("sudo", "-v")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	startSudoKeepalive(ctx)
	return nil
}

// sudoKeepaliveOnce guards the refresher so the 17 ensureSudo call sites can't
// start a herd of goroutines. The keepalive lives for the process, which for a
// short-lived CLI is the whole run.
var sudoKeepaliveOnce sync.Once

func startSudoKeepalive(ctx context.Context) {
	sudoKeepaliveOnce.Do(func() { sysutil.KeepSudoAlive(ctx) })
}

// rootHint names the things that need root on this OS, for the warning below.
func rootHint() string {
	if runtime.GOOS == "darwin" {
		return "packages, /usr/local, casks that install a system extension"
	}
	return "packages, /usr/local, systemd"
}

func warnNoRoot(reason string) {
	fmt.Println(styles.Warning.Render("Continuing without root: " + reason + "."))
	fmt.Println(styles.Dimmed.Render("  Anything that needs it (" + rootHint() + ") will report a failure."))
}

// resetTerminal cleans up escape sequences that Bubble Tea v2 may leak on exit
// (kitty keyboard protocol and synchronized output queries).
// It also drains any pending DECRPM responses from stdin so they don't leak
// into the shell prompt after ctdev exits.
func resetTerminal() {
	fmt.Print("\033[?2026l\033[?2027l")
	drainStdin()
}

// drainStdin is implemented per-platform in drain_linux.go and drain_darwin.go.

func isBatchMode() bool {
	if flagBatch {
		return true
	}
	// Screen-reader users set ACCESSIBLE to drop the live TUIs in favor of the
	// plain, line-by-line path (the Charm-ecosystem convention).
	if os.Getenv("ACCESSIBLE") != "" {
		return true
	}
	// A TUI needs a terminal on both ends: prompts read stdin, and alt-screen
	// escape sequences would garble a piped/redirected stdout (`ctdev … | tee`).
	if !term.IsTerminal(os.Stdout.Fd()) {
		return true
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return fi.Mode()&os.ModeCharDevice == 0
}
