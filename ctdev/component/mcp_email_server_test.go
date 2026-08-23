package component

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func mcpEmailServerCompose(t *testing.T) string {
	t.Helper()
	b, err := Configs.ReadFile("configs/mcp-email-server/docker-compose.yml")
	if err != nil {
		t.Fatalf("read embedded compose file: %v", err)
	}
	return string(b)
}

// The server has no authentication of its own: anything that can reach the
// published port reads every configured mailbox. Only a loopback mapping keeps
// that off the LAN — Docker's iptables rules sit ahead of UFW, so a bare
// "9557:9557" would be network-wide even on a firewalled host.
func TestMCPEmailServerPublishesOnLoopbackOnly(t *testing.T) {
	ports := regexp.MustCompile(`(?m)^\s+- "([^"]+)"`).FindAllStringSubmatch(mcpEmailServerCompose(t), -1)
	if len(ports) == 0 {
		t.Fatal("no port mappings found — the regexp or the compose file changed shape")
	}
	for _, m := range ports {
		if !strings.HasPrefix(m[1], "127.0.0.1:") {
			t.Errorf("port mapping %q is not bound to 127.0.0.1", m[1])
		}
	}
}

// A floating version would let the process holding mailbox passwords upgrade
// itself on any rebuild.
func TestMCPEmailServerPackageIsPinned(t *testing.T) {
	b, err := Configs.ReadFile("configs/mcp-email-server/Dockerfile")
	if err != nil {
		t.Fatalf("read embedded Dockerfile: %v", err)
	}
	pin := regexp.MustCompile(`mcp-email-server==(\d+\.\d+\.\d+)`)
	if !pin.Match(b) {
		t.Error("Dockerfile must pin mcp-email-server to an exact version")
	}
	if regexp.MustCompile(`(?m)^FROM\s+\S+:latest`).Match(b) {
		t.Error("base image must not track :latest")
	}
}

// Every file the installer deploys has to exist in the embedded configs, or the
// stack only breaks on a real node.
func TestMCPEmailServerStackFilesAreEmbedded(t *testing.T) {
	for _, f := range mcpEmailServerStack.Files {
		if _, err := Configs.ReadFile("configs/mcp-email-server/" + f[0]); err != nil {
			t.Errorf("stack file %q is not embedded: %v", f[0], err)
		}
	}
}

// `tailscale serve` forwards the client's original Host header, and the MCP
// transport rejects any Host it wasn't told about with a 421 — so omitting the
// node's MagicDNS name makes every tailnet request fail.
func TestMCPEmailServerWriteEnvAllowsTailnetHost(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := MCPEmailServerWriteTailnetEnv("ctpi01.tailnet.ts.net", 8443); err != nil {
		t.Fatalf("write env: %v", err)
	}

	env := MCPEmailServerReadEnv()
	hosts := strings.Split(env["MCP_ALLOWED_HOSTS"], ",")
	for _, want := range []string{"127.0.0.1", "ctpi01.tailnet.ts.net"} {
		if !slices.Contains(hosts, want) {
			t.Errorf("MCP_ALLOWED_HOSTS = %q, want it to include %q", env["MCP_ALLOWED_HOSTS"], want)
		}
	}
	if !strings.Contains(env["MCP_ALLOWED_ORIGINS"], "https://ctpi01.tailnet.ts.net") {
		t.Errorf("MCP_ALLOWED_ORIGINS = %q, want the tailnet https origin", env["MCP_ALLOWED_ORIGINS"])
	}
}

// Before Tailscale is up there is no MagicDNS name; the file must still be
// written, and must stay loopback-only rather than growing an empty entry that
// would read as a wildcard.
func TestMCPEmailServerWriteEnvWithoutTailscaleStaysLoopback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := MCPEmailServerWriteTailnetEnv("", 443); err != nil {
		t.Fatalf("write env: %v", err)
	}
	for _, host := range strings.Split(MCPEmailServerReadEnv()["MCP_ALLOWED_HOSTS"], ",") {
		switch host {
		case "127.0.0.1", "localhost", "[::1]":
		default:
			t.Errorf("unexpected allowed host %q with Tailscale down", host)
		}
	}
}

