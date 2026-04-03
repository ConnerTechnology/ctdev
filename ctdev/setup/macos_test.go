package setup

import (
	"bytes"
	"strings"
	"testing"
)

func TestApplyMacOSDefaultsDryRun(t *testing.T) {
	var buf bytes.Buffer
	err := ApplyMacOSDefaults(&buf, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "[dry-run]") {
		t.Error("expected dry-run output")
	}
	if !strings.Contains(output, "Dock") {
		t.Error("expected Dock mention in dry-run output")
	}
	if !strings.Contains(output, "Finder") {
		t.Error("expected Finder mention in dry-run output")
	}
}
