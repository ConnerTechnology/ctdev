package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// Paper (https://paper.design) ships its desktop app through ToDesktop. On
// macOS the paper-design cask is Apple Silicon only and updates itself. On
// Linux, ToDesktop serves a signed APT repository, and the package's postinst
// rewrites /etc/apt/sources.list.d/paper-desktop.sources from the vendor's
// copy, so ctdev writes the same file and apt sees one repository whichever
// wrote it last. The vendor's copy carries the same key inline, so once the
// package is in, ctdev's keyring goes unused; uninstall removes it anyway.
// apt upgrade keeps the app current, so it needs no update scanner.
const (
	paperCask        = "paper-design"
	paperPackage     = "paper"
	paperKeyURL      = "https://download.paper.design/apt/key.asc"
	paperKeyring     = "/usr/share/keyrings/paper-archive-keyring.gpg"
	paperSourceFile  = "paper-desktop.sources"
	paperRepoURL     = "https://download.paper.design/apt"
	paperDisplayName = "Paper"
)

// paperAPTSource is the deb822 entry for Paper's repository. It must be deb822,
// not a one-line "deb ..." entry: apt parses a file ending in .sources as
// deb822 and rejects anything else.
func paperAPTSource() string {
	return fmt.Sprintf(`Types: deb
URIs: %s
Suites: stable
Components: main
Architectures: amd64 arm64
Signed-By: %s`, paperRepoURL, paperKeyring)
}

func paperInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	switch p.OS {
	case platform.MacOS:
		if p.Arch != "arm64" {
			return fmt.Errorf("the Paper cask is Apple Silicon only: %w", ErrUnsupportedOS)
		}
	case platform.Linux:
		if p.PackageManager != "apt" {
			return unsupportedPMError(paperDisplayName, p.PackageManager)
		}
		if p.Arch != "amd64" && p.Arch != "arm64" {
			return fmt.Errorf("Paper only available for amd64/arm64 (got %s): %w", p.Arch, ErrUnsupportedOS)
		}
	default:
		return ErrUnsupportedOS
	}

	if !opts.Force && alreadyInstalled("paper") {
		fmt.Fprintln(opts.Stdout, "Paper already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing Paper...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskInstall(ctx, o, paperCask)
	}
	return sysutil.InstallAPTRepoPackage(ctx, o, sysutil.APTRepoPackage{
		KeyURL:      paperKeyURL,
		KeyringPath: paperKeyring,
		RepoLine:    paperAPTSource(),
		SourceFile:  paperSourceFile,
		Packages:    []string{paperPackage},
	})
}

func paperUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Paper...")

	switch p.OS {
	case platform.MacOS:
		return sysutil.BrewCaskRemove(ctx, o, paperCask)
	case platform.Linux:
		if p.PackageManager != "apt" {
			return unsupportedPMError(paperDisplayName, p.PackageManager)
		}
		// apt remove leaves the source the postinst wrote (only purge drops
		// it), so it goes here along with the keyring.
		if err := sysutil.RemovePackage(ctx, o, paperPackage); err != nil {
			return err
		}
		return sysutil.RemoveAPTRepo(ctx, o, paperSourceFile, paperKeyring)
	}
	return ErrUnsupportedOS
}
