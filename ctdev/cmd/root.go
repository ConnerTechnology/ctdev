package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

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
}

func Execute() error {
	rootCmd.Version = version
	// Strip ANSI styling globally when the user asks (NO_COLOR, no-color.org)
	// or when stdout isn't a terminal — logs and pipes shouldn't collect
	// escape codes.
	if os.Getenv("NO_COLOR") != "" || !term.IsTerminal(os.Stdout.Fd()) {
		styles.Disable()
	}
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

// ensureSudo caches sudo credentials before starting a TUI.
// Scripts using maybe_sudo/sudo will hang if stdin isn't connected,
// which happens once Bubble Tea takes over the terminal.
func ensureSudo() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	// Check if we already have cached credentials
	if err := exec.Command("sudo", "-n", "true").Run(); err == nil {
		return nil
	}
	fmt.Println("Some components require sudo. Please enter your password:")
	cmd := exec.Command("sudo", "-v")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
