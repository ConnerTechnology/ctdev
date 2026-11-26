package component

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

func TestAtkinsonFontInstall_DryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var out bytes.Buffer
	if err := atkinsonFontInstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()

	var want []string
	if platform.Detect().PackageManager == "brew" {
		want = []string{"brew install --cask font-atkinson-hyperlegible-next font-atkinson-hyperlegible-mono"}
	} else {
		fontDir := filepath.Join(home, ".local", "share", "fonts")
		want = []string{
			"googlefonts/atkinson-hyperlegible-next/16d9515c933216b238289718ac7a3b3599b4d990/fonts/variable/AtkinsonHyperlegibleNext%5Bwght%5D.ttf",
			"googlefonts/atkinson-hyperlegible-next-mono/154d50362016cc3e873eb21d242cd0772384c8f9/fonts/variable/AtkinsonHyperlegibleMono-Italic%5Bwght%5D.ttf",
			"install to " + fontDir,
			"fc-cache -f " + fontDir,
		}
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("dry-run output missing %q; got:\n%s", w, got)
		}
	}
}

func TestAtkinsonFontInstall_SkipsWhenBothFamiliesPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fontDir := atkinsonFontDir(home, platform.Detect().OS)
	if err := os.MkdirAll(fontDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, f := range atkinsonFonts {
		if err := os.WriteFile(filepath.Join(fontDir, f.file), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := atkinsonFontInstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "already installed") {
		t.Errorf("expected a skip with all four fonts present; got:\n%s", out.String())
	}
}

func TestAtkinsonFontDetection(t *testing.T) {
	c := FindByName("atkinson-font")
	if c == nil {
		t.Fatal("atkinson-font component missing from registry")
	}
	for _, dir := range []string{filepath.Join("Library", "Fonts"), filepath.Join(".local", "share", "fonts")} {
		home := t.TempDir()
		t.Setenv("HOME", home)
		if c.IsInstalled() {
			t.Fatalf("detected as installed in an empty HOME")
		}
		fontDir := filepath.Join(home, dir)
		if err := os.MkdirAll(fontDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fontDir, "AtkinsonHyperlegibleMono[wght].ttf"), nil, 0644); err != nil {
			t.Fatal(err)
		}
		if !c.IsInstalled() {
			t.Errorf("not detected with the Mono font in ~/%s", dir)
		}
	}
}
