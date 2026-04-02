package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sys/unix"
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
	return rootCmd.Execute()
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
	viper.AddConfigPath("$HOME/.config/ctdev")
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

// drainStdin waits for and discards any bytes pending on stdin (e.g. terminal
// DECRPM responses). Uses poll(2) to wait for data, then flushes the input queue.
func drainStdin() {
	fd := int(os.Stdin.Fd())
	buf := make([]byte, 256)
	for {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, _ := unix.Poll(fds, 200) // wait up to 200ms for data
		if n <= 0 {
			break // timeout — no more data coming
		}
		unix.Read(fd, buf)
	}
}

func isBatchMode() bool {
	if flagBatch {
		return true
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return fi.Mode()&os.ModeCharDevice == 0
}

