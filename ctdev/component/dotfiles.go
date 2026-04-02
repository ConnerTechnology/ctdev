package component

import (
	"os"
	"path/filepath"
)

// findDotfilesRoot attempts to locate the dotfiles repository root.
// It checks DOTFILES_ROOT env, then the executable's parent directory,
// then falls back to the conventional path under $HOME.
func findDotfilesRoot() string {
	if root := os.Getenv("DOTFILES_ROOT"); root != "" {
		return root
	}

	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Dir(filepath.Dir(exe))
		if _, err := os.Stat(filepath.Join(candidate, "lib", "utils.sh")); err == nil {
			return candidate
		}
	}

	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Repos", "github.com", "ConnerTechnology", "dotfiles")
}
