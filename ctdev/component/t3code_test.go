package component

import (
	"errors"
	"testing"
)

func TestT3CodeDebAsset(t *testing.T) {
	for arch, want := range map[string][2]string{
		"amd64": {"T3-Code-0.1.7-amd64.deb", "latest-linux.yml"},
		"arm64": {"T3-Code-0.1.7-arm64.deb", "latest-linux-arm64.yml"},
	} {
		deb, manifest, err := t3codeDebAsset(arch, "0.1.7")
		if err != nil || deb != want[0] || manifest != want[1] {
			t.Errorf("%s: got %q %q %v, want %q %q", arch, deb, manifest, err, want[0], want[1])
		}
	}
	if _, _, err := t3codeDebAsset("riscv64", "0.1.7"); !errors.Is(err, ErrUnsupportedOS) {
		t.Errorf("riscv64: got %v, want ErrUnsupportedOS", err)
	}
}

func TestParseT3CodeVersion(t *testing.T) {
	for out, want := range map[string]string{"t3 v0.1.7\n": "0.1.7", "0.2.0": "0.2.0", "": ""} {
		if got := parseT3CodeVersion(out); got != want {
			t.Errorf("parseT3CodeVersion(%q) = %q, want %q", out, got, want)
		}
	}
}
