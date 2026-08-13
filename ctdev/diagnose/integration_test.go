package diagnose

import (
	"context"
	"encoding/binary"
	"net/netip"
	"strings"
	"testing"
)

func TestLookupOUI(t *testing.T) {
	tests := map[string]string{
		// Spelling shouldn't matter: tools print MACs three different ways.
		"78:8a:20:1a:2b:3c": "Ubiquiti",
		"78-8A-20-1A-2B-3C": "Ubiquiti",
		"F4:92:BF:11:22:33": "Ubiquiti",
		"00:11:32:aa:bb:cc": "Synology",
		"f8:bb:bf:00:00:01": "eero",
		// An unregistered or unknown prefix is a normal answer, not an error.
		"12:34:56:78:9a:bc": "",
		"":                  "",
		"not-a-mac":         "",
		"78:8a":             "",
	}
	for mac, want := range tests {
		if got := LookupOUI(mac); got != want {
			t.Errorf("LookupOUI(%q) = %q, want %q", mac, got, want)
		}
	}
}

// Multi-SSID access points mint a virtual BSSID per network with the
// locally-administered bit set, so a BSSID frequently carries no vendor
// prefix. That's why the gateway MAC is the thing worth fingerprinting.
func TestLocallyAdministered(t *testing.T) {
	// Observed on a real UniFi network: the BSSID is locally administered
	// even though the hardware is Ubiquiti.
	if !LocallyAdministered("22:0b:8b:c2:7d:db") {
		t.Error("22:0b:... has the locally-administered bit set")
	}
	if LocallyAdministered("78:8a:20:1a:2b:3c") {
		t.Error("78:8a:... is a globally registered prefix")
	}
	if LookupOUI("22:0b:8b:c2:7d:db") != "" {
		t.Error("a locally-administered MAC should not match a vendor prefix")
	}
}

