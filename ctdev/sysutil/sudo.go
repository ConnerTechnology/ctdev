package sysutil

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
)

// SudoAccess describes how this process can reach root, if at all.
type SudoAccess int

const (
	// AlreadyRoot: we run as uid 0, so privileged commands need no wrapper.
	// This is the normal case inside a container.
	AlreadyRoot SudoAccess = iota
	// SudoCached: sudo elevates without a password (cached credentials or a
	// NOPASSWD sudoers rule).
	SudoCached
	// SudoNeedsPassword: sudo works, but a password has to be typed first.
	SudoNeedsPassword
	// SudoUnavailable: root is out of reach — no sudo on PATH, no sudoers
	// entry, or an environment that forbids privilege escalation (a container
	// started with no-new-privileges, or one that dropped setuid).
	SudoUnavailable
)

// IsRoot reports whether this process already has root privileges.
func IsRoot() bool {
	return os.Geteuid() == 0
}

// CanElevateQuietly reports whether a root command can run right now without
// prompting for anything. Probes and status readouts use this to stay silent.
func CanElevateQuietly(ctx context.Context) bool {
	switch CheckSudoAccess(ctx) {
	case AlreadyRoot, SudoCached:
		return true
	}
	return false
}

// SudoNoPrompt builds a command that runs name as root but never asks for a
// password: `sudo -n` for a normal user, the bare command when we already are
// root. For read-only probes that must not stall — a missing password is a
// failed probe, not a prompt.
func SudoNoPrompt(ctx context.Context, name string, args ...string) *exec.Cmd {
	if IsRoot() {
		return exec.CommandContext(ctx, name, args...)
	}
	return exec.CommandContext(ctx, "sudo", append([]string{"-n", name}, args...)...)
}

// CheckSudoAccess probes for root access without prompting for anything.
func CheckSudoAccess(ctx context.Context) SudoAccess {
	if IsRoot() {
		return AlreadyRoot
	}
	if !CommandExists("sudo") {
		return SudoUnavailable
	}
	cmd := exec.CommandContext(ctx, "sudo", "-n", "true")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// sudo translates its diagnostics; C keeps the phrase we match on stable.
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	if err := cmd.Run(); err == nil {
		return SudoCached
	}
	// "sudo: a password is required" is the one failure that means sudo itself
	// works. Everything else — not in sudoers, no-new-privileges, a stripped
	// setuid bit — means asking for a password would only stall the caller.
	if strings.Contains(stderr.String(), "password is required") {
		return SudoNeedsPassword
	}
	return SudoUnavailable
}
