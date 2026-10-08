package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// Anthropic's apt repository for the Linux desktop app (beta, Debian 12+ and
// Ubuntu 22.04+, amd64 and arm64): https://code.claude.com/docs/en/desktop-linux.
// The package registers its own key and source at the same list path, so apt
// sees one repository whichever wrote it last.
const (
	claudeDesktopKeyring    = "/usr/share/keyrings/claude-desktop-archive-keyring.gpg"
	claudeDesktopSourceFile = "claude-desktop.list"
)

func claudeDesktopInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	switch p.OS {
	case platform.MacOS:
	case platform.Linux:
		if p.PackageManager != "apt" {
			return unsupportedPMError("Claude Desktop", p.PackageManager)
		}
		if p.Arch != "amd64" && p.Arch != "arm64" {
			return fmt.Errorf("Claude Desktop only available for amd64/arm64 (got %s): %w", p.Arch, ErrUnsupportedOS)
		}
	default:
		return ErrUnsupportedOS
	}

	if !opts.Force && alreadyInstalled("claude-desktop") {
		fmt.Fprintln(opts.Stdout, "Claude Desktop already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing Claude Desktop...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskInstall(ctx, o, "claude")
	}
	return sysutil.InstallAPTRepoPackage(ctx, o, sysutil.APTRepoPackage{
		KeyURL:      "https://downloads.claude.ai/claude-desktop/key.asc",
		KeyringPath: claudeDesktopKeyring,
		RepoLine:    fmt.Sprintf("deb [arch=amd64,arm64 signed-by=%s] https://downloads.claude.ai/claude-desktop/apt/stable stable main", claudeDesktopKeyring),
		SourceFile:  claudeDesktopSourceFile,
		Packages:    []string{"claude-desktop"},
	})
}

func claudeDesktopUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Claude Desktop...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskRemove(ctx, o, "claude")
	}
	if p.PackageManager == "apt" {
		if err := sysutil.RemovePackage(ctx, o, "claude-desktop"); err != nil {
			return err
		}
		return sysutil.RemoveAPTRepo(ctx, o, claudeDesktopSourceFile, claudeDesktopKeyring)
	}
	return ErrUnsupportedOS
}
