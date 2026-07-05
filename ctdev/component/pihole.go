package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Pi-hole runs as a container (official pihole/pihole image) with host
// networking so it owns the host's :53 (DNS) and :80 (web, behind Caddy). The
// stack lives in ~/pihole/; gravity.db and config persist in ./etc-pihole,
// which restic backs up (see `ctdev configure restic`). Tune settings with
// `ctdev configure pihole`.

// The tuning drop-ins are read by FTL / Unbound on container (re)start (the
// unbound one is a single-file bind mount into the image's custom.conf.d —
// see docker-compose.yml); both are harmless to redeploy.
var piholeStack = composeStack{
	Name: "pihole",
	Files: [][2]string{
		{"docker-compose.yml", "docker-compose.yml"},
		{"dnsmasq.d/01-ctdev.conf", "etc-dnsmasq.d/01-ctdev.conf"},
		{"unbound.conf.d/tuning.conf", "unbound.conf.d/tuning.conf"},
	},
}

func piholeInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	if done, err := piholeStack.preflight(o); done || err != nil {
		return err
	}
	if err := piholeStack.deploy(); err != nil {
		return err
	}
	// etc-pihole holds gravity.db/config; the container mounts it, so it must
	// exist (with sane ownership) before the first compose up.
	if err := os.MkdirAll(filepath.Join(piholeStack.dir(), "etc-pihole"), 0o755); err != nil {
		return err
	}
	if err := piholeStack.up(ctx, o); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}

	fmt.Fprintln(opts.Stdout, "Pi-hole container up.")
	fmt.Fprintln(opts.Stdout, "  Set an admin password: docker exec -it pihole pihole setpassword")
	fmt.Fprintln(opts.Stdout, "  Tune DNS settings:     ctdev configure pihole")
	return nil
}

func piholeUninstall(ctx context.Context, opts ExecOpts) error {
	piholeStack.down(ctx, opts, "Pi-hole container stopped. ~/pihole/ kept (etc-pihole holds your lists/config).", false)
	return nil
}
