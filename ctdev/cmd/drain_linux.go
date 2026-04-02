package cmd

import (
	"os"

	"golang.org/x/sys/unix"
)

// drainStdin waits for and discards any bytes pending on stdin (e.g. terminal
// DECRPM responses). After Bubble Tea exits, the terminal is back in cooked
// mode where the line discipline buffers input until a newline — but DECRPM
// responses have no newline, so poll() never sees them. We briefly switch to
// raw mode so the bytes flow through, then drain them.
func drainStdin() {
	fd := int(os.Stdin.Fd())

	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return
	}
	raw := *old
	raw.Lflag &^= unix.ICANON | unix.ECHO
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &raw); err != nil {
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

	_ = unix.IoctlSetTermios(fd, unix.TCSETS, old)
}
