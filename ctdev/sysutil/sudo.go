package sysutil

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
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

// sudoArgv builds the argv for running name as root: the bare command when we
// already are root, otherwise `sudo [-n] name args...`. Keeping this pure means
// the wrapper logic is testable without executing anything, and SudoRun and
// SudoNoPrompt can't drift apart.
func sudoArgv(noPrompt bool, name string, args []string) []string {
	if IsRoot() {
		return append([]string{name}, args...)
	}
	argv := []string{"sudo"}
	if noPrompt {
		argv = append(argv, "-n")
	}
	return append(append(argv, name), args...)
}

// SudoNoPrompt builds a command that runs name as root but never asks for a
// password: `sudo -n` for a normal user, the bare command when we already are
// root. For read-only probes that must not stall — a missing password is a
// failed probe, not a prompt.
func SudoNoPrompt(ctx context.Context, name string, args ...string) *exec.Cmd {
	argv := sudoArgv(true, name, args)
	return exec.CommandContext(ctx, argv[0], argv[1:]...)
}

const (
	// sudoRefreshInterval is comfortably inside the shortest default
	// timestamp_timeout we care about (5 minutes on macOS, 15 on Linux).
	sudoRefreshInterval = 60 * time.Second
	// sudoRefreshMaxFailures stops the refresher once it's clearly never going
	// to work — timestamp_timeout can be 0, or the sudoers rule can be
	// command-scoped, and every failed attempt is a syslog line.
	sudoRefreshMaxFailures = 3
)

// KeepSudoAlive refreshes the cached sudo credential in the background until ctx
// is done. A long `brew upgrade` or `apt` run easily outlives macOS's 5-minute
// default timestamp_timeout, and a credential expiring mid-run reintroduces the
// very hang that caching it was meant to prevent.
//
// The refresher is deliberately silent: `sudo -n -v` writes to stderr when it
// fails, and that would land in the middle of a progress TUI frame.
func KeepSudoAlive(ctx context.Context) {
	if IsRoot() {
		return
	}
	go func() {
		ticker := time.NewTicker(sudoRefreshInterval)
		defer ticker.Stop()
		failures := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			cmd := exec.CommandContext(ctx, "sudo", "-n", "-v")
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Run(); err != nil {
				failures++
				if failures >= sudoRefreshMaxFailures {
					return
				}
				continue
			}
			failures = 0
		}
	}()
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
