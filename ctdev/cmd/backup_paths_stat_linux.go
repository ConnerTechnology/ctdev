package cmd

import (
	"os"

	"golang.org/x/sys/unix"
)

// createdUnix returns a path's creation (birth) time in Unix seconds via statx.
// Falls back to mtime when the filesystem doesn't record btime (statx sets the
// STATX_BTIME mask bit only when it's available).
func createdUnix(path string, fi os.FileInfo) int64 {
	var stx unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BTIME, &stx); err == nil {
		if stx.Mask&unix.STATX_BTIME != 0 && stx.Btime.Sec > 0 {
			return stx.Btime.Sec
		}
	}
	return fi.ModTime().Unix()
}
