package component

import (
	"context"
	"fmt"
)

// Portainer CE runs as a container (official portainer/portainer-ce image) with
// the Docker socket mounted so it can manage this host. The stack lives in
// ~/portainer/; users and settings persist in the portainer_data volume. Caddy
// reverse-proxies it at https://portainer.<domain> (port 9000); it is also
// reachable directly at https://<node>:9443.

var portainerStack = composeStack{
	Name:  "portainer",
	Files: [][2]string{{"docker-compose.yml", "docker-compose.yml"}},
}

func portainerInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	if done, err := portainerStack.preflight(o); done || err != nil {
		return err
	}
	if err := portainerStack.deploy(); err != nil {
		return err
	}
	if err := portainerStack.up(ctx, o); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}

	fmt.Fprintln(opts.Stdout, "Portainer container up.")
	fmt.Fprintln(opts.Stdout, "  Create the admin user at https://<node>:9443 (or https://portainer.<domain> via Caddy).")
	return nil
}

func portainerUninstall(ctx context.Context, opts ExecOpts) error {
	portainerStack.down(ctx, opts, "Portainer container stopped. ~/portainer/ kept (the portainer_data volume holds users/settings).", false)
	return nil
}
