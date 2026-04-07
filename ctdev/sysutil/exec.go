package sysutil

import (
	"fmt"
	"os/exec"
	"strings"
)

// Run executes a command, routing output to opts.Stdout.
// If opts.DryRun, prints the command without executing.
func Run(o Opts, name string, args ...string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] %s %s\n", name, strings.Join(args, " "))
		return nil
	}
	cmd := exec.Command(name, args...)
	cmd.Stdout = o.Stdout
	cmd.Stderr = o.Stdout
	return cmd.Run()
}

// SudoRun executes a command with sudo.
func SudoRun(o Opts, name string, args ...string) error {
	sudoArgs := append([]string{name}, args...)
	return Run(o, "sudo", sudoArgs...)
}

