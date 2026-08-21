package component

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// mcp-email-server reads mailboxes over IMAP and exposes them to MCP clients,
// so laptops reach mail through the tailnet instead of storing a credential.
// The stack lives in ~/mcp-email-server/: the managed credential catalog in
// ./config, and node-local settings in ./.env. Upstream ships no
// authentication, so the loopback-only port binding and `tailscale serve` in
// front of it are what make the service safe to run — see docker-compose.yml.
//
// Tool surface exposed by the pinned version (1.4.1), read from a live
// tools/list, with the annotations the server reports:
//
//	list_available_accounts   readOnly, idempotent
//	list_emails_metadata      readOnly, idempotent, openWorld
//	get_emails_content        idempotent, openWorld
//	list_allowed_recipients   readOnly, idempotent
//	list_allowed_senders      readOnly, idempotent
//	list_mailboxes            readOnly, idempotent, openWorld
//	send_email                openWorld
//	save_to_mailbox           openWorld
//	set_email_flags           idempotent, openWorld
//	mark_emails_as_read       idempotent, openWorld
//	delete_emails             destructive, openWorld
//	move_emails               destructive, openWorld
//	archive_emails            destructive, openWorld
//	download_attachment       destructive, openWorld
//
// ConnerTechnology/brain blocks dangerous tools by EXACT NAME in
// config/settings.json (permissions.deny), so this list is a coupling point:
// a tool that is added or renamed by a version bump silently escapes that
// block list. Re-check it whenever the pin in the Dockerfile moves.

// MCPEmailServerPort is the loopback port the stack publishes and the port
// `tailscale serve` forwards to.
const MCPEmailServerPort = 9557

// mcpEmailServerService is the compose service name, used for `compose run`.
const mcpEmailServerService = "mcp-email-server"

var mcpEmailServerStack = composeStack{
	Name: "mcp-email-server",
	Files: [][2]string{
		{"Dockerfile", "Dockerfile"},
		{"docker-compose.yml", "docker-compose.yml"},
	},
}

// MCPEmailServerDir is the on-disk location of the stack.
func MCPEmailServerDir() string { return mcpEmailServerStack.dir() }

// MCPEmailServerConfigDir holds the managed catalog and its bootstrap sidecar.
// A bind mount, not a named volume, so `docker compose down` leaves it alone
// and the host can check the catalog's mode without entering the container.
// Upstream refuses to open a catalog whose parent is not owner-only.
func MCPEmailServerConfigDir() string { return filepath.Join(MCPEmailServerDir(), "config") }

// MCPEmailServerCatalogPath is the managed SQLite catalog. It holds every
// mailbox password, unencrypted at rest — the 0600 mode and this host's
// isolation are what protect them.
func MCPEmailServerCatalogPath() string {
	return filepath.Join(MCPEmailServerConfigDir(), "managed.sqlite3")
}

// MCPEmailServerEnvPath is the dotenv carrying node-local settings: the uid/gid
// the container runs as, the Host headers the MCP transport accepts, and the
// tailnet port. Nothing secret — the secrets are all in the catalog.
func MCPEmailServerEnvPath() string { return filepath.Join(MCPEmailServerDir(), ".env") }

// MCPEmailServerDeployed reports whether the stack files are in place.
func MCPEmailServerDeployed() bool {
	_, err := os.Stat(mcpEmailServerStack.composePath())
	return err == nil
}

// MCPEmailServerReadEnv returns the key/value pairs in ~/mcp-email-server/.env,
// or an empty map.
func MCPEmailServerReadEnv() map[string]string {
	b, err := os.ReadFile(MCPEmailServerEnvPath())
	if err != nil {
		return map[string]string{}
	}
	return parseEnv(string(b))
}

// MCPEmailServerSetEnv merges keys into ~/mcp-email-server/.env. Install writes
// the uid/gid and configure writes the tailnet settings, so neither may clobber
// the other's keys.
func MCPEmailServerSetEnv(values map[string]string) error {
	if err := os.MkdirAll(MCPEmailServerDir(), 0o755); err != nil {
		return err
	}
	env := MCPEmailServerReadEnv()
	for k, v := range values {
		env[k] = v
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, env[k])
	}
	return os.WriteFile(MCPEmailServerEnvPath(), []byte(b.String()), 0o644)
}

