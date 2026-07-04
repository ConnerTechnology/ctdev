//go:build !linux

package cmd

import "os"

// createdUnix falls back to mtime on platforms where we don't read birth time.
// (macOS exposes it via Stat_t.Birthtimespec if this is ever needed there.)
func createdUnix(path string, fi os.FileInfo) int64 {
	return fi.ModTime().Unix()
}
