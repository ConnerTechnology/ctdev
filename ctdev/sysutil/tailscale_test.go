package sysutil

import "testing"

func TestParseTailscaleStatus(t *testing.T) {
	// The shape `tailscale status --json` prints: the node's own DNS name has
	// a trailing dot the callers never want, and the suffix sits at top level.
	raw := []byte(`{
	  "BackendState": "Running",
	  "Self": {"DNSName": "pi-01.tailnet-example.ts.net.", "HostName": "pi-01"},
	  "MagicDNSSuffix": "tailnet-example.ts.net",
	  "Peer": {}
	}`)
	got := ParseTailscaleStatus(raw)
	if got.DNSName != "pi-01.tailnet-example.ts.net" {
		t.Errorf("DNSName = %q", got.DNSName)
	}
	if got.MagicDNSSuffix != "tailnet-example.ts.net" {
		t.Errorf("MagicDNSSuffix = %q", got.MagicDNSSuffix)
	}
}

func TestParseTailscaleStatusGarbage(t *testing.T) {
	// A daemon that isn't running prints nothing useful; the zero value is
	// the honest answer, never a panic.
	if got := ParseTailscaleStatus([]byte("not json")); got != (TailscaleStatus{}) {
		t.Errorf("garbage parsed to %+v", got)
	}
}
