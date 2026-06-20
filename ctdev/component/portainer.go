package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// Portainer CE runs as a container (official portainer/portainer-ce image) with
// the Docker socket mounted so it can manage this host. The stack lives in
// ~/portainer/; users and settings persist in the portainer_data volume. Caddy
// reverse-proxies it at https://portainer.<domain> (port 9000); it is also
// reachable directly at https://<node>:9443.

func portainerDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "portainer")
}

func portainerInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if p.PackageManager != "apt" {
		return unsupportedPMError("portainer", p.PackageManager)
	}
	if !sysutil.CommandExists("docker") {
		return fmt.Errorf("docker is required — install the 'docker' component first")
	}

	dir := portainerDir()
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy Portainer stack → %s and docker compose up\n", dir)
		return nil
	}

	if err := sysutil.DeployFileFromFS(Configs, "configs/portainer/docker-compose.yml", filepath.Join(dir, "docker-compose.yml")); err != nil {
		return fmt.Errorf("deploy docker-compose.yml: %w", err)
	}

	compose := filepath.Join(dir, "docker-compose.yml")
	if err := sysutil.Run(ctx, o, "docker", "compose", "-f", compose, "up", "-d"); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}

	fmt.Fprintln(opts.Stdout, "Portainer container up.")
	fmt.Fprintln(opts.Stdout, "  Create the admin user at https://<node>:9443 (or https://portainer.<domain> via Caddy).")
	return nil
}

func portainerUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	compose := filepath.Join(portainerDir(), "docker-compose.yml")
	if _, err := os.Stat(compose); err == nil {
		_ = sysutil.Run(ctx, o, "docker", "compose", "-f", compose, "down")
	}
	fmt.Fprintln(opts.Stdout, "Portainer container stopped. ~/portainer/ kept (the portainer_data volume holds users/settings).")
	return nil
}
