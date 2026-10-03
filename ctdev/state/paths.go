package state

import (
	"os"
	"path/filepath"
)

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	if home == "" {
		panic("cannot determine home directory")
	}
	return home
}

func ConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "ctdev")
	}
	return filepath.Join(homeDir(), ".config", "ctdev")
}

func StateDir() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "ctdev")
	}
	return filepath.Join(homeDir(), ".local", "state", "ctdev")
}

func CacheDir() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "ctdev")
	}
	return filepath.Join(homeDir(), ".cache", "ctdev")
}
