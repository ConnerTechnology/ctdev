package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var flagMCPServePort int

var configureMCPEmailServerCmd = &cobra.Command{
	Use:   "mcp-email-server",
	Short: "Configure the mailboxes mcp-email-server reads",
	Long: "Add the IMAP mailboxes mcp-email-server exposes to MCP clients, and publish it to " +
		"the tailnet with `tailscale serve`. Mailbox passwords are entered here and stored " +
		"only on this host, in ~/mcp-email-server/config/config.toml (owner-only) — they are " +
		"never written to a laptop, an environment variable, or the dotfiles repo.\n\n" +
		"Upstream's `ui` subcommand needs a browser, so this walks the same setup over SSH.",
	RunE: runConfigureMCPEmailServer,
}

func init() {
	configureMCPEmailServerCmd.Flags().IntVar(&flagMCPServePort, "serve-port", 0,
		"tailnet port to publish on (default 443, or 8443 when caddy already owns 443 on this node)")
	configureCmd.AddCommand(configureMCPEmailServerCmd)
}

// mailPreset saves looking up the IMAP/SMTP hosts for the services actually in
// use. "Other" is always available for anything not listed.
type mailPreset struct {
	label    string
	imapHost string
	imapPort int
	smtpHost string
	smtpPort int
}

var mailPresets = []mailPreset{
	{"iCloud", "imap.mail.me.com", 993, "smtp.mail.me.com", 587},
	{"Gmail / Google Workspace", "imap.gmail.com", 993, "smtp.gmail.com", 465},
	{"Outlook / Microsoft 365", "outlook.office365.com", 993, "smtp.office365.com", 587},
	{"Fastmail", "imap.fastmail.com", 993, "smtp.fastmail.com", 465},
	{"Other (enter hosts manually)", "", 993, "", 465},
}

func runConfigureMCPEmailServer(cmd *cobra.Command, args []string) error {
	return cancelToClean(configureMCPEmailServer(cmdContext(cmd)))
}

func configureMCPEmailServer(ctx context.Context) error {
	if !component.MCPEmailServerDeployed() {
		return fmt.Errorf("stack not deployed — run 'ctdev install mcp-email-server' first")
	}

	// Checked before anything reads the account list, so a profile apply fails
	// fast instead of starting a container to tell the user it can't continue.
	if isBatchMode() && !flagConfigShow {
		// Every field here is a mailbox credential; there is nothing sensible
		// to apply without asking.
		return fmt.Errorf("mcp-email-server mailboxes must be entered interactively")
	}

	accounts, err := component.MCPEmailServerAccounts(ctx)
	if err != nil {
		return err
	}
	if flagConfigShow {
		return showMCPEmailServerConfig(ctx, accounts)
	}

	fmt.Println(styles.Title.Render("mcp-email-server mailboxes"))
	fmt.Println()
	fmt.Println(styles.Dimmed.Render("  Use an app-specific password, not your account password — iCloud and Gmail"))
	fmt.Println(styles.Dimmed.Render("  require one for IMAP, and it can be revoked without touching the account."))
	fmt.Println()

	if err := editMCPEmailAccounts(ctx, accounts); err != nil {
		return err
	}
	fmt.Println()

	// Unconditional, not just after an edit: this is also how a node that was
	// configured before `tailscale up` gets its endpoint published.
	return publishMCPEmailServer(ctx)
}

func printMCPEmailAccounts(accounts []map[string]any) {
	if len(accounts) == 0 {
		fmt.Println(styles.Dimmed.Render("  No mailboxes configured yet."))
		return
	}
	for _, a := range accounts {
		note := "read-only"
		if a["has_outgoing"] == true {
			note = "can send"
		}
		if a["enabled"] == false {
			note = "disabled"
		}
		fmt.Printf("  %s %s\n",
			styles.Value.Render(fmt.Sprint(a["name"])),
			styles.Dimmed.Render(fmt.Sprintf("%s — %s", a["email_address"], note)))
	}
}

// editMCPEmailAccounts loops over add/remove until the user is done.
func editMCPEmailAccounts(ctx context.Context, accounts []map[string]any) error {
	for {
		printMCPEmailAccounts(accounts)
		fmt.Println()
		fmt.Println("  1) Add or update a mailbox")
		fmt.Println("  2) Remove a mailbox")
		fmt.Println("  3) Done")
		fmt.Printf("  %s ", styles.Dimmed.Render("Choice [3]:"))
		choice, err := promptChoiceCtx(ctx, 3)
		if err != nil {
			return err
		}
		fmt.Println()

		switch choice {
		case 1:
			if err := addMCPEmailAccount(ctx); err != nil {
				return err
			}
		case 2:
			if len(accounts) == 0 {
				fmt.Println(styles.Warning.Render("  Nothing to remove."))
				continue
			}
			name, err := promptRequiredCtx(ctx, "Account name to remove", "")
			if err != nil {
				return err
			}
			if err := component.MCPEmailServerRemoveAccount(ctx, name); err != nil {
				return err
			}
			fmt.Printf("  Removed %s\n", name)
		default:
			return nil
		}

		fmt.Println()
		if accounts, err = component.MCPEmailServerAccounts(ctx); err != nil {
			return err
		}
	}
}

