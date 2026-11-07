package component

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

func TestKopiaIsRegistered(t *testing.T) {
	c := FindByName("kopia")
	if c == nil {
		t.Fatal("kopia should be in the registry")
	}
	if !c.SupportsOS(OSLinux) || c.SupportsOS(OSMacOS) {
		t.Error("kopia should support Linux only")
	}
	if c.Category != CategoryInfra {
		t.Errorf("kopia should be under %s, got %s", CategoryInfra, c.Category)
	}
}

// Both architectures a node can be get pinned hashes, and nothing else gets a
// download at all.
func TestKopiaPins(t *testing.T) {
	sha := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for arch, want := range map[string]string{
		"amd64": "kopia-0.23.1-linux-x64",
		"arm64": "kopia-0.23.1-linux-arm64",
	} {
		pin, err := kopiaPinFor(arch)
		if err != nil || pin.asset != want {
			t.Errorf("kopiaPinFor(%q) = %q, %v; want %q", arch, pin.asset, err, want)
		}
		if !sha.MatchString(pin.tarSHA256) || !sha.MatchString(pin.binSHA256) {
			t.Errorf("kopiaPinFor(%q) hashes aren't 64 lowercase hex characters: %+v", arch, pin)
		}
	}
	if _, err := kopiaPinFor("arm"); !errors.Is(err, ErrUnsupportedOS) {
		t.Errorf("an arch with no pin should be unsupported, got %v", err)
	}
}

// fakeKopiaBin points kopiaBin at a temp file holding content, or at a
// missing file when content is nil.
func fakeKopiaBin(t *testing.T, content []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kopia")
	if content != nil {
		if err := os.WriteFile(path, content, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	orig := kopiaBin
	kopiaBin = path
	t.Cleanup(func() { kopiaBin = orig })
}

// Detect hashes the file and never runs it, so a binary that only claims the
// version (or a script that prints it) doesn't count.
func TestKopiaDetectHashesTheFile(t *testing.T) {
	if _, err := kopiaPinFor(platform.Detect().Arch); err != nil {
		t.Skip("no pinned kopia build for this arch")
	}
	fakeKopiaBin(t, []byte("#!/bin/sh\necho '0.23.1 build: 72ec08fd from: kopia/kopia'\n"))
	if kopiaPinnedInstalled() {
		t.Error("a file that doesn't hash to the pin counted as installed")
	}
	fakeKopiaBin(t, nil)
	if kopiaPinnedInstalled() {
		t.Error("a missing binary counted as installed")
	}
}

// A node's boot re-runs the install on every start, so anything that isn't
// the pinned binary, or no binary at all, gets the pinned download.
func TestKopiaInstallReplacesAnythingElse(t *testing.T) {
	arch := platform.Detect().Arch
	pin, err := kopiaPinFor(arch)
	if err != nil {
		t.Skipf("no pinned kopia build for %s", arch)
	}
	for name, content := range map[string][]byte{
		"other": []byte("not the pinned kopia"),
		"none":  nil,
	} {
		t.Run(name, func(t *testing.T) {
			fakeKopiaBin(t, content)
			var out bytes.Buffer
			if err := kopiaInstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
				t.Fatal(err)
			}
			got := out.String()
			if strings.Contains(got, "already installed") {
				t.Errorf("expected an install, got:\n%s", got)
			}
			if !strings.Contains(got, "https://github.com/kopia/kopia/releases/download/v0.23.1/"+pin.asset+".tar.gz") ||
				!strings.Contains(got, "verify sha256 "+pin.tarSHA256) {
				t.Errorf("expected the pinned download and its hash, got:\n%s", got)
			}
		})
	}
}
