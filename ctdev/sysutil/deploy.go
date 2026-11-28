package sysutil

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DeployFile writes a managed configuration file, saving changed originals.
func DeployFile(content []byte, dest string) error {
	_, err := DeployFileWithBackup(content, dest)
	return err
}

// DeployFileWithBackup also reports the saved original for interactive reviews.
// Keep regular files' permissions. Replace old symlinks (even dangling ones)
// at their path without writing through them. Stage the complete new file
// beside the destination so that the final rename leaves a usable live file
// even if staging, backing up, or activation fails.
func DeployFileWithBackup(content []byte, dest string) (string, error) {
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create parent dirs for %s: %w", dest, err)
	}
	unlock, err := lockDeployDir(dir, true)
	if err != nil {
		return "", err
	}
	defer unlock()

	fi, err := os.Lstat(dest)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	var old []byte
	regular := fi != nil && fi.Mode().IsRegular()
	if regular {
		old, err = os.ReadFile(dest)
		if err != nil || bytes.Equal(old, content) {
			return "", err
		}
	} else if fi != nil && fi.Mode()&os.ModeSymlink == 0 {
		return "", fmt.Errorf("cannot replace non-regular file %s", dest)
	}

	stage, err := os.MkdirTemp(dir, ".ctdev-deploy-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	next := filepath.Join(stage, "new")
	if err := os.WriteFile(next, content, 0o644); err != nil {
		return "", err
	}
	var backup string
	if regular {
		if err := os.Chmod(next, fi.Mode().Perm()); err != nil {
			return "", err
		}
		previous := filepath.Join(stage, "previous")
		if err := os.WriteFile(previous, old, 0600); err != nil {
			return "", err
		}
		if err := os.Chmod(previous, fi.Mode().Perm()); err != nil {
			return "", err
		}
		backup, err = moveToBackup(previous, dest)
		if err != nil {
			return "", err
		}
	}
	if err := os.Rename(next, dest); err != nil {
		return backup, fmt.Errorf("activate %s: %w", dest, err)
	}
	return backup, nil
}

// BackupPath is the first dated backup name for dest. Additional backups in
// the same second have a numbered suffix so a restore can order them.
func BackupPath(dest string) string {
	return fmt.Sprintf("%s.%s.bak", dest, time.Now().Format("2006-01-02T15-04-05"))
}

// moveToBackup publishes a complete file or symlink; a restore may read a
// backup as soon as its name appears. Both callers hold the directory lock.
func moveToBackup(source, dest string) (string, error) {
	base := strings.TrimSuffix(BackupPath(dest), ".bak")
	name := base + ".bak"
	for i := 1; ; i++ {
		switch err := os.Link(source, name); {
		case err == nil:
			return name, os.Remove(source)
		case !errors.Is(err, os.ErrExist):
			// FAT and some network homes do not support hard links. The
			// directory lock also serializes the existence check and rename
			// between ctdev processes on filesystems without link support.
			if _, err := os.Lstat(name); errors.Is(err, os.ErrNotExist) {
				if err := os.Rename(source, name); err != nil {
					return "", err
				}
				return name, nil
			} else if err != nil {
				return "", err
			}
		}
		name = fmt.Sprintf("%s_%09d.bak", base, i)
	}
}

// BackupFile moves an unwanted file (including a symlink) out of the way,
// returning its unique backup name.
func BackupFile(path string) (string, error) {
	unlock, err := lockDeployDir(filepath.Dir(path), true)
	if err != nil {
		return "", err
	}
	defer unlock()
	return moveToBackup(path, path)
}

// DeployFileFromFS reads a file from an embedded FS and deploys it to dest with
// DeployFile.
func DeployFileFromFS(fs embed.FS, srcPath, dest string) error {
	content, err := fs.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read embedded %s: %w", srcPath, err)
	}
	return DeployFile(content, dest)
}
