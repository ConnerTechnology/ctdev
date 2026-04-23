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
	return cmd.Run()
}

// SudoRun executes a command with sudo.
func SudoRun(ctx context.Context, o Opts, name string, args ...string) error {
	sudoArgs := append([]string{name}, args...)
	return Run(ctx, o, "sudo", sudoArgs...)
}