func addMCPEmailAccount(ctx context.Context) error {
	for i, p := range mailPresets {
		fmt.Printf("  %d) %s\n", i+1, p.label)
	}
	fmt.Printf("  %s ", styles.Dimmed.Render("Provider [1]:"))
	pick, err := promptChoiceCtx(ctx, 1)
	if err != nil {
		return err
	}
	if pick < 1 || pick > len(mailPresets) {
		pick = 1
	}
	preset := mailPresets[pick-1]
	fmt.Println()

	address, err := promptRequiredCtx(ctx, "Email address", "")
	if err != nil {
		return err
	}
	// The account name is what an MCP client asks for by name, so default it to
	// the local part rather than the whole address.
	local, domain, _ := strings.Cut(address, "@")
	name, err := promptRequiredCtx(ctx, "Account name (how you'll refer to it)", local)
	if err != nil {
		return err
	}
	fullName, err := promptRequiredCtx(ctx, "Display name", local)
	if err != nil {
		return err
	}
	imapHost, err := promptRequiredCtx(ctx, "IMAP host", firstNonEmpty(preset.imapHost, "imap."+domain))
	if err != nil {
		return err
	}
	imapPort, err := promptPortCtx(ctx, "IMAP port", preset.imapPort)
	if err != nil {
		return err
	}
	user, err := promptRequiredCtx(ctx, "IMAP username", address)
	if err != nil {
		return err
	}
	password, err := promptSecretRequiredCtx(ctx, "App-specific password", "")
	if err != nil {
		return err
	}

	// Upstream takes the password on stdin and is explicit that credentials must
	// never appear in argv, which /proc exposes.
	args := []string{"account", "add", name,
		"--email", address,
		"--full-name", fullName,
		"--imap-host", imapHost,
		"--imap-port", strconv.Itoa(imapPort),
		"--imap-user", user,
		"--password-stdin", "--json"}

	// Sending is opt-in per account: an account with no SMTP host can only read,
	// which is the right default for a mailbox an agent has access to.
	send, err := promptYesNoCtx(ctx, "Allow sending from this account?", false)
	if err != nil {
		return err
	}
	if send {
		smtpHost, err := promptRequiredCtx(ctx, "SMTP host", firstNonEmpty(preset.smtpHost, "smtp."+domain))
		if err != nil {
			return err
		}
		smtpPort, err := promptPortCtx(ctx, "SMTP port", preset.smtpPort)
		if err != nil {
			return err
		}
		args = append(args, "--smtp-host", smtpHost, "--smtp-port", strconv.Itoa(smtpPort), "--smtp-user", user)
		// 465 is implicit TLS; submission (587) and SMTP (25) upgrade with STARTTLS.
		if smtpPort == 465 {
			args = append(args, "--smtp-ssl", "--no-smtp-starttls")
		} else {
			args = append(args, "--no-smtp-ssl", "--smtp-starttls")
		}
	}

	if flagDryRun {
		fmt.Printf("  [dry-run] would add mailbox %s to %s\n", name, component.MCPEmailServerCatalogPath())
		return nil
	}

	// Upstream rejects a duplicate name outright ("Account name already exists"),
	// so re-running configure to fix a typo or rotate a password has to clear the
	// old record first. Only a genuine removal failure is fatal — "no account
	// named X" is the normal first-time case.
	if existing, err := component.MCPEmailServerAccounts(ctx); err == nil {
		for _, a := range existing {
			if fmt.Sprint(a["name"]) == name {
				if err := component.MCPEmailServerRemoveAccount(ctx, name); err != nil {
					return fmt.Errorf("replace existing account %s: %w", name, err)
				}
				break
			}
		}
	}

	if _, err := component.MCPEmailServerCLI(ctx, password, args...); err != nil {
		return err
	}
	fmt.Printf("  Added %s\n", name)

	// Prove the credential works now, while the user still has the password to
	// hand — a wrong app password would otherwise surface as an empty inbox days
	// later.
	fmt.Println(styles.Dimmed.Render("  Testing IMAP login..."))
	if _, err := component.MCPEmailServerCLI(ctx, "", "account", "test", name, "--json"); err != nil {
		fmt.Println(styles.Warning.Render(fmt.Sprintf("  IMAP test failed: %v", err)))
		fmt.Println(styles.Dimmed.Render("  The account is saved — fix it by adding it again with the same name."))
		return nil
	}
	fmt.Println(styles.Success.Render("  IMAP login OK"))
	return nil
}

