package sysutil

import (
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
func ServiceEnable(o Opts, name string) error {
	return SudoRun(o, "systemctl", "enable", name+".service")
}

// ServiceDisable stops and disables a systemd service.
func ServiceDisable(o Opts, name string) error {
	_ = SudoRun(o, "systemctl", "stop", name+".service")
	return SudoRun(o, "systemctl", "disable", name+".service")
}

// ServiceStart starts a systemd service.
func ServiceStart(o Opts, name string) error {
	return SudoRun(o, "systemctl", "start", name+".service")
}

// SudoWriteFile writes content to a root-owned path via a temp file and sudo cp.
func SudoWriteFile(o Opts, content, path string) error {
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
	return SudoRun(o, "cp", tmp.Name(), path)
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
