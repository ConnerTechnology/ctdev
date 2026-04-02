package cmd

import (
	"os"

	"golang.org/x/sys/unix"
)

func drainStdin() {
	fd := int(os.Stdin.Fd())

	old, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return
	}
	raw := *old
	raw.Lflag &^= unix.ICANON | unix.ECHO
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TIOCSETA, &raw); err != nil {
		return
	}

	buf := make([]byte, 256)
	for {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, _ := unix.Poll(fds, 200)
		if n <= 0 {
			break
		}
		unix.Read(fd, buf)
	}

	_ = unix.IoctlSetTermios(fd, unix.TIOCSETA, old)
}