// MCPEmailServerWriteTailnetEnv records which Host headers the MCP transport
// accepts and the tailnet port the endpoint is served on. Everything reaches
// the server through `tailscale serve`, which forwards the client's original
// Host, so the node's MagicDNS name has to be listed or every tailnet request
// is rejected with 421. The bare name covers every port: upstream expands an
// entry without a colon to "<name>" and "<name>:*".
func MCPEmailServerWriteTailnetEnv(dnsName string, servePort int) error {
	hosts := []string{"127.0.0.1", "localhost", "[::1]"}
	origins := []string{"http://127.0.0.1", "http://localhost"}
	if dnsName != "" {
		hosts = append(hosts, dnsName)
		origins = append(origins, "https://"+dnsName)
	}
	return MCPEmailServerSetEnv(map[string]string{
		"MCP_ALLOWED_HOSTS":   strings.Join(hosts, ","),
		"MCP_ALLOWED_ORIGINS": strings.Join(origins, ","),
		"MCP_SERVE_PORT":      strconv.Itoa(servePort),
	})
}

// MCPEmailServerServePort is the tailnet port `tailscale serve` publishes on.
// 443 is the natural choice, but on a node already running the caddy component
// it is not free: `ctdev configure caddy` points *.<domain> at this node's
// TAILSCALE IP, and Caddy answers there on 443. A serve rule on 443 intercepts
// that port for the node's own tailnet addresses, so every https://<svc>.<domain>
// on the tailnet would start hitting the email server instead of Caddy.
func MCPEmailServerServePort() int {
	if port, err := strconv.Atoi(MCPEmailServerReadEnv()["MCP_SERVE_PORT"]); err == nil && port > 0 {
		return port
	}
	// Checked by path rather than through FindByName: the registry references
	// this component's uninstaller, so reading Registry from here is an
	// initialization cycle.
	if _, err := os.Stat(caddyStack.composePath()); err == nil {
		return 8443
	}
	return 443
}

// MCPEmailServerComposeUp rebuilds and (re)starts the stack, picking up a
// changed .env. A no-op until the stack has been deployed.
func MCPEmailServerComposeUp(ctx context.Context, o sysutil.Opts) error {
	if !MCPEmailServerDeployed() {
		return nil
	}
	return sysutil.Run(ctx, o, "docker", "compose", "-f", mcpEmailServerStack.composePath(), "up", "-d", "--build")
}

// MCPEmailServerCatalogMode returns the credential catalog's permission bits,
// so the installer can show that they really are owner-only rather than assert
// it. Zero when the catalog doesn't exist yet.
func MCPEmailServerCatalogMode() os.FileMode {
	fi, err := os.Stat(MCPEmailServerCatalogPath())
	if err != nil {
		return 0
	}
	return fi.Mode().Perm()
}

// mcpEmailServerResult is upstream's stable --json envelope.
type mcpEmailServerResult struct {
	OK   bool           `json:"ok"`
	Data map[string]any `json:"data"`
}

// MCPEmailServerCLI runs one upstream CLI command in a throwaway container and
// returns the parsed --json document. secret is piped to stdin for
// `account add --password-stdin`; upstream is explicit that credentials must
// never go in argv, which /proc exposes.
func MCPEmailServerCLI(ctx context.Context, secret string, args ...string) (map[string]any, error) {
	if !MCPEmailServerDeployed() {
		return nil, fmt.Errorf("stack not deployed — run 'ctdev install mcp-email-server' first")
	}

	// --no-deps keeps the running server untouched; the one-off container shares
	// the service's user, environment, and ./config mount.
	full := append([]string{"compose", "-f", mcpEmailServerStack.composePath(),
		"run", "--rm", "--no-deps", "-T", mcpEmailServerService}, args...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	if secret != "" {
		cmd.Stdin = strings.NewReader(secret + "\n")
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	runErr := cmd.Run()

	// Upstream prints its JSON envelope on stdout, but reports some failures as
	// a plain "Error: ..." line on stderr — surface whichever we got.
	var res mcpEmailServerResult
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &res); err != nil {
		return nil, fmt.Errorf("%s: %s", strings.Join(args, " "), mcpEmailServerErrText(errOut.String(), out.String(), runErr))
	}
	if !res.OK {
		return res.Data, fmt.Errorf("%s: %s", strings.Join(args, " "), mcpEmailServerErrText(errOut.String(), out.String(), runErr))
	}
	return res.Data, nil
}

// mcpEmailServerErrText picks the most useful line out of a failed CLI run.
// Upstream's own "Error: ..." beats docker's exit status, which by the time it
// reaches a summary screen says nothing at all.
func mcpEmailServerErrText(stderr, stdout string, runErr error) string {
	for _, line := range strings.Split(stderr+"\n"+stdout, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "Error: ") {
			return strings.TrimPrefix(line, "Error: ")
		}
	}
	if s := strings.TrimSpace(stderr); s != "" {
		return s
	}
	if runErr != nil {
		return runErr.Error()
	}
	return "unknown error"
}

