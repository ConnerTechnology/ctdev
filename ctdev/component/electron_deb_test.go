package component

import (
	"crypto/sha512"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The shape of a real electron-builder latest-linux.yml; the hashes are fake.
const electronTestManifest = `version: 0.1.7
files:
  - url: T3-Code-0.1.7-x86_64.AppImage
    sha512: APPIMAGESUM
    size: 190513758
    blockMapSize: 198754
  - url: T3-Code-0.1.7-amd64.deb
    sha512: DEBSUM
    size: 159667676
path: T3-Code-0.1.7-x86_64.AppImage
sha512: APPIMAGESUM
`

func TestParseElectronManifestSHA512_PicksTheNamedFile(t *testing.T) {
	got, err := parseElectronManifestSHA512(strings.NewReader(electronTestManifest), "T3-Code-0.1.7-amd64.deb")
	if err != nil {
		t.Fatal(err)
	}
	if got != "DEBSUM" {
		t.Errorf("got %q, want the .deb's hash, not the AppImage's", got)
	}
}

func TestParseElectronManifestSHA512_FailsClosed(t *testing.T) {
	cases := map[string]struct{ body, file string }{
		"file not listed":     {electronTestManifest, "T3-Code-0.1.6-amd64.deb"},
		"entry has no hash":   {"files:\n  - url: a.deb\n    size: 1\n  - url: b.deb\n    sha512: BSUM\n", "a.deb"},
		"empty hash":          {"files:\n  - url: a.deb\n    sha512:\n", "a.deb"},
		"top-level path only": {"path: a.deb\nsha512: TOPSUM\n", "a.deb"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got, err := parseElectronManifestSHA512(strings.NewReader(c.body), c.file); err == nil {
				t.Errorf("got %q, want an error", got)
			}
		})
	}
}

func TestVerifySHA512Base64(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.deb")
	if err := os.WriteFile(path, []byte("example deb"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha512.Sum512([]byte("example deb"))
	if err := verifySHA512Base64(path, base64.StdEncoding.EncodeToString(sum[:])); err != nil {
		t.Errorf("matching hash: %v", err)
	}
	if err := verifySHA512Base64(path, "WRONG"); err == nil {
		t.Error("mismatched hash passed")
	}
}
