package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// rustupInstallScript runs the official rustup installer unattended. It puts
// rustup, cargo and the stable toolchain under ~/.cargo and ~/.rustup on both
// macOS and Linux. --no-modify-path because configs/zsh/path.zsh adds
// ~/.cargo/bin itself.
const rustupInstallScript = "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --no-modify-path"

func rustupPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cargo", "bin", "rustup"), nil
}

func rustInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	rustup, err := rustupPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(rustup); err == nil && !opts.Force {
		fmt.Fprintln(opts.Stdout, "Rust already installed")
		return nil
	}

	// cargo links through the system C compiler. Homebrew already requires the
	// Xcode Command Line Tools, which provide it; a bare Debian/Ubuntu does not.
	if p.PackageManager == "apt" && !sysutil.CommandExists("cc") {
		fmt.Fprintln(opts.Stdout, "Installing a C linker for cargo (build-essential)...")
		if err := sysutil.InstallPackage(ctx, o, "build-essential"); err != nil {
			return fmt.Errorf("install build-essential: %w", err)
		}
	}

	fmt.Fprintln(opts.Stdout, "Installing Rust via rustup...")
	if err := sysutil.Run(ctx, o, "sh", "-c", rustupInstallScript); err != nil {
		return err
	}
	// rustup's closing message says PATH was left alone, which is true of
	// rustup but not of ctdev.
	fmt.Fprintln(opts.Stdout, "Rust installed to ~/.cargo (PATH wired via path.zsh: run `ctdev install zsh` if cargo is not found in a new shell)")
	return nil
}

func rustUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Rust (rustup)...")

	rustup, err := rustupPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(rustup); err != nil {
		return nil
	}
	// Removes ~/.cargo and ~/.rustup, every toolchain and cargo-installed binary
	// with them. build-essential stays: other things compile against it.
	return sysutil.Run(ctx, o, rustup, "self", "uninstall", "-y")
}
