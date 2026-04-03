package setup

import (
	"bytes"
	"strings"
	"testing"
)

func TestResetLinuxDefaultsDryRun(t *testing.T) {
	var buf bytes.Buffer
	err := ResetLinuxDefaults(&buf, true)
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
