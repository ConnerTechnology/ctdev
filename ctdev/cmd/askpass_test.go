package cmd

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSetupAskpassWritesRunnableHelper(t *testing.T) {
	t.Setenv("SUDO_ASKPASS", "")
	os.Unsetenv("SUDO_ASKPASS")

	cleanup := setupAskpass()
	path := os.Getenv("SUDO_ASKPASS")
	if path == "" {
		t.Fatal("SUDO_ASKPASS was not set")
	}

	// sudo execs this directly, so it has to be executable and self-contained.
	out, err := exec.Command(path).CombinedOutput()
	if err == nil {
		t.Error("askpass helper exited 0; sudo would take that as a supplied password")
	}
	if !strings.Contains(string(out), "sudo password is needed") {
		t.Errorf("helper output does not explain itself:\n%s", out)
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cleanup left the helper behind at %s", path)
	}
}

// A user who has their own askpass program keeps it.
func TestSetupAskpassRespectsExistingSetting(t *testing.T) {
	t.Setenv("SUDO_ASKPASS", "/usr/bin/my-askpass")

	cleanup := setupAskpass()
	defer cleanup()

	if got := os.Getenv("SUDO_ASKPASS"); got != "/usr/bin/my-askpass" {
		t.Errorf("SUDO_ASKPASS = %q, want the user's own setting preserved", got)
	}
}

func TestSetupHomebrewEnvDoesNotOverrideUser(t *testing.T) {
	t.Setenv("HOMEBREW_NO_AUTO_UPDATE", "0")
	t.Setenv("HOMEBREW_NO_ENV_HINTS", "")
	os.Unsetenv("HOMEBREW_NO_ENV_HINTS")

	setupHomebrewEnv()

	if got := os.Getenv("HOMEBREW_NO_AUTO_UPDATE"); got != "0" {
		t.Errorf("HOMEBREW_NO_AUTO_UPDATE = %q, want the user's explicit 0 preserved", got)
	}
	if got := os.Getenv("HOMEBREW_NO_ENV_HINTS"); got != "1" {
		t.Errorf("HOMEBREW_NO_ENV_HINTS = %q, want 1 when unset", got)
	}
}
