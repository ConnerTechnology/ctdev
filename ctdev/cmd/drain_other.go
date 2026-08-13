//go:build !linux && !darwin

package cmd

// drainStdin is a no-op outside Linux and macOS. It exists to clean up after
// Bubble Tea, and the only command that runs on other platforms is `ctdev
// diagnose`, which never starts a TUI.
func drainStdin() {}
