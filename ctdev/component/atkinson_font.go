package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// atkinsonFont is one variable TTF of the Atkinson Hyperlegible families. The
// googlefonts repos publish no releases, so Linux downloads each file from a
// pinned commit and checks it against the recorded SHA-256.
type atkinsonFont struct {
	file   string
	repo   string
	commit string
	sha256 string
}

var atkinsonFonts = []atkinsonFont{
	{"AtkinsonHyperlegibleNext[wght].ttf", "atkinson-hyperlegible-next", "16d9515c933216b238289718ac7a3b3599b4d990", "5a455d1cfa099b601ab70751bb9673e8fe1854dc4500c80e1a220d0d75e31745"},
	{"AtkinsonHyperlegibleNext-Italic[wght].ttf", "atkinson-hyperlegible-next", "16d9515c933216b238289718ac7a3b3599b4d990", "ce9cffed32742ad2d9238c561a93220385e5934cdc02b8eb4097a50efa957dc6"},
	{"AtkinsonHyperlegibleMono[wght].ttf", "atkinson-hyperlegible-next-mono", "154d50362016cc3e873eb21d242cd0772384c8f9", "5ce8b1698d1ded7dff2178c1a3ad159470085a58ea239e8b2cb88f4fb4a6f646"},
	{"AtkinsonHyperlegibleMono-Italic[wght].ttf", "atkinson-hyperlegible-next-mono", "154d50362016cc3e873eb21d242cd0772384c8f9", "e18f523877530e4abe229df6a36d394326254da488a7f5abb9b7b5ea8f780cd4"},
}

// The casks install the same four files into ~/Library/Fonts.
var atkinsonCasks = []string{"font-atkinson-hyperlegible-next", "font-atkinson-hyperlegible-mono"}

// url escapes the brackets in the file name, which a URL reserves for IPv6 hosts.
func (f atkinsonFont) url() string {
	name := strings.NewReplacer("[", "%5B", "]", "%5D").Replace(f.file)
	return fmt.Sprintf("https://raw.githubusercontent.com/googlefonts/%s/%s/fonts/variable/%s", f.repo, f.commit, name)
}

func atkinsonFontDir(home string, goos platform.OS) string {
	if goos == platform.MacOS {
		return filepath.Join(home, "Library", "Fonts")
	}
	return filepath.Join(home, ".local", "share", "fonts")
}

func atkinsonFontsPresent(fontDir string) bool {
	for _, f := range atkinsonFonts {
		if _, err := os.Stat(filepath.Join(fontDir, f.file)); err != nil {
			return false
		}
	}
	return true
}

func atkinsonFontInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	fontDir := atkinsonFontDir(home, p.OS)

	if !opts.Force && atkinsonFontsPresent(fontDir) {
		fmt.Fprintln(opts.Stdout, "Atkinson Hyperlegible fonts already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Atkinson Hyperlegible Next and Mono fonts...")

	if p.PackageManager == "brew" {
		args := append([]string{"install", "--cask"}, atkinsonCasks...)
		return sysutil.Run(ctx, o, "brew", args...)
	}

	if o.DryRun {
		for _, f := range atkinsonFonts {
			fmt.Fprintf(o.Stdout, "[dry-run] download %s, verify sha256, install to %s\n", f.url(), fontDir)
		}
		fmt.Fprintf(o.Stdout, "[dry-run] fc-cache -f %s\n", fontDir)
		return nil
	}

	if err := os.MkdirAll(fontDir, 0755); err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp("", "ctdev-atkinson-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// A failed font is a warning, as in fontsInstall: the others still install.
	for _, f := range atkinsonFonts {
		tmp := filepath.Join(tmpDir, f.file)
		fmt.Fprintf(opts.Stdout, "Downloading %s...\n", f.file)
		if err := sysutil.DownloadFile(ctx, f.url(), tmp); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: failed to download %s: %v\n", f.file, err)
			continue
		}
		if err := sysutil.VerifyChecksum(tmp, f.sha256); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: %s failed verification: %v\n", f.file, err)
			continue
		}
		if err := os.Rename(tmp, filepath.Join(fontDir, f.file)); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: failed to install %s: %v\n", f.file, err)
		}
	}

	_ = sysutil.Run(ctx, o, "fc-cache", "-f", fontDir)
	return nil
}

func atkinsonFontUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Atkinson Hyperlegible fonts...")

	if p.PackageManager == "brew" {
		args := append([]string{"uninstall", "--cask"}, atkinsonCasks...)
		return sysutil.Run(ctx, o, "brew", args...)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	fontDir := atkinsonFontDir(home, p.OS)
	for _, f := range atkinsonFonts {
		path := filepath.Join(fontDir, f.file)
		if o.DryRun {
			fmt.Fprintf(o.Stdout, "[dry-run] rm %s\n", path)
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if !o.DryRun {
		_ = sysutil.Run(ctx, o, "fc-cache", "-f", fontDir)
	}
	return nil
}
