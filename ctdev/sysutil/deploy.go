package sysutil

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DeployFile writes content to dest, backing up any differing existing file.
// If dest already has identical content, it's a no-op.
// Backup format: <filename>.<YYYY-MM-DDTHH-MM-SS>.bak
func DeployFile(content []byte, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("create parent dirs for %s: %w", dest, err)
	}

	existing, err := os.ReadFile(dest)
	if err == nil {
		if bytes.Equal(existing, content) {
			return nil
		}
		stamp := time.Now().Format("2006-01-02T15-04-05")
		backup := fmt.Sprintf("%s.%s.bak", dest, stamp)
		if err := os.Rename(dest, backup); err != nil {
			return fmt.Errorf("backup %s: %w", dest, err)
		}
	}

	return os.WriteFile(dest, content, 0644)
}

// DeployFileFromFS reads a file from an embedded FS and deploys it to dest.
func DeployFileFromFS(fs embed.FS, srcPath, dest string) error {
	content, err := fs.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read embedded %s: %w", srcPath, err)
	}
	return DeployFile(content, dest)
}
