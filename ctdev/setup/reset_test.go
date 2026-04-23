package setup

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func TestResetLinuxDefaultsDryRun(t *testing.T) {
	var buf bytes.Buffer
	err := ResetLinuxDefaults(context.Background(), sysutil.Opts{Stdout: &buf, DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "DRY-RUN") {
		t.Error("expected DRY-RUN in output")
	}
	// Should mention the major categories being reset
	for _, keyword := range []string{"Power", "Screensaver", "Keyboard", "GRUB"} {
		if !strings.Contains(output, keyword) {
			t.Errorf("expected %q in dry-run output", keyword)
		}
	}
}
