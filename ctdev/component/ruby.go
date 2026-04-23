package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

const rubyVersion = "3.4.1"

func rubyInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	rbenvDir := filepath.Join(home, ".rbenv")

	if !opts.Force && dirExists(rbenvDir) {
		fmt.Fprintln(opts.Stdout, "Ruby already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Ruby via rbenv...")

	switch p.PackageManager {
	case "brew":
		if err := sysutil.Run(ctx, o, "brew", "install", "rbenv", "ruby-build"); err != nil {
			return err
		}
		// Build deps
		for _, pkg := range []string{"openssl", "readline", "libyaml", "gmp"} {
			_ = sysutil.Run(ctx, o, "brew", "install", pkg)
		}

	case "apt":
		if err := ensureGitClone(ctx, o, "https://github.com/rbenv/rbenv.git", rbenvDir); err != nil {
			return fmt.Errorf("clone rbenv: %w", err)
		}
		pluginDir := filepath.Join(rbenvDir, "plugins", "ruby-build")
		if err := ensureGitClone(ctx, o, "https://github.com/rbenv/ruby-build.git", pluginDir); err != nil {
			return fmt.Errorf("clone ruby-build: %w", err)
		}
		fmt.Fprintln(opts.Stdout, "Installing Ruby build dependencies...")
		if err := sysutil.InstallPackage(ctx, o, "build-essential", "autoconf", "libssl-dev",
			"libyaml-dev", "zlib1g-dev", "libffi-dev", "libgmp-dev", "rustc"); err != nil {
			return fmt.Errorf("install build deps: %w", err)
		}

	case "dnf":
		if err := ensureGitClone(ctx, o, "https://github.com/rbenv/rbenv.git", rbenvDir); err != nil {
			return fmt.Errorf("clone rbenv: %w", err)
		}
		pluginDir := filepath.Join(rbenvDir, "plugins", "ruby-build")
		if err := ensureGitClone(ctx, o, "https://github.com/rbenv/ruby-build.git", pluginDir); err != nil {
			return fmt.Errorf("clone ruby-build: %w", err)
		}
		fmt.Fprintln(opts.Stdout, "Installing Ruby build dependencies...")
		if err := sysutil.SudoRun(ctx, o, "dnf", "groupinstall", "-y", "Development Tools"); err != nil {
			return fmt.Errorf("install development tools: %w", err)
		}
		if err := sysutil.SudoRun(ctx, o, "dnf", "install", "-y",
			"openssl-devel", "libyaml-devel", "zlib-devel", "libffi-devel", "gmp-devel", "rust"); err != nil {
			return fmt.Errorf("install build deps: %w", err)
		}

	default:
		return unsupportedPMError("ruby", p.PackageManager)
	}

	// Use rbenv from the install location
	rbenvBin := "rbenv"
	if p.PackageManager != "brew" {
		rbenvBin = filepath.Join(rbenvDir, "bin", "rbenv")
	}

	fmt.Fprintf(opts.Stdout, "Installing Ruby %s...\n", rubyVersion)
	if err := sysutil.Run(ctx, o, rbenvBin, "install", "--skip-existing", rubyVersion); err != nil {
		return fmt.Errorf("rbenv install %s: %w", rubyVersion, err)
	}

	if err := sysutil.Run(ctx, o, rbenvBin, "global", rubyVersion); err != nil {
		return fmt.Errorf("rbenv global %s: %w", rubyVersion, err)
	}

	// Install colorls gem
	fmt.Fprintln(opts.Stdout, "Installing colorls gem...")
	gemBin := "gem"
	if p.PackageManager != "brew" {
		gemBin = filepath.Join(rbenvDir, "shims", "gem")
	}
	if err := sysutil.Run(ctx, o, gemBin, "install", "colorls"); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: could not install colorls: %v\n", err)
	} else if !opts.DryRun {
		aliasLine := "alias lc='colorls -lA --sd'"
		exportsPath := sysutil.ExportsLocalPath()
		if added, err := sysutil.AppendLineIfMissing(exportsPath, aliasLine); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: could not add alias: %v\n", err)
		} else if added {
			fmt.Fprintln(opts.Stdout, "  Added alias lc to exports.local.zsh")
		}
	}

	return nil
}

func rubyUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Ruby (rbenv)...")

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	rbenvDir := filepath.Join(home, ".rbenv")
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] rm -rf %s\n", rbenvDir)
		return nil
	}

	return os.RemoveAll(rbenvDir)
}
