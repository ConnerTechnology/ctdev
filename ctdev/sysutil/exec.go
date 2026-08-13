package sysutil

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Run executes a command, routing output to opts.Stdout and respecting
// ctx for cancellation. If opts.DryRun, prints the command without executing.
func Run(ctx context.Context, o Opts, name string, args ...string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] %s %s\n", name, strings.Join(args, " "))
		return nil
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = o.Stdout
	cmd.Stderr = o.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", commandLabel(name, args), err)
	}
	return nil
}

// commandLabel names the command for an error message. A bare "exit status 1"
// is useless by the time it reaches a summary screen, and for a sudo invocation
// the interesting name is the wrapped command, not "sudo" — so skip past sudo's
// own flags (`-n`) to find it.
func commandLabel(name string, args []string) string {
	if name != "sudo" {
		return name
	}
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return "sudo " + a
		}
	}
	return name
}

// SudoRun executes a command with root privileges — through sudo as a normal
// user, directly when we already are root. Containers routinely run as root
// with no sudo installed at all, where wrapping unconditionally would fail on
// "sudo: executable file not found".
func SudoRun(ctx context.Context, o Opts, name string, args ...string) error {
	if IsRoot() {
		return Run(ctx, o, name, args...)
	}
	// Say what's missing rather than letting exec report `"sudo": executable
	// file not found` — a container image without sudo is the common cause.
	if !o.DryRun && !CommandExists("sudo") {
		return fmt.Errorf("%s needs root, but there is no sudo to run it with", name)
	}
	argv := sudoArgv(o.NoSudoPrompt, name, args)
	return Run(ctx, o, argv[0], argv[1:]...)
}
