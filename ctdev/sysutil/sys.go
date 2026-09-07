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
// The result is root-only (0600): cp carries the temp file's mode across. That
// suits drop-ins only root reads and secrets like restic.env — for a file
// every process must read, use SudoWriteFileMode.
func SudoWriteFile(ctx context.Context, o Opts, content, path string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] write %s\n", path)
		return nil
	}
	tmp, err := stageTempFile(content)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	return SudoRun(ctx, o, "cp", tmp, path)
}

// SudoWriteFileMode is SudoWriteFile with an explicit mode (e.g. "0644"),
// installed atomically-enough via install(1). Use it for anything read by
// unprivileged processes — /etc/resolv.conf unreadable by the user looks like
// "DNS works" because glibc silently falls back to loopback.
func SudoWriteFileMode(ctx context.Context, o Opts, content, path, mode string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] write %s (mode %s)\n", path, mode)
		return nil
	}
	tmp, err := stageTempFile(content)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	return SudoRun(ctx, o, "install", "-m", mode, tmp, path)
}

func stageTempFile(content string) (string, error) {
	tmp, err := os.CreateTemp("", "ctdev-write-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	tmp.Close()
	return tmp.Name(), nil
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

// HumanKB renders a size in 1K blocks as M/G/T with one decimal where useful —
// the same shape `df -h` prints, so ctdev's own sizes sit beside it cleanly.
func HumanKB(kb int64) string {
	// Below a gibibyte, report megabytes: a 372M /boot reads as "0.4G"
	// otherwise, which loses the precision exactly where it matters.
	if kb < 1024*1024 {
		return fmt.Sprintf("%.0fM", float64(kb)/1024)
	}
	gb := float64(kb) / (1024 * 1024)
	if gb >= 1024 {
		return fmt.Sprintf("%.1fT", gb/1024)
	}
	if gb >= 10 {
		return fmt.Sprintf("%.0fG", gb)
	}
	return fmt.Sprintf("%.1fG", gb)
}
