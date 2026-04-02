package sysutil

import "io"

// Opts controls behavior of sysutil operations.
type Opts struct {
	Stdout io.Writer // output destination (progress TUI captures this)
	DryRun bool      // print what would happen but don't execute
}
