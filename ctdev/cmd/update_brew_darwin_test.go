package cmd

import (
	"os"
	"os/exec"
	"testing"
)

// The hosted application suite exercises Homebrew's actual JSON format.
// Other update sources have separate command tests with controlled inputs.
func TestNativeHomebrewScan(t *testing.T) {
	if os.Getenv("CTDEV_TEST_NATIVE_BREW") != "1" {
		t.Skip("enabled on the macOS Homebrew runner")
	}
	if _, err := exec.LookPath("brew"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOMEBREW_NO_AUTO_UPDATE", "1")
	if _, err := scanBrew(t.Context()); err != nil {
		t.Fatal(err)
	}
}
