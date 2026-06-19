package component

import (
	"context"
	"fmt"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// piholeInstall installs Pi-hole via its official unattended installer, then
// applies our base config (Cloudflare upstreams, listen on all interfaces).
func piholeInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if p.PackageManager != "apt" {
		return unsupportedPMError("pihole", p.PackageManager)
	}

	// Phase 1: install the binary (skip if present unless --force).
	if opts.Force || !sysutil.CommandExists("pihole") {
		fmt.Fprintln(opts.Stdout, "Installing Pi-hole (official unattended installer)...")
		if err := piholeSeedSetupVars(ctx, opts); err != nil {
			return fmt.Errorf("seed setupVars: %w", err)
		}
		if o.DryRun {
			fmt.Fprintln(o.Stdout, "[dry-run] curl -sSL https://install.pi-hole.net | sudo bash /dev/stdin --unattended")
		} else if err := sysutil.Run(ctx, o, "bash", "-c",
			"curl -sSL https://install.pi-hole.net | sudo bash /dev/stdin --unattended"); err != nil {
			return fmt.Errorf("pi-hole installer: %w", err)
		}
	} else {
		fmt.Fprintln(opts.Stdout, "Pi-hole already installed")
	}

	// Phase 2: always apply base config (keeps nodes consistent).
	fmt.Fprintln(opts.Stdout, "Applying Pi-hole base config (upstreams, listen mode)...")
	_ = sysutil.SudoRun(ctx, o, "pihole-FTL", "--config", "dns.upstreams", `["1.1.1.1","1.0.0.1"]`)
	_ = sysutil.SudoRun(ctx, o, "pihole-FTL", "--config", "dns.listeningMode", "ALL")
	if !o.DryRun {
		_ = sysutil.SudoRun(ctx, o, "systemctl", "restart", "pihole-FTL")
	}
	fmt.Fprintln(opts.Stdout, "Set an admin password with: sudo pihole setpassword")
	return nil
}

// piholeSeedSetupVars pre-writes /etc/pihole/setupVars.conf so the unattended
// installer has sane defaults (interface, upstreams, listen on all interfaces).
func piholeSeedSetupVars(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	iface := piholeDefaultIface(ctx, opts)
	if iface == "" {
		iface = "eth0"
	}
	vars := strings.Join([]string{
		"PIHOLE_INTERFACE=" + iface,
		"PIHOLE_DNS_1=1.1.1.1",
		"PIHOLE_DNS_2=1.0.0.1",
		"QUERY_LOGGING=true",
		"INSTALL_WEB_SERVER=true",
		"INSTALL_WEB_INTERFACE=true",
		"LIGHTTPD_ENABLED=false",
		"DNSMASQ_LISTENING=all",
		"BLOCKING_ENABLED=true",
	}, "\n") + "\n"
	return sysutil.SudoWriteFile(ctx, o, vars, "/etc/pihole/setupVars.conf")
}

func piholeDefaultIface(ctx context.Context, opts ExecOpts) string {
	if opts.DryRun {
		return ""
	}
	out, err := captureOutput(ctx, "sh", "-c", "ip route show default | awk '{print $5; exit}'")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func piholeUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Pi-hole...")
	if sysutil.CommandExists("pihole") {
		return sysutil.SudoRun(ctx, o, "pihole", "uninstall")
	}
	return nil
}
