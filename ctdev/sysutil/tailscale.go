package sysutil

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
)

// TailscaleStatus is the slice of `tailscale status --json` ctdev reads: the
// node's own MagicDNS name and the tailnet's suffix. Both come back "" when
// Tailscale is absent, not running, or has MagicDNS off.
type TailscaleStatus struct {
	DNSName        string // e.g. ctpi01.tail3c73d9.ts.net (no trailing dot)
	MagicDNSSuffix string // e.g. tail3c73d9.ts.net
}

// ParseTailscaleStatus extracts the DNS facts from the daemon's JSON status.
// Garbage yields the zero value — a daemon that isn't up prints nothing useful.
func ParseTailscaleStatus(raw []byte) TailscaleStatus {
	var st struct {
		Self           struct{ DNSName string }
		MagicDNSSuffix string
	}
	if json.Unmarshal(raw, &st) != nil {
		return TailscaleStatus{}
	}
	return TailscaleStatus{
		DNSName:        strings.TrimSuffix(st.Self.DNSName, "."),
		MagicDNSSuffix: strings.TrimSuffix(st.MagicDNSSuffix, "."),
	}
}

// TailscaleDNS asks the running daemon for this node's DNS facts.
func TailscaleDNS(ctx context.Context) TailscaleStatus {
	if !CommandExists("tailscale") {
		return TailscaleStatus{}
	}
	out, err := exec.CommandContext(ctx, "tailscale", "status", "--json").Output()
	if err != nil {
		return TailscaleStatus{}
	}
	return ParseTailscaleStatus(out)
}