// MCPEmailServerAccounts lists the configured mailboxes. The catalog is
// owner-only, so the container is what reads it back.
func MCPEmailServerAccounts(ctx context.Context) ([]map[string]any, error) {
	data, err := MCPEmailServerCLI(ctx, "", "account", "list", "--json")
	if err != nil {
		return nil, err
	}
	raw, _ := data["accounts"].([]any)
	accounts := make([]map[string]any, 0, len(raw))
	for _, a := range raw {
		if m, ok := a.(map[string]any); ok {
			accounts = append(accounts, m)
		}
	}
	return accounts, nil
}

// MCPEmailServerRemoveAccount deletes one mailbox. Upstream guards removal with
// optimistic concurrency and a typed confirmation, so the current revision has
// to be read back first — calling `account remove <name>` alone fails on the
// two missing required flags, and did so silently until this was tested against
// a real server.
func MCPEmailServerRemoveAccount(ctx context.Context, name string) error {
	accounts, err := MCPEmailServerAccounts(ctx)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		if fmt.Sprint(a["name"]) != name {
			continue
		}
		rev, ok := a["revision"].(float64) // encoding/json numbers
		if !ok {
			return fmt.Errorf("account %s: no revision in the catalog listing", name)
		}
		_, err := MCPEmailServerCLI(ctx, "", "account", "remove", name,
			"--expected-revision", strconv.Itoa(int(rev)), "--confirm", name, "--json")
		return err
	}
	return fmt.Errorf("no account named %q", name)
}

// MCPEmailServerInitCatalog creates the managed catalog on first install.
// Idempotent: an already-managed node is left alone.
func MCPEmailServerInitCatalog(ctx context.Context) error {
	status, err := MCPEmailServerCLI(ctx, "", "config", "status", "--json")
	if err != nil {
		return err
	}
	if status["catalog_status"] != "not_configured" {
		return nil
	}
	_, err = MCPEmailServerCLI(ctx, "", "config", "init", "--database", "/config/managed.sqlite3", "--json")
	return err
}

// MCPEmailServerTailscaleDNSName returns this node's MagicDNS name (no trailing
// dot), or "" when Tailscale isn't up yet.
func MCPEmailServerTailscaleDNSName(ctx context.Context) string {
	if !sysutil.CommandExists("tailscale") {
		return ""
	}
	out, err := captureOutput(ctx, "tailscale", "status", "--json")
	if err != nil {
		return ""
	}
	var status struct {
		Self struct {
			DNSName string
		}
	}
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		return ""
	}
	return strings.TrimSuffix(status.Self.DNSName, ".")
}

// MCPEmailServerServe puts `tailscale serve` in front of the loopback port:
// TLS from the tailnet's own certificate, reachable only by authenticated
// tailnet peers. serve, never funnel — funnel publishes to the internet.
func MCPEmailServerServe(ctx context.Context, o sysutil.Opts, servePort int) error {
	return sysutil.SudoRun(ctx, o, "tailscale", "serve", "--bg", "--yes",
		fmt.Sprintf("--https=%d", servePort), fmt.Sprintf("http://127.0.0.1:%d", MCPEmailServerPort))
}

// MCPEmailServerURL is the endpoint an MCP client connects to.
func MCPEmailServerURL(dnsName string, servePort int) string {
	host := dnsName
	if servePort != 443 {
		host = fmt.Sprintf("%s:%d", dnsName, servePort)
	}
	return "https://" + host + "/mcp"
}

// mcpEmailServerOwnImage is the tag this component builds. A container created
// from anything else is not ours, however it happens to be named.
const mcpEmailServerOwnImage = "mcp-email-server:local"

// mcpEmailServerCatalogFile is the managed catalog's basename — the marker that
// ./config belongs to this version rather than some other email server.
const mcpEmailServerCatalogFile = "managed.sqlite3"

