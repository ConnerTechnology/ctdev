package sysutil

import (
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

// SafeSymlink creates a symlink at dst pointing to src.
// Removes any existing file or symlink at dst first.
func SafeSymlink(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	os.Remove(dst)
	return os.Symlink(src, dst)
}
