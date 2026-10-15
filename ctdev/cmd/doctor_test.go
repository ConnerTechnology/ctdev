package cmd

import (
	"context"
	"net/netip"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/diagnose"
	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

// --strict fails the run on failures only; an sshd that is off is reported as
// information and must not trip it.
func TestStrictIgnoresInfoRows(t *testing.T) {
	results := map[string]diagnose.Result{
		"service.ssh":    {Severity: diagnose.Info, Detail: "off"},
		"service.docker": {Severity: diagnose.OK, Detail: "running"},
	}
	if n := countFailures(results); n != 0 {
		t.Errorf("countFailures = %d, want 0", n)
	}
	results["service.tailscaled"] = diagnose.Result{Severity: diagnose.Fail, Detail: "tailscaled is inactive"}
	if n := countFailures(results); n != 1 {
		t.Errorf("countFailures = %d, want 1 for a stopped tailscaled", n)
	}
}

// doctor is the one caller that knows what ctdev installed; the facts it hands
// the catalog have to carry that, on top of what GatherFacts found.
func TestDoctorFactsCarryInstalledComponents(t *testing.T) {
	gather := func(context.Context, platform.Info) diagnose.Facts {
		return diagnose.Facts{Hostname: "test-host", SSHUnitInstalled: true}
	}
	installed := func() map[string]bool { return map[string]bool{"git": true, "docker": true} }

	f := gatherDoctorFacts(context.Background(), platform.Info{OS: platform.Linux}, gather, installed)

	if !f.Components["git"] || !f.Components["docker"] || len(f.Components) != 2 {
		t.Errorf("Components = %v, want git and docker", f.Components)
	}
	if f.Hostname != "test-host" || !f.SSHUnitInstalled {
		t.Errorf("gathered facts were lost: %+v", f)
	}
}

// The set-up instructions promise that setting CTDEV_UNIFI_API_KEY is enough.
// Requiring the endpoint separately made that promise false, and left the tool
// refusing to talk to a controller it had just finished identifying.
func TestUnifiEndpointDefaultsToGateway(t *testing.T) {
	t.Setenv("CTDEV_UNIFI_API_KEY", "test-key")
	flagDoctorUnifi = ""
	flagDoctorBatch()

	facts := diagnose.Facts{Gateway: netip.MustParseAddr("10.2.2.1")}
	creds := integrationCreds(context.Background(), facts)

	if got := creds.UniFi.Endpoint; got != "https://10.2.2.1" {
		t.Errorf("endpoint = %q, want it defaulted to the gateway", got)
	}
	if !creds.Any() {
		t.Error("a key plus a known gateway should be usable credentials")
	}
}

// An explicit endpoint always wins: the controller is not always the gateway.
func TestUnifiExplicitEndpointWins(t *testing.T) {
	t.Setenv("CTDEV_UNIFI_API_KEY", "test-key")
	t.Setenv("CTDEV_UNIFI_HOST", "https://10.9.9.9:8443")
	flagDoctorUnifi = ""
	flagDoctorBatch()

	creds := integrationCreds(context.Background(), diagnose.Facts{
		Gateway: netip.MustParseAddr("10.2.2.1"),
	})
	if got := creds.UniFi.Endpoint; got != "https://10.9.9.9:8443" {
		t.Errorf("endpoint = %q, want the explicitly configured controller", got)
	}
}

// Without a key there is nothing to point anywhere, and no probe should run.
func TestNoKeyMeansNoIntegration(t *testing.T) {
	t.Setenv("CTDEV_UNIFI_API_KEY", "")
	t.Setenv("CTDEV_UNIFI_HOST", "")
	flagDoctorUnifi = ""
	flagDoctorBatch()

	creds := integrationCreds(context.Background(), diagnose.Facts{
		Gateway: netip.MustParseAddr("10.2.2.1"),
	})
	if creds.Any() {
		t.Errorf("no credentials should mean no integration, got %+v", creds)
	}
}

// flagDoctorBatch keeps the one-shot prompt from blocking a test run.
func flagDoctorBatch() { flagBatch = true }

// Doctor is built to diagnose machines we do not manage, so it must never ask a
// stranger for their password on its own initiative. Root is opt-in, and the
// default is the half of that contract worth pinning.
func TestDoctorRootFlagDefaultsOff(t *testing.T) {
	f := doctorCmd.Flags().Lookup("root")
	if f == nil {
		t.Fatal("doctor has no --root flag")
	}
	if f.DefValue != "false" {
		t.Errorf("--root default = %q, want %q", f.DefValue, "false")
	}
}
