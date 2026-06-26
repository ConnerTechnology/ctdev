package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// Pi-hole runs as a container (official pihole/pihole image) with host
// networking so it owns the host's :53 (DNS) and :80 (web, behind Caddy). The
// stack lives in ~/pihole/; gravity.db and config persist in ./etc-pihole.
// Reproduce the lists and settings with `ctdev restore pihole` and
// `ctdev configure pihole`.

func piholeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "pihole")
}

func piholeInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if p.PackageManager != "apt" {
		return unsupportedPMError("pihole", p.PackageManager)
	}
	if !sysutil.CommandExists("docker") {
		return fmt.Errorf("docker is required — install the 'docker' component first")
	}

	dir := piholeDir()
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy Pi-hole stack → %s and docker compose up\n", dir)
		return nil
	}

	if err := sysutil.DeployFileFromFS(Configs, "configs/pihole/docker-compose.yml", filepath.Join(dir, "docker-compose.yml")); err != nil {
		return fmt.Errorf("deploy docker-compose.yml: %w", err)
	}
	for _, sub := range []string{"etc-pihole", "etc-dnsmasq.d"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return err
		}
	}

	// dnsmasq tuning drop-in (e.g. dns-forward-max). Read by FTL on container
	// (re)start; harmless to redeploy.
	if err := sysutil.DeployFileFromFS(Configs, "configs/pihole/dnsmasq.d/01-ctdev.conf", filepath.Join(dir, "etc-dnsmasq.d", "01-ctdev.conf")); err != nil {
		return fmt.Errorf("deploy dnsmasq tuning: %w", err)
	}

	// Unbound tuning drop-in (more threads + larger TCP backlog). Single-file
	// bind mount into the image's custom.conf.d (see docker-compose.yml), read
	// on Unbound (re)start; harmless to redeploy.
	if err := sysutil.DeployFileFromFS(Configs, "configs/pihole/unbound.conf.d/tuning.conf", filepath.Join(dir, "unbound.conf.d", "tuning.conf")); err != nil {
		return fmt.Errorf("deploy unbound tuning: %w", err)
	}

	compose := filepath.Join(dir, "docker-compose.yml")
	if err := sysutil.Run(ctx, o, "docker", "compose", "-f", compose, "up", "-d"); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}

	fmt.Fprintln(opts.Stdout, "Pi-hole container up.")
	fmt.Fprintln(opts.Stdout, "  Set an admin password: docker exec -it pihole pihole setpassword")
	fmt.Fprintln(opts.Stdout, "  Reproduce lists/config: ctdev restore pihole && ctdev configure pihole --batch")
	return nil
}

func piholeUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	compose := filepath.Join(piholeDir(), "docker-compose.yml")
	if _, err := os.Stat(compose); err == nil {
		_ = sysutil.Run(ctx, o, "docker", "compose", "-f", compose, "down")
	}
	fmt.Fprintln(opts.Stdout, "Pi-hole container stopped. ~/pihole/ kept (etc-pihole holds your lists/config).")
	return nil
}