// mcpEmailServerState is the pre-existing state install probes for before it
// touches anything. Split from the checking so the rules can be tested without
// a docker daemon or a tailnet.
type mcpEmailServerState struct {
	ContainerImage string   // .Config.Image of a container named mcp-email-server, "" if none
	ConfigEntries  []string // names directly under ./config
	ServeProxies   []string // proxy targets already bound to the serve port
}

// mcpEmailServerConflicts reports pre-existing state that installing would
// replace. It exists because this component once did exactly that: another
// implementation was serving the same role, install swapped it out, and the
// replacement answered list_available_accounts with an empty list and
// isError:false — which reads to a client as "no mailboxes configured", not
// "your accounts are gone". An assistant triaging mail against it would have
// reported a clean inbox.
func mcpEmailServerConflicts(st mcpEmailServerState) []string {
	var found []string
	if st.ContainerImage != "" && st.ContainerImage != mcpEmailServerOwnImage {
		found = append(found, fmt.Sprintf("a container named %q was created from image %q, which this component did not build",
			mcpEmailServerService, st.ContainerImage))
	}
	// A non-empty ./config without the catalog is another server's state. An
	// empty or absent directory is a fresh install and not a conflict.
	if len(st.ConfigEntries) > 0 && !slices.Contains(st.ConfigEntries, mcpEmailServerCatalogFile) {
		found = append(found, fmt.Sprintf("%s holds %s but no %s, so it belongs to a different email server",
			MCPEmailServerConfigDir(), strings.Join(st.ConfigEntries, ", "), mcpEmailServerCatalogFile))
	}
	want := fmt.Sprintf("http://127.0.0.1:%d", MCPEmailServerPort)
	for _, proxy := range st.ServeProxies {
		if proxy != want {
			found = append(found, fmt.Sprintf("tailscale serve already proxies port %d to %s", MCPEmailServerServePort(), proxy))
		}
	}
	return found
}

// mcpEmailServerInspectState gathers the state above. Every probe is read-only
// and degrades to "nothing found" when its tool is missing, so a fresh machine
// reports no conflicts rather than failing.
func mcpEmailServerInspectState(ctx context.Context) mcpEmailServerState {
	st := mcpEmailServerState{ContainerImage: sysutil.ContainerConfigImage(ctx, mcpEmailServerService)}
	if entries, err := os.ReadDir(MCPEmailServerConfigDir()); err == nil {
		for _, e := range entries {
			st.ConfigEntries = append(st.ConfigEntries, e.Name())
		}
		sort.Strings(st.ConfigEntries)
	}
	st.ServeProxies = mcpEmailServerServeProxies(ctx, MCPEmailServerServePort())
	return st
}

// mcpEmailServerServeProxies returns the proxy targets `tailscale serve` already
// binds on the given tailnet port.
func mcpEmailServerServeProxies(ctx context.Context, port int) []string {
	if !sysutil.CommandExists("tailscale") {
		return nil
	}
	out, err := captureOutput(ctx, "tailscale", "serve", "status", "--json")
	if err != nil {
		return nil
	}
	// A node with no serve config prints a bare message, not JSON.
	var cfg struct {
		Web map[string]struct {
			Handlers map[string]struct{ Proxy string }
		}
	}
	if json.Unmarshal([]byte(out), &cfg) != nil {
		return nil
	}
	var proxies []string
	for hostPort, web := range cfg.Web {
		if !strings.HasSuffix(hostPort, fmt.Sprintf(":%d", port)) {
			continue
		}
		for _, h := range web.Handlers {
			if h.Proxy != "" {
				proxies = append(proxies, h.Proxy)
			}
		}
	}
	sort.Strings(proxies)
	return proxies
}

// mcpEmailServerConflictError explains what was found and what replacing it
// would cost, rather than leaving the operator to discover it from an empty
// inbox.
func mcpEmailServerConflictError(conflicts []string) error {
	var b strings.Builder
	b.WriteString("something is already serving this role on this machine:\n")
	for _, c := range conflicts {
		fmt.Fprintf(&b, "  - %s\n", c)
	}
	b.WriteString("Installing would replace it. Its mailbox accounts live in a different store and\n")
	b.WriteString("would NOT carry over — every account would need adding again, with its password.\n")
	b.WriteString("Re-run with --force to replace it anyway. ./config is never deleted either way.")
	return errors.New(b.String())
}