// Uninstall stops the stack but keeps the credentials: losing config.toml means
// re-issuing an app-specific password for every mailbox.
func TestMCPEmailServerUninstallKeepsCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(MCPEmailServerConfigDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	catalog := MCPEmailServerCatalogPath()
	if err := os.WriteFile(catalog, []byte("credentials"), 0o600); err != nil {
		t.Fatal(err)
	}

	// No docker-compose.yml in this fake home, so down() skips docker entirely
	// and the test needs no daemon.
	var out bytes.Buffer
	if err := mcpEmailServerUninstall(context.Background(), ExecOpts{Stdout: &out}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(catalog); err != nil {
		t.Errorf("uninstall removed %s: %v", catalog, err)
	}
	if !strings.Contains(out.String(), filepath.Base(MCPEmailServerDir())) {
		t.Errorf("uninstall should say what it kept, got:\n%s", out.String())
	}
}

func TestMCPEmailServerRegistryEntry(t *testing.T) {
	c := FindByName("mcp-email-server")
	if c == nil {
		t.Fatal("mcp-email-server missing from the registry")
	}
	if !c.SupportsOS(OSLinux) || c.SupportsOS(OSMacOS) {
		t.Errorf("expected a Linux-only component, got SupportedOS %v", c.SupportedOS)
	}
	if !slices.Contains(c.Dependencies, "docker") {
		t.Errorf("expected a docker dependency, got %v", c.Dependencies)
	}
	// Install and uninstall stay inside $HOME and the docker socket; only
	// `ctdev configure mcp-email-server` needs root, for `tailscale serve`.
	if c.Root != RootNever {
		t.Errorf("Root = %v, want RootNever", c.Root)
	}
	t.Setenv("HOME", t.TempDir())
	if got := os.ExpandEnv(c.DetectPath); got != mcpEmailServerStack.composePath() {
		t.Errorf("DetectPath expands to %q, want %q", got, mcpEmailServerStack.composePath())
	}
	for _, tag := range []string{"homelab", "ai"} {
		if !slices.Contains(c.Tags, tag) {
			t.Errorf("expected tag %q, got %v", tag, c.Tags)
		}
	}
}

// 443 on a caddy node is already taken: `ctdev configure caddy` points
// *.<domain> at this node's tailnet address, and a serve rule on 443 would
// intercept it — every homelab site on the tailnet would hit the email server.
func TestMCPEmailServerServePortAvoidsCaddy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got := MCPEmailServerServePort(); got != 443 {
		t.Errorf("serve port on a node without caddy = %d, want 443", got)
	}

	// Make the caddy component read as installed (its DetectPath is its compose file).
	caddy := FindByName("caddy")
	if caddy == nil {
		t.Fatal("caddy missing from the registry")
	}
	compose := os.ExpandEnv(caddy.DetectPath)
	if err := os.MkdirAll(filepath.Dir(compose), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(compose, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := MCPEmailServerServePort(); got != 8443 {
		t.Errorf("serve port on a caddy node = %d, want 8443", got)
	}

	// An explicit choice already on disk wins over the guess.
	if err := MCPEmailServerWriteTailnetEnv("node.ts.net", 9443); err != nil {
		t.Fatal(err)
	}
	if got := MCPEmailServerServePort(); got != 9443 {
		t.Errorf("serve port with MCP_SERVE_PORT set = %d, want 9443", got)
	}
}

func TestMCPEmailServerURLOmitsDefaultPort(t *testing.T) {
	if got := MCPEmailServerURL("node.ts.net", 443); got != "https://node.ts.net/mcp" {
		t.Errorf("URL on 443 = %q", got)
	}
	if got := MCPEmailServerURL("node.ts.net", 8443); got != "https://node.ts.net:8443/mcp" {
		t.Errorf("URL on 8443 = %q", got)
	}
}

// 1.x refuses to open its catalog unless the parent directory is owner-only
// *from the running user's* point of view, so a root-run container against a
// user-owned ./config fails. The compose file must therefore take the uid/gid
// from .env, and must fail loudly rather than defaulting to root.
func TestMCPEmailServerRunsAsInvokingUser(t *testing.T) {
	compose := mcpEmailServerCompose(t)
	if !strings.Contains(compose, "${MCP_UID:?") || !strings.Contains(compose, "${MCP_GID:?") {
		t.Error("compose must require MCP_UID/MCP_GID via the :? form, not default to root")
	}
}

// Install has to write the uid/gid before the first compose up, or the stack
// won't start at all.
func TestMCPEmailServerSetEnvMergesKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := MCPEmailServerSetEnv(map[string]string{"MCP_UID": "1000", "MCP_GID": "1000"}); err != nil {
		t.Fatal(err)
	}
	if err := MCPEmailServerWriteTailnetEnv("node.ts.net", 8443); err != nil {
		t.Fatal(err)
	}
	env := MCPEmailServerReadEnv()
	// configure must not clobber what install wrote, or the stack stops booting.
	for k, want := range map[string]string{"MCP_UID": "1000", "MCP_GID": "1000", "MCP_SERVE_PORT": "8443"} {
		if env[k] != want {
			t.Errorf("%s = %q, want %q", k, env[k], want)
		}
	}
}

// The guard that exists because install once silently replaced a different
// email server and left its accounts orphaned behind an empty, non-error
// list_available_accounts.
func TestMCPEmailServerConflicts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ours := fmt.Sprintf("http://127.0.0.1:%d", MCPEmailServerPort)

	cases := []struct {
		name  string
		state mcpEmailServerState
		want  bool
	}{
		{"fresh machine", mcpEmailServerState{}, false},
		{"our own stack, re-run", mcpEmailServerState{
			ContainerImage: mcpEmailServerOwnImage,
			ConfigEntries:  []string{"config.bootstrap.toml", "managed.sqlite3"},
			ServeProxies:   []string{ours},
		}, false},
		{"empty config dir before first init", mcpEmailServerState{
			ContainerImage: mcpEmailServerOwnImage,
		}, false},
		{"a container we did not build", mcpEmailServerState{
			ContainerImage: "ghcr.io/ai-zerolab/mcp-email-server:0.16.0",
		}, true},
		{"another server's config", mcpEmailServerState{
			ConfigEntries: []string{"config.toml"},
		}, true},
		{"serve port already taken", mcpEmailServerState{
			ServeProxies: []string{"http://127.0.0.1:9999"},
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mcpEmailServerConflicts(tc.state)
			if (len(got) > 0) != tc.want {
				t.Errorf("conflicts = %v, want conflict=%v", got, tc.want)
			}
		})
	}
}

