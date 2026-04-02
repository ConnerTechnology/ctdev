package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDir(t *testing.T) {
	dir := ConfigDir()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config", "ctdev")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}

func TestStateDir(t *testing.T) {
	dir := StateDir()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".local", "state", "ctdev")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}

func TestCacheDir(t *testing.T) {
	dir := CacheDir()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".cache", "ctdev")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}
