//go:build unix

package backup

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// resticProcess runs restic in its own session, with no controlling
// terminal, so the ssh an SFTP destination starts can't take the terminal's
// foreground from a full-screen screen. Cancelling sends restic an interrupt,
// which it answers by removing its lock; a SIGKILL would leave the lock to
// block other computers until restic calls it stale.
func resticProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	// Removing the lock is one request to the destination; 15 seconds covers
	// a slow remote one without leaving someone who pressed Ctrl-C waiting.
	cmd.WaitDelay = 15 * time.Second
}
