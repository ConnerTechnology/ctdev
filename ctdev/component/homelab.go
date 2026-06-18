package component

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// homelabFiles are the stack files deployed verbatim into ~/homelab/. They are
// generic — the Caddyfile and compose file read the domain, ACME email, and
// Cloudflare token from ~/homelab/.env, so the same files work on every node.
var homelabFiles = []string{
	"Dockerfile",
	"Caddyfile",
	"docker-compose.yml",
	"sites/zz-placeholder.caddy",
}

// homelabInstall provisions the Caddy reverse-proxy stack: it decrypts this
// node's SOPS host config into ~/homelab/.env, deploys the stack, wires Pi-hole
// (free port 443 + wildcard DNS to the Tailscale IP), and brings Caddy up.
func homelabInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if p.PackageManager != "apt" {
		return unsupportedPMError("homelab", p.PackageManager)
	}
	if !sysutil.CommandExists("docker") {
		return fmt.Errorf("docker is required — install the 'docker' component first")
	}
	if !sysutil.CommandExists("sops") {
		return fmt.Errorf("sops is required to decrypt the host config — install the 'sops' component first")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, "homelab")
	envPath := filepath.Join(dir, ".env")

	env, err := homelabEnsureEnv(ctx, opts, envPath)
	if err != nil {
		return err
	}
	domain := env["HOMELAB_DOMAIN"]
	if domain == "" {
		return fmt.Errorf("HOMELAB_DOMAIN missing from host config %s", envPath)
	}

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy homelab stack → %s\n", dir)
	} else {
		for _, f := range homelabFiles {
			if err := sysutil.DeployFileFromFS(Configs, "configs/homelab/"+f, filepath.Join(dir, f)); err != nil {
				return fmt.Errorf("deploy %s: %w", f, err)
			}
		}
	}

	// Pi-hole wiring is optional — only when Pi-hole runs on this node.
	if sysutil.CommandExists("pihole") {
		if err := homelabWirePihole(ctx, opts, domain); err != nil {
			return fmt.Errorf("wire pi-hole: %w", err)
		}
	} else {
		fmt.Fprintln(opts.Stdout, "Pi-hole not found — skipping DNS wiring (install 'pihole' to enable)")
	}

	compose := filepath.Join(dir, "docker-compose.yml")
	if err := sysutil.SudoRun(ctx, o, "docker", "compose", "-f", compose, "up", "-d", "--build"); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}

	fmt.Fprintf(opts.Stdout, "Homelab proxy up — reach services at https://<name>.%s\n", domain)
	return nil
}

// homelabEnsureEnv guarantees ~/homelab/.env exists, decrypting this node's
// SOPS host config when it's missing, and returns its key/value pairs.
func homelabEnsureEnv(ctx context.Context, opts ExecOpts, envPath string) (map[string]string, error) {
	o := execOpts(opts)

	if !opts.Force {
		if b, err := os.ReadFile(envPath); err == nil {
			return parseEnv(string(b)), nil
		}
	}

	host := homelabHostName()
	src := "configs/homelab/hosts/" + host + ".sops.env"
	cipher, err := Configs.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("no homelab host config for %q (expected ctdev/component/configs/homelab/hosts/%s.sops.env)", host, host)
	}

	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] sops decrypt %s → %s\n", src, envPath)
		return map[string]string{"HOMELAB_DOMAIN": "<from-sops>"}, nil
	}

	plain, err := homelabSopsDecrypt(ctx, cipher)
	if err != nil {
		return nil, fmt.Errorf("decrypt host config (is the age key at ~/.config/sops/age/keys.txt?): %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(envPath, []byte(plain), 0o600); err != nil {
		return nil, fmt.Errorf("write %s: %w", envPath, err)
	}
	fmt.Fprintf(opts.Stdout, "Decrypted host config → %s\n", envPath)
	return parseEnv(plain), nil
}

// homelabWirePihole frees port 443 for Caddy and points *.<domain> at this
// node's Tailscale IP via a Pi-hole dnsmasq record, so names resolve the same
// on the LAN and remotely.
func homelabWirePihole(ctx context.Context, opts ExecOpts, domain string) error {
	o := execOpts(opts)

	// Pi-hole keeps plain HTTP on 80; Caddy terminates TLS on 443.
	if err := sysutil.SudoRun(ctx, o, "pihole-FTL", "--config", "webserver.port", "80o,[::]:80o"); err != nil {
		return err
	}
	if err := sysutil.SudoRun(ctx, o, "pihole-FTL", "--config", "misc.etc_dnsmasq_d", "true"); err != nil {
		return err
	}

	tsIP := homelabTailscaleIP(ctx, opts)
	if tsIP == "" {
		fmt.Fprintln(opts.Stdout, "Tailscale IP not available yet — skipping wildcard DNS record (run 'sudo tailscale up', then re-run 'ctdev install homelab --force')")
	} else {
		record := fmt.Sprintf("address=/%s/%s\n", domain, tsIP)
		if err := sysutil.SudoWriteFile(ctx, o, record, "/etc/dnsmasq.d/02-homelab.conf"); err != nil {
			return err
		}
		fmt.Fprintf(opts.Stdout, "Pi-hole resolves *.%s → %s\n", domain, tsIP)
	}

	return sysutil.SudoRun(ctx, o, "systemctl", "restart", "pihole-FTL")
}

// homelabUninstall stops the proxy stack but leaves ~/homelab/ in place.
func homelabUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	compose := filepath.Join(home, "homelab", "docker-compose.yml")
	if _, err := os.Stat(compose); err == nil {
		_ = sysutil.SudoRun(ctx, o, "docker", "compose", "-f", compose, "down")
	}
	fmt.Fprintln(opts.Stdout, "Homelab proxy stopped. ~/homelab/ kept; restore Pi-hole port 443 manually if needed.")
	return nil
}

func homelabHostName() string {
	if h := os.Getenv("CTDEV_HOMELAB_HOST"); h != "" {
		return h
	}
	h, _ := os.Hostname()
	return h
}

func homelabSopsDecrypt(ctx context.Context, cipher []byte) (string, error) {
	tmp, err := os.CreateTemp("", "homelab-*.env")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(cipher); err != nil {
		tmp.Close()
		return "", err
	}
	tmp.Close()
	return captureOutput(ctx, "sops", "--decrypt", tmp.Name())
}

func homelabTailscaleIP(ctx context.Context, opts ExecOpts) string {
	if opts.DryRun || !sysutil.CommandExists("tailscale") {
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
