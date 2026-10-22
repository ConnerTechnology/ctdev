//go:build !unix

package backup

import "os"

// flock is a no-op where there's no scheduler to run backups concurrently
// with: ctdev schedules backups on Linux and macOS only.
func flock(*os.File, bool) error { return nil }
