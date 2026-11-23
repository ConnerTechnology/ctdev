package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateDir(t *testing.T) {
	dir := StateDir()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".local", "state", "ctdev")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}

func TestStateDirXDGOverride(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/custom-state")
	got := StateDir()
	expected := filepath.Join("/tmp/custom-state", "ctdev")
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}
