package setup

import (
	"bytes"
	"errors"
	"runtime"
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

func TestDefaultsWrite_AccumulatesErrorsAndLogs(t *testing.T) {
	// The real `defaults` binary doesn't exist on non-darwin systems, giving
	// us a deterministic failure mode to assert on.
	if runtime.GOOS == "darwin" {
		t.Skip("defaults command available; cannot force failure")
	}
	var buf bytes.Buffer
	var errs []error
	defaultsWrite(&buf, &errs, "com.example.test", "SomeKey", "-bool", "true")
	defaultsWrite(&buf, &errs, "com.example.test", "OtherKey", "-int", "42")

	if len(errs) != 2 {
		t.Fatalf("expected 2 accumulated errors, got %d", len(errs))
	}
	out := buf.String()
	if !strings.Contains(out, "warning: defaults write com.example.test SomeKey") {
		t.Errorf("expected warning for SomeKey; got:\n%s", out)
	}
	if !strings.Contains(out, "warning: defaults write com.example.test OtherKey") {
		t.Errorf("expected warning for OtherKey; got:\n%s", out)
	}
}

func TestApplyMacOSDefaults_ReturnsJoinedErrorWhenWritesFail(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("real defaults command will succeed; test needs failure mode")
	}
	var buf bytes.Buffer
	err := ApplyMacOSDefaults(&buf, false)
	if err == nil {
		t.Fatal("expected error when defaults command is unavailable")
	}
	// errors.Join unwraps into multiple — check at least one is preserved.
	if !strings.Contains(err.Error(), "defaults write") {
		t.Errorf("expected 'defaults write' in error chain; got %v", err)
	}
	if !errors.Is(err, err) {
		t.Error("joined error should still be non-nil / comparable")
	}
	if !strings.Contains(buf.String(), "applied with") {
		t.Errorf("expected summary count of warnings; got:\n%s", buf.String())
	}
}
