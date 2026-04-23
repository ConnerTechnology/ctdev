package sysutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CommandExists checks if a command is available on PATH.
func CommandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// ServiceEnable enables a systemd service.
func ServiceEnable(ctx context.Context, o Opts, name string) error {
	return SudoRun(ctx, o, "systemctl", "enable", name+".service")
}

// ServiceDisable stops and disables a systemd service.
func ServiceDisable(ctx context.Context, o Opts, name string) error {
	_ = SudoRun(ctx, o, "systemctl", "stop", name+".service")
	return SudoRun(ctx, o, "systemctl", "disable", name+".service")
}

// ServiceStart starts a systemd service.
func ServiceStart(ctx context.Context, o Opts, name string) error {
	return SudoRun(ctx, o, "systemctl", "start", name+".service")
}

// SudoWriteFile writes content to a root-owned path via a temp file and sudo cp.
func SudoWriteFile(ctx context.Context, o Opts, content, path string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] write %s\n", path)
		return nil
	}
	tmp, err := os.CreateTemp("", "ctdev-write-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	return SudoRun(ctx, o, "cp", tmp.Name(), path)
}

// SafeSymlink creates a symlink at dst pointing to src.
// Removes any existing file or symlink at dst first.
func SafeSymlink(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	os.Remove(dst)
	return os.Symlink(src, dst)
}
