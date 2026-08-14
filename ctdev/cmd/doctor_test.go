package cmd

import (
	"context"
	"net/netip"
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/diagnose"
)

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