// Re-running install over our own stack must leave the credential directory
// exactly as it was — contents and mode. Losing it means re-issuing an
// app-specific password for every mailbox.
func TestMCPEmailServerEnsureConfigDirNeverTouchesExisting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// First install: creates it owner-only.
	if err := mcpEmailServerEnsureConfigDir(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(MCPEmailServerConfigDir())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("fresh config dir mode = %04o, want 0700", fi.Mode().Perm())
	}

	catalog := MCPEmailServerCatalogPath()
	if err := os.WriteFile(catalog, []byte("mailbox credentials"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Every later install re-runs this; nothing about ./config may change.
	for i := 0; i < 3; i++ {
		if err := mcpEmailServerEnsureConfigDir(); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(catalog)
	if err != nil {
		t.Fatalf("catalog gone after re-install: %v", err)
	}
	if string(b) != "mailbox credentials" {
		t.Errorf("catalog rewritten: %q", b)
	}
	if fi, err := os.Stat(catalog); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("catalog mode changed: %v %v", fi.Mode().Perm(), err)
	}
}

// The policy is revision-guarded, so a change has to quote the revision it saw.
// Getting this wrong fails as a silent no-op, which is how the setting looked
// enabled while every download was still refused.
func TestMCPEmailServerPolicyArgs(t *testing.T) {
	policy := map[string]any{"revision": float64(9), "enable_attachment_download": false}

	args, err := mcpEmailServerPolicyArgs(policy, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"config", "update-policy", "--expected-revision", "9", "--enable-attachment-download", "--json"}
	if !slices.Equal(args, want) {
		t.Errorf("args = %v, want %v", args, want)
	}

	if args, err := mcpEmailServerPolicyArgs(policy, false); err != nil || args != nil {
		t.Errorf("no change needed should return nil args, got %v (%v)", args, err)
	}

	on := map[string]any{"revision": float64(9), "enable_attachment_download": true}
	args, err = mcpEmailServerPolicyArgs(on, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(args, "--disable-attachment-download") {
		t.Errorf("turning it off should disable, got %v", args)
	}

	// A policy document with no revision cannot be updated safely — refuse
	// rather than guessing a revision the server will reject.
	if _, err := mcpEmailServerPolicyArgs(map[string]any{}, true); err == nil {
		t.Error("expected an error when the policy has no revision")
	}
}