func TestNormalizeMAC(t *testing.T) {
	tests := map[string]string{
		"78:8A:20:1A:2B:3C": "78:8a:20:1a:2b:3c",
		// BSD arp prints octets unpadded, which would miss every OUI lookup.
		"0:1a:2b:3:4:5":        "00:1a:2b:03:04:05",
		"78-8a-20-1a-2b-3c":    "78:8a:20:1a:2b:3c",
		"(incomplete)":         "",
		"":                     "",
		"78:8a:20:1a:2b":       "",
		"78:8a:20:1a:2b:3c:4d": "",
	}
	for in, want := range tests {
		if got := normalizeMAC(in); got != want {
			t.Errorf("normalizeMAC(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseIPNeigh(t *testing.T) {
	const out = "10.2.2.1 dev wlp5s0 lladdr 22:0b:8b:c2:7d:db REACHABLE"
	if got := parseIPNeigh(out); got != "22:0b:8b:c2:7d:db" {
		t.Errorf("parseIPNeigh = %q", got)
	}
	// A neighbour entry with no hardware address yet.
	if got := parseIPNeigh("10.2.2.1 dev wlp5s0  FAILED"); got != "" {
		t.Errorf("parseIPNeigh on an incomplete entry = %q, want empty", got)
	}
}

func TestParseArpOutput(t *testing.T) {
	const out = "? (192.168.1.1) at 78:8a:20:1a:2b:3c on en0 ifscope [ethernet]"
	if got := parseArpOutput(out); got != "78:8a:20:1a:2b:3c" {
		t.Errorf("parseArpOutput = %q", got)
	}
	if got := parseArpOutput("? (192.168.1.9) at (incomplete) on en0"); got != "" {
		t.Errorf("parseArpOutput on incomplete = %q, want empty", got)
	}
}

// buildDiscoveryReply assembles a well-formed response for the parser tests.
func buildDiscoveryReply(tlvs map[byte][]byte) []byte {
	var body []byte
	// Deterministic order so the test doesn't depend on map iteration.
	for _, tag := range []byte{0x01, 0x03, 0x0b, 0x14} {
		value, present := tlvs[tag]
		if !present {
			continue
		}
		header := []byte{tag, 0, 0}
		binary.BigEndian.PutUint16(header[1:3], uint16(len(value)))
		body = append(body, header...)
		body = append(body, value...)
	}
	out := []byte{0x01, 0x00, 0, 0}
	binary.BigEndian.PutUint16(out[2:4], uint16(len(body)))
	return append(out, body...)
}

func TestParseUbiquitiDiscovery(t *testing.T) {
	reply := buildDiscoveryReply(map[byte][]byte{
		0x01: {0x78, 0x8a, 0x20, 0x1a, 0x2b, 0x3c},
		0x03: []byte("UDMPRO.alpine.v4.2.12"),
		0x0b: []byte("Dream Machine Pro"),
		0x14: []byte("UDM-Pro"),
	})

	info := parseUbiquitiDiscovery(reply)
	if info == nil {
		t.Fatal("expected a parsed reply")
	}
	if info.MAC != "78:8a:20:1a:2b:3c" {
		t.Errorf("MAC = %q", info.MAC)
	}
	if info.Model != "UDM-Pro" {
		t.Errorf("Model = %q", info.Model)
	}
	if info.Firmware != "UDMPRO.alpine.v4.2.12" {
		t.Errorf("Firmware = %q", info.Firmware)
	}
	if info.Hostname != "Dream Machine Pro" {
		t.Errorf("Hostname = %q", info.Hostname)
	}
}

// This is unauthenticated input from a device we know nothing about. It must
// never be able to panic the run, no matter how malformed.
func TestParseUbiquitiDiscoveryRejectsMalformed(t *testing.T) {
	cases := map[string][]byte{
		"empty":                          {},
		"header only":                    {0x01, 0x00, 0x00, 0x00},
		"wrong version":                  {0x09, 0x00, 0x00, 0x04, 0x01, 0x00, 0x01, 0xff},
		"truncated tlv header":           {0x01, 0x00, 0x00, 0x02, 0x01, 0x00},
		"length overruns buffer":         {0x01, 0x00, 0x00, 0x09, 0x03, 0xff, 0xf0, 0x41},
		"declared length beyond payload": {0x01, 0x00, 0xff, 0xff, 0x01, 0x00, 0x02, 0x41, 0x42},
		"zero length value":              {0x01, 0x00, 0x00, 0x03, 0x03, 0x00, 0x00},
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			// The assertion is that this returns rather than panicking or
			// reading out of bounds.
			_ = parseUbiquitiDiscovery(data)
		})
	}
}

// A binary field misread as text must not put control characters into a report
// somebody opens in an editor later.
func TestPrintableString(t *testing.T) {
	got := printableString([]byte{0x00, 'U', 'D', 'M', 0x1b, '[', '3', '1', 'm', 0x7f, ' '})
	if strings.ContainsAny(got, "\x00\x1b\x7f") {
		t.Errorf("printableString kept control characters: %q", got)
	}
	if !strings.Contains(got, "UDM") {
		t.Errorf("printableString dropped the readable part: %q", got)
	}
}

func TestUnifiDetectConfidence(t *testing.T) {
	p := unifiProvider{}
	addr := mustAddr("192.168.1.1")

	// The device identified itself — nothing beats that.
	d := p.Detect(context.Background(), Facts{}, GatewayIdentity{
		Addr:     addr,
		Ubiquiti: &UbiquitiInfo{Model: "UDM-Pro", Firmware: "v4.2.12"},
	})
	if !d.Found || d.Confidence != Definite {
		t.Errorf("discovery reply should be Definite, got found=%v conf=%v", d.Found, d.Confidence)
	}
	if !strings.Contains(d.Product, "UDM-Pro") {
		t.Errorf("product = %q, want the model named", d.Product)
	}

	// A registered MAC prefix is strong but not proof.
	d = p.Detect(context.Background(), Facts{}, GatewayIdentity{
		Addr: addr, MAC: "78:8a:20:1a:2b:3c", Vendor: "Ubiquiti",
	})
	if !d.Found || d.Confidence != Likely {
		t.Errorf("OUI match should be Likely, got found=%v conf=%v", d.Found, d.Confidence)
	}

	// A certificate name is the weakest signal of the three.
	d = p.Detect(context.Background(), Facts{}, GatewayIdentity{
		Addr: addr, TLSNames: []string{"UniFi Dream Machine"}, TLSPort: 443,
	})
	if !d.Found || d.Confidence != Possible {
		t.Errorf("cert name should be Possible, got found=%v conf=%v", d.Found, d.Confidence)
	}

	// Someone else's router must not be reported as UniFi.
	d = p.Detect(context.Background(), Facts{}, GatewayIdentity{
		Addr: addr, MAC: "00:11:32:aa:bb:cc", Vendor: "Synology",
	})
	if d.Found {
		t.Errorf("Synology hardware was detected as UniFi: %+v", d)
	}
}

func TestUnifiEndpoint(t *testing.T) {
	addr := mustAddr("192.168.1.1")
	if got := unifiEndpoint(GatewayIdentity{Addr: addr, TLSPort: 443}); got != "https://192.168.1.1" {
		t.Errorf("endpoint = %q", got)
	}
	// A legacy self-hosted controller lives on 8443 and the port has to survive.
	if got := unifiEndpoint(GatewayIdentity{Addr: addr, TLSPort: 8443}); got != "https://192.168.1.1:8443" {
		t.Errorf("endpoint = %q", got)
	}
}

// The instructions have to be followable standing in a utility room, so they
// name the exact menu path and the exact command to run next.
func TestUnifiSetupHelp(t *testing.T) {
	help := unifiProvider{}.SetupHelp(Detection{Endpoint: "https://192.168.1.1"})
	for _, want := range []string{
		"Control Plane → Integrations",
		"shows it exactly once",
		"ctdev diagnose --deep --unifi https://192.168.1.1",
		"CTDEV_UNIFI_API_KEY",
		":8443",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("setup help is missing %q\n---\n%s", want, help)
		}
	}
}

// Detection must never open a TLS connection or fire a discovery datagram at
// something out on the internet — only at a gateway on a private network.
func TestFingerprintingIsPrivateOnly(t *testing.T) {
	public := mustAddr("203.0.113.9")
	if names, port := tlsIdentity(context.Background(), public); names != nil || port != 0 {
		t.Error("tlsIdentity probed a public address")
	}
	if info := ubiquitiDiscover(context.Background(), public); info != nil {
		t.Error("ubiquitiDiscover probed a public address")
	}
}

func mustAddr(s string) netip.Addr { return netip.MustParseAddr(s) }
