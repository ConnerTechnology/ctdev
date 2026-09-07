package sysutil

import "testing"

func TestParseTailscaleStatus(t *testing.T) {
	// The shape `tailscale status --json` prints: the node's own DNS name has
	// a trailing dot the callers never want, and the suffix sits at top level.
	raw := []byte(`{
	  "BackendState": "Running",
	  "Self": {"DNSName": "ctpi01.tail3c73d9.ts.net.", "HostName": "ctpi01"},
	  "MagicDNSSuffix": "tail3c73d9.ts.net",
	  "Peer": {}
	}`)
	got := ParseTailscaleStatus(raw)
	if got.DNSName != "ctpi01.tail3c73d9.ts.net" {
		t.Errorf("DNSName = %q", got.DNSName)
	}
	if got.MagicDNSSuffix != "tail3c73d9.ts.net" {
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
