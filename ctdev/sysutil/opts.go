package sysutil

import "io"

// Opts controls behavior of sysutil operations.
type Opts struct {
	Stdout io.Writer // output destination (progress TUI captures this)
	DryRun bool      // print what would happen but don't execute

	// NoSudoPrompt makes SudoRun pass `-n`, so a missing credential fails
	// immediately instead of prompting. Set it wherever a Bubble Tea program
	// owns the terminal: sudo would otherwise prompt on /dev/tty, where the
	// prompt is invisible (output is piped to the TUI) and the keystrokes are
	// eaten by the TUI's input reader, hanging the run until Ctrl-C.
	NoSudoPrompt bool
}
