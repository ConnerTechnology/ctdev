package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

var nerdFonts = []string{"FiraCode", "JetBrainsMono"}

func fontsInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	if !opts.Force {
		var fontDir string
		if p.OS == platform.MacOS {
			fontDir = filepath.Join(home, "Library", "Fonts")
		} else {
			fontDir = filepath.Join(home, ".local", "share", "fonts")
		}
		matches, _ := filepath.Glob(filepath.Join(fontDir, "*Nerd*"))
		if len(matches) > 0 {
			fmt.Fprintln(opts.Stdout, "Fonts already installed")
			return nil
		}
	}

	fmt.Fprintln(opts.Stdout, "Installing Nerd Fonts...")

	if p.PackageManager == "brew" {
		casks := []string{
			"font-fira-code-nerd-font",
			"font-jetbrains-mono-nerd-font",
		}
		for _, cask := range casks {
			if err := sysutil.Run(o, "brew", "install", "--cask", cask); err != nil {
				return fmt.Errorf("install %s: %w", cask, err)
			}
		}
		return nil
	}

	// Linux: download from GitHub releases
	fontDir := filepath.Join(home, ".local", "share", "fonts")
	if !o.DryRun {
		if err := os.MkdirAll(fontDir, 0755); err != nil {
			return err
		}
	}

	for _, font := range nerdFonts {
		url := fmt.Sprintf("https://github.com/ryanoasis/nerd-fonts/releases/latest/download/%s.zip", font)
		zipPath := filepath.Join(os.TempDir(), font+".zip")

		if o.DryRun {
			fmt.Fprintf(o.Stdout, "[dry-run] download %s and extract to %s\n", url, fontDir)
			continue
		}

		fmt.Fprintf(opts.Stdout, "Downloading %s...\n", font)
		if err := sysutil.DownloadFile(url, zipPath); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: failed to download %s: %v\n", font, err)
			continue
		}

		err := sysutil.Run(o, "unzip", "-oq", zipPath, "-d", fontDir)
		os.Remove(zipPath)
		if err != nil {
			fmt.Fprintf(opts.Stdout, "warning: failed to extract %s: %v\n", font, err)
		}
	}

	if !o.DryRun {
		_ = sysutil.Run(o, "fc-cache", "-f", fontDir)
	}

	return nil
}

func fontsUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Nerd Fonts...")

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	if p.PackageManager == "brew" {
		_ = sysutil.Run(o, "brew", "uninstall", "--cask", "font-fira-code-nerd-font")
		_ = sysutil.Run(o, "brew", "uninstall", "--cask", "font-jetbrains-mono-nerd-font")
		return nil
	}

	fontDir := filepath.Join(home, ".local", "share", "fonts")
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] rm -rf %s/*Nerd*\n", fontDir)
		return nil
	}

	matches, _ := filepath.Glob(filepath.Join(fontDir, "*Nerd*"))
	for _, m := range matches {
		os.RemoveAll(m)
	}
	return nil
}
