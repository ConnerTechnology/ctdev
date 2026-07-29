package state

import (
	"os"
	"path/filepath"
	"strings"
)

// appliedProfileFile records which profile this machine was built from, so
// `ctdev info` can name it outright instead of guessing from what's installed.
func appliedProfileFile() string {
	return filepath.Join(StateDir(), "applied-profile")
}

// RecordAppliedProfile notes the profile `ctdev apply` just finished applying.
func RecordAppliedProfile(name string) error {
	if err := os.MkdirAll(StateDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(appliedProfileFile(), []byte(name+"\n"), 0o644)
}

// AppliedProfile returns the recorded profile name, or "" on a machine that was
// composed by hand (or applied before this was recorded).
func AppliedProfile() string {
	b, err := os.ReadFile(appliedProfileFile())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
