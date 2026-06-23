package component

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// NoMachine ships no apt repo, and its "latest" download alias resolves to the
// marketing homepage rather than a package, so the version is pinned here and
// bumped manually on upgrade. See https://www.nomachine.com/download (Linux DEB
// amd64). The download path embeds the major.minor branch derived from it.
const (
	nomachineVersion = "9.7.3"
	nomachineBuild   = "1"
)

// nomachinePort is the NX service port the server listens on. The installer
// binds it on all interfaces, so we scope reachability with a ufw rule on the
// Tailscale interface only — remote desktop works across the tailnet but is
// never exposed to the LAN or the internet.
const nomachinePort = "4000"

func nomachineInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if p.PackageManager != "apt" {
		return unsupportedPMError("nomachine", p.PackageManager)
	}
	if p.Arch != "amd64" {
		return fmt.Errorf("nomachine .deb only available on amd64 (got %s)", p.Arch)
	}

	// Phase 1: install the .deb (skip if already present unless --force).
	if opts.Force || !alreadyInstalled("nomachine") {
		fmt.Fprintln(opts.Stdout, "Installing NoMachine...")

		branch := nomachineVersion[:strings.LastIndex(nomachineVersion, ".")]
		url := fmt.Sprintf("https://download.nomachine.com/download/%s/Linux/nomachine_%s_%s_amd64.deb", branch, nomachineVersion, nomachineBuild)

		if o.DryRun {
			fmt.Fprintf(o.Stdout, "[dry-run] download and install %s\n", url)
		} else {
			tmp, err := os.CreateTemp("", "nomachine-*.deb")
			if err != nil {
				return err
			}
			defer os.Remove(tmp.Name())
			tmp.Close()

			if err := sysutil.DownloadFile(ctx, url, tmp.Name()); err != nil {
				return fmt.Errorf("download nomachine: %w", err)
			}
			if err := installDebWithDepFix(ctx, o, tmp.Name(), "nomachine"); err != nil {
				return err
			}
		}
	} else {
		fmt.Fprintln(opts.Stdout, "NoMachine already installed")
	}

	// Phase 2: scope the NX port to the Tailscale interface. ufw rules are
	// idempotent, so this is safe to re-run; skip entirely when ufw is absent.
	if sysutil.CommandExists("ufw") {
		fmt.Fprintf(opts.Stdout, "Allowing NX port %s on tailscale0...\n", nomachinePort)
		if err := sysutil.SudoRun(ctx, o, "ufw", "allow", "in", "on", "tailscale0", "to", "any", "port", nomachinePort, "proto", "tcp"); err != nil {
			return fmt.Errorf("ufw allow nomachine port: %w", err)
		}
	}

	return nil
}

func nomachineUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing NoMachine...")

	if sysutil.CommandExists("ufw") {
		// A missing rule makes ufw exit non-zero; that's not fatal to removal.
		if err := sysutil.SudoRun(ctx, o, "ufw", "delete", "allow", "in", "on", "tailscale0", "to", "any", "port", nomachinePort, "proto", "tcp"); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: could not remove ufw rule for port %s: %v\n", nomachinePort, err)
		}
	}

	return sysutil.RemovePackage(ctx, o, "nomachine")
}
