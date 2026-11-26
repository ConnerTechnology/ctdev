//go:build unix

package backup

import (
	"errors"
	"os"
	"syscall"
)

// flock takes an exclusive flock on f, waiting for it or failing with
// errLocked.
func flock(f *os.File, wait bool) error {
	how := syscall.LOCK_EX
	if !wait {
		how |= syscall.LOCK_NB
	}
	for {
		err := syscall.Flock(int(f.Fd()), how)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.EWOULDBLOCK):
			return errLocked
		}
		return err
	}
}

// funlock explicitly releases the flock on f's open file description before
// Close. Relying on Close alone leaves the lock held briefly if another
// goroutine forked a child process (before execve applies O_CLOEXEC).
func funlock(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
