//go:build unix

package sysutil

import (
	"os/exec"
	"syscall"
	"time"
)

// configureProcessGroup puts the child in its own process group and kills the
// entire group on context cancellation. Without this, cancelling a shell
// command whose stdout is a bytes.Buffer leaves grandchildren holding the pipe
// open until they finish on their own.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 500 * time.Millisecond
}
