package state

import (
	"os"
	"path/filepath"
)

func ConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "ctdev")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ctdev")
}

func StateDir() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "ctdev")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "ctdev")
}

func CacheDir() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "ctdev")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "ctdev")
}
