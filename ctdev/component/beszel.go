package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// Beszel is lightweight server/container monitoring. The stack lives in
// ~/beszel/ and runs two containers from a single compose file: the hub (web UI
// + embedded data store, published on :8090, reverse-proxied by Caddy at
// https://beszel.<domain>) and an agent that collects this host's metrics over a
// shared unix socket. The agent's KEY/TOKEN, issued by the hub's "Add System"
// dialog, live in ~/beszel/.env; the hub comes up first so they can be obtained.

var beszelStack = composeStack{
	Name:  "beszel",
	Files: [][2]string{{"docker-compose.yml", "docker-compose.yml"}},
}

// beszelEnvPath is the dotenv holding the agent's BESZEL_KEY/BESZEL_TOKEN,
// issued by the hub, and the hub's BESZEL_APP_URL. The credentials are absent
// until the admin adds this system in the web UI.
func beszelEnvPath() string {
	return filepath.Join(beszelStack.dir(), ".env")
}

// beszelReadEnv returns the key/value pairs in ~/beszel/.env, or an empty map.
func beszelReadEnv() map[string]string {
	b, err := os.ReadFile(beszelEnvPath())
	if err != nil {
		return map[string]string{}
	}
	return parseEnv(string(b))
}

// beszelSetEnv merges keys into ~/beszel/.env without touching the hand-pasted
// KEY/TOKEN. 0600 because the file holds those credentials.
func beszelSetEnv(values map[string]string) error {
	return mergeEnvFile(beszelEnvPath(), values, 0o600)
}

// beszelAppURL is the address the hub puts in its notification links. The hub
// defaults to http://localhost:8090, which is what every push alert's "View
// Beszel" button opens on a phone unless told the Caddy name. Empty when Caddy
// has not been configured, so the compose default stays in effect.
func beszelAppURL(homelabDomain string) string {
	if homelabDomain == "" {
		return ""
	}
	return "https://beszel." + homelabDomain
}

func beszelInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	if done, err := beszelStack.preflight(o); done || err != nil {
		return err
	}
	if err := beszelStack.deploy(); err != nil {
		return err
	}

	compose := beszelStack.composePath()

	if appURL := beszelAppURL(CaddyReadEnv()["HOMELAB_DOMAIN"]); appURL != "" {
		if err := beszelSetEnv(map[string]string{"BESZEL_APP_URL": appURL}); err != nil {
			return fmt.Errorf("write %s: %w", beszelEnvPath(), err)
		}
	}

	// Bring the hub up first so the admin account can be created and a system
	// added — that dialog issues the agent's KEY/TOKEN.
	if err := sysutil.Run(ctx, o, "docker", "compose", "-f", compose, "up", "-d", "beszel"); err != nil {
		return fmt.Errorf("docker compose up (hub): %w", err)
	}

	// Start the agent only once its credentials are present in ~/beszel/.env.
	if _, err := os.Stat(beszelEnvPath()); err == nil {
		// The credentials are hand-pasted, so a default umask leaves the
		// KEY/TOKEN world-readable; tighten it the way caddy's .env writer does.
		if err := os.Chmod(beszelEnvPath(), 0o600); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: could not chmod %s to 0600: %v\n", beszelEnvPath(), err)
		}
	}
	if beszelReadEnv()["BESZEL_KEY"] != "" {
		if err := sysutil.Run(ctx, o, "docker", "compose", "-f", compose, "up", "-d"); err != nil {
			return fmt.Errorf("docker compose up (agent): %w", err)
		}
		fmt.Fprintln(opts.Stdout, "Beszel hub + agent up.")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Beszel hub up.")
	fmt.Fprintln(opts.Stdout, "  1. Open https://beszel.<domain> (or http://<node>:8090), create the admin user, click 'Add System'.")
	fmt.Fprintln(opts.Stdout, "  2. Put the shown KEY and TOKEN in ~/beszel/.env (BESZEL_KEY=, BESZEL_TOKEN=).")
	fmt.Fprintln(opts.Stdout, "  3. Re-run 'ctdev install beszel' to start the agent.")
	return nil
}

func beszelUninstall(ctx context.Context, opts ExecOpts) error {
	beszelStack.down(ctx, opts, "Beszel stopped. ~/beszel/ kept (the beszel_data volume holds the hub's users and history).", false)
	return nil
}
