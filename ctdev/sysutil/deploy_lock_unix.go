//go:build unix

package sysutil

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// lockDeployDir serializes ctdev's backup and activation steps on an existing
// directory inode. Closing it releases the lock even after an interrupted run.
func lockDeployDir(dir string, wait bool) (func(), error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	how := syscall.LOCK_EX
	if !wait {
		how |= syscall.LOCK_NB
	}
	for {
		err = syscall.Flock(int(f.Fd()), how)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("lock %s for deployment: %w", dir, err)
	}
	return func() { f.Close() }, nil
}