func promptPortCtx(ctx context.Context, label string, def int) (int, error) {
	for {
		v, err := promptWithDefaultCtx(ctx, label, strconv.Itoa(def))
		if err != nil {
			return 0, err
		}
		n, convErr := strconv.Atoi(v)
		if convErr == nil && n > 0 && n < 65536 {
			return n, nil
		}
		fmt.Println(styles.Warning.Render("  not a port number — try again"))
	}
}

// publishMCPEmailServer records the Host headers the MCP transport will accept,
// restarts the stack so a config change takes effect, and puts `tailscale serve`
// in front of the loopback port.
func publishMCPEmailServer(ctx context.Context) error {
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}

	servePort := flagMCPServePort
	if servePort == 0 {
		servePort = component.MCPEmailServerServePort()
	}
	if servePort != 443 {
		fmt.Printf("  Publishing on tailnet port %d — 443 on this node's tailnet address\n", servePort)
		fmt.Println(styles.Dimmed.Render("  already serves Caddy's *.<domain> sites. Override with --serve-port."))
		fmt.Println()
	}

	dnsName := component.MCPEmailServerTailscaleDNSName(ctx)
	if dnsName == "" {
		fmt.Println(styles.Warning.Render("  Tailscale isn't up — skipping the tailnet published endpoint."))
		fmt.Println(styles.Dimmed.Render("  Run 'sudo tailscale up', then re-run 'ctdev configure mcp-email-server'."))
	}
	if err := component.MCPEmailServerWriteTailnetEnv(dnsName, servePort); err != nil {
		return fmt.Errorf("write %s: %w", component.MCPEmailServerEnvPath(), err)
	}
	if err := component.MCPEmailServerComposeUp(ctx, o); err != nil {
		return fmt.Errorf("restart mcp-email-server: %w", err)
	}

	if mode := component.MCPEmailServerCatalogMode(); mode != 0 {
		fmt.Printf("  Mailbox credentials: %s (mode %04o, this host only)\n", component.MCPEmailServerCatalogPath(), mode)
		if mode != 0o600 {
			fmt.Println(styles.Warning.Render(fmt.Sprintf("  expected mode 0600 — fix with: chmod 600 %s", component.MCPEmailServerCatalogPath())))
		}
	}

	if dnsName == "" {
		return nil
	}
	if !flagDryRun {
		if err := ensureSudo(ctx); err != nil {
			return fmt.Errorf("sudo required for tailscale serve: %w", err)
		}
	}
	if err := component.MCPEmailServerServe(ctx, o, servePort); err != nil {
		fmt.Println(styles.Warning.Render(fmt.Sprintf("  tailscale serve failed: %v", err)))
		fmt.Println(styles.Dimmed.Render("  It needs HTTPS certificates enabled for the tailnet (admin console → DNS → HTTPS Certificates)."))
		return nil
	}
	url := component.MCPEmailServerURL(dnsName, servePort)
	fmt.Printf("\n  Reachable from the tailnet at %s\n", styles.Value.Render(url))
	fmt.Println(styles.Dimmed.Render("  Add it on a laptop with:"))
	fmt.Printf("    claude mcp add --transport http email %s\n", url)
	return nil
}

func showMCPEmailServerConfig(ctx context.Context, accounts []map[string]any) error {
	label := styles.Label(22)
	fmt.Println(styles.Title.Render("mcp-email-server"))
	fmt.Println()
	printMCPEmailAccounts(accounts)
	fmt.Println()

	mode := "—"
	if m := component.MCPEmailServerCatalogMode(); m != 0 {
		mode = fmt.Sprintf("%04o", m)
	}
	fmt.Printf("  %s %s\n", label.Render("Credentials:"), styles.Value.Render(component.MCPEmailServerCatalogPath()))
	fmt.Printf("  %s %s\n", label.Render("Mode:"), styles.Value.Render(mode))
	fmt.Printf("  %s %s\n", label.Render("Listening on:"), styles.Value.Render(fmt.Sprintf("127.0.0.1:%d (loopback only)", component.MCPEmailServerPort)))
	fmt.Printf("  %s %s\n", label.Render("Accepted Hosts:"), styles.Value.Render(orDash(component.MCPEmailServerReadEnv()["MCP_ALLOWED_HOSTS"])))

	endpoint := "—"
	if dnsName := component.MCPEmailServerTailscaleDNSName(ctx); dnsName != "" {
		endpoint = component.MCPEmailServerURL(dnsName, component.MCPEmailServerServePort())
	}
	fmt.Printf("  %s %s\n", label.Render("Tailnet endpoint:"), styles.Value.Render(endpoint))
	return nil
}
