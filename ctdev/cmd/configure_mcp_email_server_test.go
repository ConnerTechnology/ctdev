package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
)

func TestConfigureMCPEmailServerIsRegistered(t *testing.T) {
	configure := childCommand(t, rootCmd, "configure")
	if !hasSubcommand(configure, "mcp-email-server") {
		t.Error("configure should have an mcp-email-server subcommand")
	}
}

// `ctdev install mcp-email-server` has to chain into the wizard: the stack comes
// up with no mailboxes and no tailnet endpoint until it runs.
func TestInstallChainsIntoMCPEmailServerWizard(t *testing.T) {
	if !componentHasConfigure("mcp-email-server") {
		t.Error("install should run the mcp-email-server configuration step")
	}
	if componentWizards["mcp-email-server"] == nil {
		t.Error("mcp-email-server should resolve to its dedicated wizard")
	}
}

// Nothing about a mailbox password can be defaulted, so a non-interactive run
// must refuse — before it starts a container to find that out.
func TestConfigureMCPEmailServerRejectsBatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(component.MCPEmailServerDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(component.MCPEmailServerDir(), "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	flagBatch = true
	t.Cleanup(func() { flagBatch = false })

	err := configureMCPEmailServer(context.Background())
	if err == nil || !strings.Contains(err.Error(), "interactively") {
		t.Errorf("expected an interactive-only error, got %v", err)
	}
}

// An undeployed stack has no container to talk to; say so instead of failing
// inside docker compose.
func TestConfigureMCPEmailServerNeedsInstallFirst(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	err := configureMCPEmailServer(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ctdev install mcp-email-server") {
		t.Errorf("expected a not-deployed error, got %v", err)
	}
}
