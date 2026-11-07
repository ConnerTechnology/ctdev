//go:build windows

package claudecli

import (
	"os"
	"os/exec"
	"os/signal"
)

// Launch runs an interactive Claude Code session in root, started on prompt,
// and returns when it ends. Windows has no exec, so ctdev waits, ignoring
// Ctrl-C, which belongs to the session.
func Launch(root, prompt string) error {
	signal.Ignore(os.Interrupt)
	cmd := exec.Command(Bin, prompt)
	cmd.Dir = root
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
