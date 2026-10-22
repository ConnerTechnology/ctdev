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
	t.Setenv("HOMEBREW_SUDO_CHECKED", "")
	os.Unsetenv("HOMEBREW_SUDO_CHECKED")

	setupHomebrewEnv()

	if got := os.Getenv("HOMEBREW_NO_AUTO_UPDATE"); got != "0" {
		t.Errorf("HOMEBREW_NO_AUTO_UPDATE = %q, want the user's explicit 0 preserved", got)
	}
	if got := os.Getenv("HOMEBREW_NO_ENV_HINTS"); got != "1" {
		t.Errorf("HOMEBREW_NO_ENV_HINTS = %q, want 1 when unset", got)
	}
	if got := os.Getenv("HOMEBREW_SUDO_CHECKED"); got != "1" {
		t.Errorf("HOMEBREW_SUDO_CHECKED = %q, want 1 so brew keeps the cached sudo credential", got)
	}
}

// releaseProcessState undoes what ctdev set for itself and only that: a
// variable the user had set survives it.
func TestReleaseProcessStateKeepsTheUsersEnv(t *testing.T) {
	t.Cleanup(func() { askpassPath, ctdevEnv = "", nil })
	t.Setenv("HOMEBREW_NO_AUTO_UPDATE", "user-value")
	t.Setenv("SUDO_ASKPASS", "")
	os.Unsetenv("SUDO_ASKPASS")
	for _, k := range []string{"HOMEBREW_NO_ENV_HINTS", "HOMEBREW_SUDO_CHECKED"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	cleanup := setupAskpass()
	defer cleanup()
	setupHomebrewEnv()
	script := os.Getenv("SUDO_ASKPASS")
	if script == "" || os.Getenv("HOMEBREW_NO_ENV_HINTS") != "1" {
		t.Fatal("setup didn't set its variables")
	}

	releaseProcessState()
	for _, k := range []string{"SUDO_ASKPASS", "HOMEBREW_NO_ENV_HINTS", "HOMEBREW_SUDO_CHECKED"} {
		if _, ok := os.LookupEnv(k); ok {
			t.Errorf("%s is still set", k)
		}
	}
	if got := os.Getenv("HOMEBREW_NO_AUTO_UPDATE"); got != "user-value" {
		t.Errorf("the user's HOMEBREW_NO_AUTO_UPDATE became %q", got)
	}
	if _, err := os.Stat(script); !os.IsNotExist(err) {
		t.Error("the askpass script is still there")
	}
}
