//go:build !windows

package claudecli

import (
	"os"
	"os/exec"
	"syscall"
)

// Launch replaces this process with an interactive Claude Code session in
// root, started on prompt. Replacing rather than waiting keeps ctdev's
// Ctrl-C handling out of the session, where Ctrl-C interrupts Claude.
func Launch(root, prompt string) error {
	bin, err := exec.LookPath(Bin)
	if err != nil {
		return err
	}
	if err := os.Chdir(root); err != nil {
		return err
	}
	return syscall.Exec(bin, []string{Bin, prompt}, os.Environ())
}
