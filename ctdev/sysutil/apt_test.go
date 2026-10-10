package sysutil

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestAddAPTKeyringDryRun(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}
	err := AddAPTKeyring(context.Background(), o, "https://example.com/key.gpg", "/etc/apt/keyrings/example.gpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "[dry-run]") {
		t.Error("expected dry-run prefix in output")
	}
	if !strings.Contains(buf.String(), "example.com") {
		t.Error("expected URL in dry-run output")
	}
}

func TestAddAPTSourceDryRun(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}
	err := AddAPTSource(context.Background(), o, "deb [arch=amd64] https://example.com stable main", "example.list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "[dry-run]") {
		t.Error("expected dry-run prefix")
	}
}

func TestAPTSourceInstallArgsWorldReadable(t *testing.T) {
	got := strings.Join(aptSourceInstallArgs("/tmp/apt-source-1", "/etc/apt/sources.list.d/example.list"), " ")
	want := "-m 0644 /tmp/apt-source-1 /etc/apt/sources.list.d/example.list"
	if got != want {
		t.Errorf("install args = %q, want %q", got, want)
	}
}
