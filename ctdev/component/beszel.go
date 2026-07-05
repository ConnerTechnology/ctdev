package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// Beszel is lightweight server/container monitoring. The stack lives in
// ~/beszel/ and runs two containers from a single compose file: the hub (web UI
// + embedded data store, published on :8090, reverse-proxied by Caddy at
// https://beszel.<domain>) and an agent that collects this host's metrics over a
// shared unix socket. The agent's KEY/TOKEN, issued by the hub's "Add System"
// dialog, live in ~/beszel/.env; the hub comes up first so they can be obtained.

func beszelDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "beszel")
}

// beszelEnvPath is the dotenv holding the agent's BESZEL_KEY/BESZEL_TOKEN,
// issued by the hub. Absent until the admin adds this system in the web UI.
func beszelEnvPath() string {
	return filepath.Join(beszelDir(), ".env")
}

func beszelInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if p.PackageManager != "apt" {
		return unsupportedPMError("beszel", p.PackageManager)
	}
	if !sysutil.CommandExists("docker") {
		return fmt.Errorf("docker is required — install the 'docker' component first")
	}

	dir := beszelDir()
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy Beszel stack → %s and docker compose up\n", dir)
		return nil
	}

	if err := sysutil.DeployFileFromFS(Configs, "configs/beszel/docker-compose.yml", filepath.Join(dir, "docker-compose.yml")); err != nil {
		return fmt.Errorf("deploy docker-compose.yml: %w", err)
	}

	compose := filepath.Join(dir, "docker-compose.yml")

	// Bring the hub up first so the admin account can be created and a system
	// added — that dialog issues the agent's KEY/TOKEN.
	if err := sysutil.Run(ctx, o, "docker", "compose", "-f", compose, "up", "-d", "beszel"); err != nil {
		return fmt.Errorf("docker compose up (hub): %w", err)
	}

	// Start the agent only once its credentials are present in ~/beszel/.env.
	env := map[string]string{}
	if b, err := os.ReadFile(beszelEnvPath()); err == nil {
		// The file is hand-pasted, so a default umask leaves the KEY/TOKEN
		// world-readable; tighten it the way caddy's .env writer does.
		if err := os.Chmod(beszelEnvPath(), 0o600); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: could not chmod %s to 0600: %v\n", beszelEnvPath(), err)
		}
		env = parseEnv(string(b))
	}
	if env["BESZEL_KEY"] != "" {
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
	o := execOpts(opts)
	compose := filepath.Join(beszelDir(), "docker-compose.yml")
	if _, err := os.Stat(compose); err == nil {
		_ = sysutil.Run(ctx, o, "docker", "compose", "-f", compose, "down")
	}
	fmt.Fprintln(opts.Stdout, "Beszel stopped. ~/beszel/ kept (the beszel_data volume holds the hub's users and history).")
	return nil
}
