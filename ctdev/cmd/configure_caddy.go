package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var (
	flagCaddyDomain string
	flagCaddyEmail  string
	flagCaddyToken  string
)

var configureCaddyCmd = &cobra.Command{
	Use:   "caddy",
	Short: "Configure the Caddy reverse proxy",
	Long: "Set the wildcard domain, ACME email, and Cloudflare API token for the Caddy " +
		"reverse proxy (written to ~/caddy/.env), and wire Pi-hole DNS when Pi-hole runs " +
		"on this node. Bring the proxy up afterward with 'ctdev install caddy'.",
	RunE: runConfigureCaddy,
}

func init() {
	configureCaddyCmd.Flags().StringVar(&flagCaddyDomain, "domain", "", "wildcard domain (e.g. example.com)")
	configureCaddyCmd.Flags().StringVar(&flagCaddyEmail, "acme-email", "", "email for Let's Encrypt/ACME registration")
	configureCaddyCmd.Flags().StringVar(&flagCaddyToken, "cf-token", "", "Cloudflare API token for the DNS-01 challenge (prefer CF_API_TOKEN env or the wizard — flags land in shell history and ps)")
	configureCmd.AddCommand(configureCaddyCmd)
}

func runConfigureCaddy(cmd *cobra.Command, args []string) error {
	return cancelToClean(configureCaddy(cmdContext(cmd)))
}

// configureCaddy writes ~/caddy/.env (domain, ACME email, CF token), wires
// Pi-hole DNS when Pi-hole is present, and brings the stack up if it has been
// deployed. Reused by `ctdev install caddy` so install = install + configure.
func configureCaddy(ctx context.Context) error {
	current := component.CaddyReadEnv()

	if flagConfigShow {
		return showCaddyConfig(current)
	}

	domain := firstNonEmpty(flagCaddyDomain, current["HOMELAB_DOMAIN"])
	email := firstNonEmpty(flagCaddyEmail, current["HOMELAB_ACME_EMAIL"])
	// The token can come from the CF_API_TOKEN env var so it never has to be
	// typed on a command line (shell history, /proc/<pid>/cmdline).
	token := firstNonEmpty(flagCaddyToken, os.Getenv("CF_API_TOKEN"), current["CF_API_TOKEN"])

	if !isBatchMode() {
		fmt.Println(styles.Title.Render("Caddy reverse proxy"))
		fmt.Println()
		var err error
		if domain, err = promptWithDefaultCtx(ctx, "Wildcard domain", domain); err != nil {
			return err
		}
		if email, err = promptWithDefaultCtx(ctx, "ACME email", email); err != nil {
			return err
		}
		if token, err = promptSecretCtx(ctx, "Cloudflare API token", token); err != nil {
			return err
		}
		fmt.Println()
	}

	if domain == "" {
		return fmt.Errorf("a domain is required (pass --domain, or set it in the wizard)")
	}

	if flagDryRun {
		fmt.Printf("  [dry-run] would write %s (domain=%s)\n", component.CaddyEnvPath(), domain)
	} else {
		if err := component.CaddyWriteEnv(domain, email, token); err != nil {
			return fmt.Errorf("write %s: %w", component.CaddyEnvPath(), err)
		}
		fmt.Printf("  Wrote %s\n", component.CaddyEnvPath())
	}

	// PiholeAvailable, not CommandExists: the pihole component is a container,
	// which puts no `pihole` binary on the host PATH — checking for the binary
	// silently skipped the whole wiring step on containerized nodes.
	havePihole := sysutil.PiholeAvailable()
	if !flagDryRun && (havePihole || component.CaddyStackDeployed()) {
		if err := ensureSudo(ctx); err != nil {
			return fmt.Errorf("sudo required: %w", err)
		}
	}
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}

	// Wire Pi-hole (free 443, wildcard DNS → Tailscale IP) when Pi-hole is here.
	if havePihole {
		if err := component.CaddyWirePihole(ctx, o, domain); err != nil {
			return fmt.Errorf("wire pi-hole: %w", err)
		}
	}

	// Bring the proxy up if its stack has been deployed (`ctdev install caddy`).
	if !flagDryRun && component.CaddyStackDeployed() {
		if err := component.CaddyComposeUp(ctx, o); err != nil {
			return fmt.Errorf("bring up caddy: %w", err)
		}
		fmt.Printf("  Caddy proxy up — reach services at https://<name>.%s\n", domain)
	} else {
		fmt.Println("\n  Next: deploy and start the proxy with 'ctdev install caddy'")
	}
	return nil
}

func showCaddyConfig(env map[string]string) error {
	label := styles.Label(20)
	fmt.Println(styles.Title.Render("Caddy reverse proxy"))
	fmt.Println()
	fmt.Printf("  %s %s\n", label.Render("Domain:"), styles.Value.Render(orDash(env["HOMELAB_DOMAIN"])))
	fmt.Printf("  %s %s\n", label.Render("ACME email:"), styles.Value.Render(orDash(env["HOMELAB_ACME_EMAIL"])))
	tok := "(unset)"
	if env["CF_API_TOKEN"] != "" {
		tok = "(set)"
	}
	fmt.Printf("  %s %s\n", label.Render("Cloudflare token:"), styles.Value.Render(tok))
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
