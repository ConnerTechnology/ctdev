package component

import (
	"context"
	"fmt"
	"os"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// TradingView's desktop app: a Homebrew cask on macOS, a .deb on apt. The
// site also offers a snap, but Linux Mint pins snapd out of APT on purpose,
// so the .deb is the path that works on every apt host without touching
// distro policy. The package installs to /opt/TradingView and symlinks
// /usr/bin/tradingview, which is what DetectCmd finds.
const (
	tradingviewCask   = "tradingview"
	tradingviewApp    = "/Applications/TradingView.app"
	tradingviewPkg    = "tradingview"
	tradingviewDebURL = "https://tvd-packages.tradingview.com/ubuntu/stable/latest/jammy/tradingview_amd64.deb"
)

func tradingviewInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && alreadyInstalled("tradingview") {
		fmt.Fprintln(opts.Stdout, "TradingView already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing TradingView...")

	switch p.PackageManager {
	case "brew":
		if err := sysutil.BrewCaskInstall(ctx, o, tradingviewCask); err != nil {
			return err
		}
		// The Electron app hangs on Gatekeeper's first-launch assessment while
		// quarantined. The attribute may already be gone, so a failure here is
		// not an install failure.
		_ = sysutil.Run(ctx, o, "xattr", "-dr", "com.apple.quarantine", tradingviewApp)
		return nil
	case "apt":
		if p.Arch != "amd64" {
			return fmt.Errorf("tradingview .deb only available on amd64 (got %s)", p.Arch)
		}
		if o.DryRun {
			fmt.Fprintln(o.Stdout, "[dry-run] download and install tradingview .deb")
			return nil
		}
		tmp, err := os.CreateTemp("", "tradingview-*.deb")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		tmp.Close()

		if err := sysutil.DownloadFile(ctx, tradingviewDebURL, tmp.Name()); err != nil {
			return fmt.Errorf("download tradingview: %w", err)
		}
		return installDebWithDepFix(ctx, o, tmp.Name(), tradingviewPkg)
	default:
		return unsupportedPMError("tradingview", p.PackageManager)
	}
}

func tradingviewUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing TradingView...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskRemove(ctx, o, tradingviewCask)
	case "apt":
		return sysutil.RemovePackage(ctx, o, tradingviewPkg)
	default:
		return ErrUnsupportedOS
	}
}
