package diagnose

import (
	"net/netip"
	"testing"
)

func TestParseIPRouteDefault(t *testing.T) {
	tests := map[string]string{
		"default via 10.2.2.1 dev wlp5s0 proto dhcp src 10.2.2.200 metric 600 ": "10.2.2.1",
		"default via 192.168.1.254 dev eth0":                                    "192.168.1.254",
		// Two default routes: the kernel lists them in preference order, so
		// the first is the one traffic actually takes.
		"default via 10.0.0.1 dev tun0 metric 50\ndefault via 192.168.1.1 dev wlan0 metric 600": "10.0.0.1",
		// A directly-attached default route has no "via" at all.
		"default dev ppp0 scope link": "",
		"":                            "",
	}
	for out, want := range tests {
		got := parseIPRouteDefault(out)
		if want == "" {
			if got.IsValid() {
				t.Errorf("parseIPRouteDefault(%q) = %v, want invalid", out, got)
			}
			continue
		}
		if got.String() != want {
			t.Errorf("parseIPRouteDefault(%q) = %v, want %s", out, got, want)
		}
	}
}

func TestParseResolvectlLinks(t *testing.T) {
	// Real `resolvectl dns` output from a machine running Tailscale and Docker.
	const out = `Global:
Link 2 (enp7s0):
Link 3 (wlp5s0): 10.2.2.144
Link 4 (tailscale0): 100.100.100.100 fd7a:115c:a1e0::53
Link 5 (br-bc39a9c47468):
Link 6 (docker0):`

	links := parseResolvectlLinks(out)
	if got := len(links); got != 2 {
		t.Fatalf("expected 2 links with resolvers, got %d: %v", got, links)
	}

	// The egress link's resolver is the one queries actually reach.
	if got := resolversForLink(links, "wlp5s0"); len(got) != 1 || got[0].String() != "10.2.2.144" {
		t.Errorf("resolversForLink(wlp5s0) = %v, want [10.2.2.144]", got)
	}

	// Tailscale's MagicDNS serves tailscale0 only. Reporting it as a system
	// resolver would send a diagnosis chasing the wrong server.
	if got := resolversForLink(links, "tailscale0"); len(got) != 2 {
		t.Errorf("resolversForLink(tailscale0) = %v, want both MagicDNS addresses", got)
	}

	// An interface with no resolvers of its own falls back to Global, which is
	// empty here — so the caller moves on to resolv.conf.
	if got := resolversForLink(links, "enp7s0"); len(got) != 0 {
		t.Errorf("resolversForLink(enp7s0) = %v, want none", got)
	}
}

func TestParseResolvConf(t *testing.T) {
	const out = `# This is /run/systemd/resolve/stub-resolv.conf managed by systemd-resolved(8).
# Do not edit.
nameserver 127.0.0.53
options edns0 trust-ad
search lan
; a comment in the other style
nameserver 1.1.1.1`

	got := parseResolvConf(out)
	if len(got) != 2 || got[0].String() != "127.0.0.53" || got[1].String() != "1.1.1.1" {
		t.Errorf("parseResolvConf = %v, want [127.0.0.53 1.1.1.1]", got)
	}
}

func TestParseRouteGetDefault(t *testing.T) {
	const out = `   route to: default
destination: default
       mask: default
    gateway: 192.168.1.1
  interface: en0
      flags: <UP,GATEWAY,DONE,STATIC,PRCLONING,GLOBAL>`

	if got := parseRouteGetDefault(out); got.String() != "192.168.1.1" {
		t.Errorf("parseRouteGetDefault = %v, want 192.168.1.1", got)
	}
	if got := parseRouteGetDefault("route: writing to routing socket: not in table"); got.IsValid() {
		t.Errorf("parseRouteGetDefault on failure = %v, want invalid", got)
	}
}

func TestParseScutilDNS(t *testing.T) {
	// Only the first resolver block is the system default; later ones are
	// scoped to a domain or interface and would be misleading here.
	const out = `DNS configuration

resolver #1
  search domain[0] : lan
  nameserver[0] : 192.168.1.1
  nameserver[1] : 1.1.1.1
  flags    : Request A records
  reach    : 0x00020002 (Reachable,Directly Reachable Address)

resolver #2
  domain   : local
  nameserver[0] : 224.0.0.251
  flags    : Request A records`

	got := parseScutilDNS(out)
	if len(got) != 2 || got[0].String() != "192.168.1.1" || got[1].String() != "1.1.1.1" {
		t.Errorf("parseScutilDNS = %v, want [192.168.1.1 1.1.1.1]", got)
	}
}

func TestParseHardwarePortDevice(t *testing.T) {
	const out = `Hardware Port: Ethernet Adapter (en4)
Device: en4
Ethernet Address: aa:bb:cc:dd:ee:ff

Hardware Port: Wi-Fi
Device: en0
Ethernet Address: 11:22:33:44:55:66

Hardware Port: Thunderbolt Bridge
Device: bridge0
Ethernet Address: N/A`

	if got := parseHardwarePortDevice(out, "Wi-Fi"); got != "en0" {
		t.Errorf("parseHardwarePortDevice(Wi-Fi) = %q, want en0", got)
	}
	if got := parseHardwarePortDevice(out, "Bluetooth PAN"); got != "" {
		t.Errorf("parseHardwarePortDevice(absent port) = %q, want empty", got)
	}
}

func TestParseAddrListDedupes(t *testing.T) {
	got := parseAddrList([]string{"1.1.1.1", "1.1.1.1,", "not-an-address", "8.8.8.8"})
	if len(got) != 2 {
		t.Fatalf("parseAddrList = %v, want 2 unique addresses", got)
	}
	if got[0].String() != "1.1.1.1" || got[1].String() != "8.8.8.8" {
		t.Errorf("parseAddrList = %v, want [1.1.1.1 8.8.8.8]", got)
	}
}

func TestIsPrivate(t *testing.T) {
	tests := map[string]bool{
		"192.168.1.1": true,
		"10.2.2.200":  true,
		"172.16.0.1":  true,
		"127.0.0.1":   true,
		"169.254.9.1": true,
		// Carrier-grade NAT is exactly what an ISP-supplied router hands out,
		// and netip's IsPrivate does not cover it.
		"100.64.0.1":    true,
		"100.127.255.1": true,
		"8.8.8.8":       false,
		"1.1.1.1":       false,
		// 100.128.x is outside 100.64/10 and is ordinary public space.
		"100.128.0.1": false,
	}
	for in, want := range tests {
		if got := IsPrivate(netip.MustParseAddr(in)); got != want {
			t.Errorf("IsPrivate(%s) = %v, want %v", in, got, want)
		}
	}
	if IsPrivate(netip.Addr{}) {
		t.Error("IsPrivate(invalid) should be false")
	}
}

func TestLinkLocalIPv4(t *testing.T) {
	if !LinkLocalIPv4(netip.MustParseAddr("169.254.9.12")) {
		t.Error("169.254.9.12 is APIPA")
	}
	if LinkLocalIPv4(netip.MustParseAddr("192.168.1.5")) {
		t.Error("192.168.1.5 is not APIPA")
	}
	// IPv6 link-local is normal and always present; it must not be mistaken
	// for the IPv4 "DHCP failed" signal.
	if LinkLocalIPv4(netip.MustParseAddr("fe80::1")) {
		t.Error("fe80::1 is IPv6 link-local, not APIPA")
	}
}
