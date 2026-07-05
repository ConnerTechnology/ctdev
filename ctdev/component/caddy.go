package component

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// The stack files are generic — the Caddyfile and compose file read the
// domain, ACME email, and Cloudflare token from ~/caddy/.env, so the same
// files work on every node.
var caddyStack = composeStack{
	Name: "caddy",
	Files: [][2]string{
		{"Dockerfile", "Dockerfile"},
		{"Caddyfile", "Caddyfile"},
		{"docker-compose.yml", "docker-compose.yml"},
		{"sites/zz-placeholder.caddy", "sites/zz-placeholder.caddy"},
	},
}

// CaddyDir is the on-disk location of the reverse-proxy stack.
func CaddyDir() string {
	return caddyStack.dir()
}

// CaddyEnvPath is the dotenv the Caddyfile and compose file read the domain,
// ACME email, and Cloudflare token from. Written by `ctdev configure caddy`.
func CaddyEnvPath() string {
	return filepath.Join(CaddyDir(), ".env")
}

// caddyInstall deploys the Caddy reverse-proxy stack and brings it up. The
// domain/token come from ~/caddy/.env, which `ctdev configure caddy` writes.
func caddyInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	if done, err := caddyStack.preflight(o); done || err != nil {
		return err
	}
	if err := caddyStack.deploy(); err != nil {
		return err
	}

	// The proxy needs ~/caddy/.env (domain + Cloudflare token), written by
	// `ctdev configure caddy`. Without it, leave the files in place but don't
	// start — `ctdev install caddy` chains into configure, which brings it up.
	if _, err := os.Stat(CaddyEnvPath()); err != nil {
		fmt.Fprintln(opts.Stdout, "Deployed caddy stack to ~/caddy. Set the domain/token next ('ctdev configure caddy') to start the proxy.")
		return nil
	}

	if err := CaddyComposeUp(ctx, o); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	fmt.Fprintf(opts.Stdout, "Caddy proxy up — reach services at https://<name>.%s\n", CaddyReadEnv()["HOMELAB_DOMAIN"])
	return nil
}

// CaddyStackDeployed reports whether the stack files are present in ~/caddy.
func CaddyStackDeployed() bool {
	_, err := os.Stat(filepath.Join(CaddyDir(), "docker-compose.yml"))
	return err == nil
}

// CaddyComposeUp builds and (re)starts the Caddy stack. A no-op when the stack
// hasn't been deployed yet (run `ctdev install caddy` to deploy it).
func CaddyComposeUp(ctx context.Context, o sysutil.Opts) error {
	if !CaddyStackDeployed() {
		return nil
	}
	compose := filepath.Join(CaddyDir(), "docker-compose.yml")
	return sysutil.SudoRun(ctx, o, "docker", "compose", "-f", compose, "up", "-d", "--build")
}

// caddyUninstall stops the proxy stack but leaves ~/caddy/ in place.
func caddyUninstall(ctx context.Context, opts ExecOpts) error {
	caddyStack.down(ctx, opts, "Caddy proxy stopped. ~/caddy/ kept; restore Pi-hole port 443 manually if needed.", true)
	return nil
}

// CaddyReadEnv returns the key/value pairs in ~/caddy/.env, or an empty map.
func CaddyReadEnv() map[string]string {
	b, err := os.ReadFile(CaddyEnvPath())
	if err != nil {
		return map[string]string{}
	}
	return parseEnv(string(b))
}

// CaddyWriteEnv writes ~/caddy/.env (0600) with the proxy's domain, ACME email,
// and Cloudflare API token.
func CaddyWriteEnv(domain, email, token string) error {
	if err := os.MkdirAll(CaddyDir(), 0o755); err != nil {
		return err
	}
	host, _ := os.Hostname()
	var b strings.Builder
	fmt.Fprintf(&b, "HOSTNAME=%s\n", host)
	fmt.Fprintf(&b, "HOMELAB_DOMAIN=%s\n", domain)
	fmt.Fprintf(&b, "HOMELAB_ACME_EMAIL=%s\n", email)
	fmt.Fprintf(&b, "CF_API_TOKEN=%s\n", token)
	return os.WriteFile(CaddyEnvPath(), []byte(b.String()), 0o600)
}

// CaddyWirePihole frees port 443 for Caddy and points *.<domain> at this node's
// Tailscale IP via a Pi-hole dnsmasq record, so names resolve the same on the
// LAN and remotely. Skips the DNS record (with a note) until Tailscale is up.
func CaddyWirePihole(ctx context.Context, o sysutil.Opts, domain string) error {
	// Pi-hole keeps plain HTTP on 80; Caddy terminates TLS on 443.
	if err := sysutil.PiholeRun(ctx, o, "pihole-FTL", "--config", "webserver.port", "80o,[::]:80o"); err != nil {
		return err
	}
	if err := sysutil.PiholeRun(ctx, o, "pihole-FTL", "--config", "misc.etc_dnsmasq_d", "true"); err != nil {
		return err
	}

	tsIP := caddyTailscaleIP(ctx)
	if tsIP == "" {
		fmt.Fprintln(o.Stdout, "Tailscale IP not available yet — skipping wildcard DNS record (run 'sudo tailscale up', then re-run 'ctdev configure caddy')")
	} else {
		record := fmt.Sprintf("address=/%s/%s\n", domain, tsIP)
		if err := writeDnsmasqRecord(ctx, o, record); err != nil {
			return err
		}
		fmt.Fprintf(o.Stdout, "Pi-hole resolves *.%s → %s\n", domain, tsIP)
	}

	return sysutil.PiholeReload(ctx, o)
}

// writeDnsmasqRecord writes Pi-hole's dnsmasq drop-in. For a containerized
// Pi-hole it lands in the bind-mounted ~/pihole/etc-dnsmasq.d (no sudo); for a
// host install, in /etc/dnsmasq.d (sudo).
func writeDnsmasqRecord(ctx context.Context, o sysutil.Opts, record string) error {
	if sysutil.PiholeContainerized() {
		home, _ := os.UserHomeDir()
		dest := filepath.Join(home, "pihole", "etc-dnsmasq.d", "02-homelab.conf")
		if o.DryRun {
			fmt.Fprintf(o.Stdout, "[dry-run] write dnsmasq record → %s\n", dest)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, []byte(record), 0o644)
	}
	return sysutil.SudoWriteFile(ctx, o, record, "/etc/dnsmasq.d/02-homelab.conf")
}

func caddyTailscaleIP(ctx context.Context) string {
	if !sysutil.CommandExists("tailscale") {
		return ""
	}
	out, err := captureOutput(ctx, "tailscale", "ip", "-4")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// captureOutput runs a command and returns its stdout, respecting ctx.
func captureOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// parseEnv turns KEY=VALUE dotenv lines into a map, ignoring blanks/comments.
func parseEnv(s string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		m[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return m
}
