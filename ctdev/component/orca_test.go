package component

import (
	"errors"
	"testing"
)

func TestOrcaDebAsset(t *testing.T) {
	for arch, want := range map[string][2]string{
		"amd64": {"orca-ide_1.4.271_amd64.deb", "latest-linux.yml"},
		"arm64": {"orca-ide_1.4.271_arm64.deb", "latest-linux-arm64.yml"},
	} {
		deb, manifest, err := orcaDebAsset(arch, "1.4.271")
		if err != nil || deb != want[0] || manifest != want[1] {
			t.Errorf("%s: got %q %q %v, want %q %q", arch, deb, manifest, err, want[0], want[1])
		}
	}
	if _, _, err := orcaDebAsset("riscv64", "1.4.271"); !errors.Is(err, ErrUnsupportedOS) {
		t.Errorf("riscv64: got %v, want ErrUnsupportedOS", err)
	}
}