// mcpEmailServerEnsureConfigDir creates the credential directory owner-only on
// first install and leaves an existing one completely alone — mode included.
// Re-running install must never disturb the catalog.
func mcpEmailServerEnsureConfigDir() error {
	if _, err := os.Stat(MCPEmailServerConfigDir()); err == nil {
		return nil
	}
	// Upstream refuses to open a catalog whose parent is group- or
	// world-accessible, and Docker would otherwise create the bind-mount source
	// itself, world-readable.
	return os.MkdirAll(MCPEmailServerConfigDir(), 0o700)
}

func mcpEmailServerInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)

	// Probed before anything is deployed, and reported inside the dry-run
	// preview so `--dry-run` shows that the install would be refused.
	conflicts := mcpEmailServerConflicts(mcpEmailServerInspectState(ctx))

	if done, err := mcpEmailServerStack.preflight(o); done || err != nil {
		if done && len(conflicts) > 0 {
			fmt.Fprintf(opts.Stdout, "[dry-run] would REFUSE: %v\n", mcpEmailServerConflictError(conflicts))
		}
		return err
	}
	if len(conflicts) > 0 {
		if !opts.Force {
			return mcpEmailServerConflictError(conflicts)
		}
		fmt.Fprintf(opts.Stdout, "--force: replacing existing state (%s). ./config kept — move it aside yourself if the new server can't read it.\n",
			strings.Join(conflicts, "; "))
		// A container we didn't create still owns the name, so compose can't
		// take it. Remove the container only — never its volumes, which may be
		// the other server's credential store.
		if img := sysutil.ContainerConfigImage(ctx, mcpEmailServerService); img != "" && img != mcpEmailServerOwnImage {
			fmt.Fprintf(opts.Stdout, "  removing the %s container built from %s (its volumes are left alone)\n", mcpEmailServerService, img)
			if err := sysutil.Run(ctx, o, "docker", "rm", "-f", mcpEmailServerService); err != nil {
				return fmt.Errorf("remove the existing %s container: %w", mcpEmailServerService, err)
			}
		}
	}

	if err := mcpEmailServerStack.deploy(); err != nil {
		return err
	}
	if err := mcpEmailServerEnsureConfigDir(); err != nil {
		return err
	}
	// The container runs as this user so the catalog is owner-only from its
	// point of view too; compose fails loudly if these are missing.
	if err := MCPEmailServerSetEnv(map[string]string{
		"MCP_UID": strconv.Itoa(os.Getuid()),
		"MCP_GID": strconv.Itoa(os.Getgid()),
	}); err != nil {
		return fmt.Errorf("write %s: %w", MCPEmailServerEnvPath(), err)
	}

	if err := MCPEmailServerComposeUp(ctx, o); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	if err := MCPEmailServerInitCatalog(ctx); err != nil {
		// Almost always leftover state from another server in the same
		// directory — which install deliberately kept rather than deleting.
		return fmt.Errorf("initialize the credential catalog: %w\n  check %s: if it holds another email server's files, move the directory aside and re-run", err, MCPEmailServerConfigDir())
	}

	fmt.Fprintf(opts.Stdout, "mcp-email-server up on 127.0.0.1:%d (loopback only — nothing on the LAN can reach it).\n", MCPEmailServerPort)
	if mode := MCPEmailServerCatalogMode(); mode != 0 {
		fmt.Fprintf(opts.Stdout, "  Mailbox credentials: %s (mode %04o)\n", MCPEmailServerCatalogPath(), mode)
	}
	// Said out loud on every install: an empty catalog is the exact state that
	// reads as "no mail today" to a client rather than "nothing is configured".
	if accounts, err := MCPEmailServerAccounts(ctx); err == nil {
		fmt.Fprintf(opts.Stdout, "  Mailboxes configured: %d\n", len(accounts))
	}
	fmt.Fprintln(opts.Stdout, "  Add mailboxes and publish it to the tailnet: ctdev configure mcp-email-server")
	return nil
}

func mcpEmailServerUninstall(ctx context.Context, opts ExecOpts) error {
	mcpEmailServerStack.down(ctx, opts,
		"mcp-email-server stopped. ~/mcp-email-server/ kept (config/managed.sqlite3 holds every mailbox password — deleting it means re-entering all of them).", false)
	fmt.Fprintf(opts.Stdout, "  Stop publishing it to the tailnet with: sudo tailscale serve --https=%d off\n", MCPEmailServerServePort())
	return nil
}
