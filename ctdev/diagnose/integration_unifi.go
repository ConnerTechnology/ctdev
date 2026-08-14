package diagnose

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

// UbiquitiInfo is what a Ubiquiti device volunteers about itself when asked on
// the discovery port. No credentials, no session — one datagram out, one back.
type UbiquitiInfo struct {
	Hostname string
	Model    string
	Firmware string
	MAC      string
	ESSID    string
}

// ubiquitiDiscoveryPort is Ubiquiti's device discovery service. A controller
// uses it to find gear to adopt; we use it to find out what we're standing in
// front of.
const ubiquitiDiscoveryPort = 10001

// ubiquitiDiscover sends the discovery probe to a single address and parses
// whatever comes back.
//
// This is aimed at the gateway only, never broadcast: sweeping a network you
// were invited onto but don't own is a different thing entirely, and this
// service is a known amplification vector that shouldn't be sprayed around.
func ubiquitiDiscover(ctx context.Context, addr netip.Addr) *UbiquitiInfo {
	if !IsPrivate(addr) {
		return nil
	}

	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "udp",
		net.JoinHostPort(addr.String(), fmt.Sprint(ubiquitiDiscoveryPort)))
	if err != nil {
		return nil
	}
	defer conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)

	// Version 1, command 0 (discovery request), zero-length payload.
	if _, err := conn.Write([]byte{0x01, 0x00, 0x00, 0x00}); err != nil {
		return nil
	}

	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil || n < 4 {
		return nil
	}
	return parseUbiquitiDiscovery(buf[:n])
}

// Discovery response TLV tags. Only the ones worth reporting are named; the
// rest are skipped by length.
const (
	ubntTagMAC      = 0x01
	ubntTagMACIP    = 0x02
	ubntTagFirmware = 0x03
	// ubntTagName carries the hostname on wired gear and the ESSID on radios.
	// It's one tag, not two, so it gets one name.
	ubntTagName    = 0x0b
	ubntTagModelV1 = 0x0c
	ubntTagModelV2 = 0x14
)

// parseUbiquitiDiscovery decodes the TLV response body.
//
// The wire format is a 4-byte header (version, command, 2-byte payload length)
// followed by tag/length/value triples. It's parsed defensively: this is
// unauthenticated input from a device we know nothing about, so every length
// is bounds-checked and anything unrecognized is skipped rather than guessed at.
func parseUbiquitiDiscovery(data []byte) *UbiquitiInfo {
	if len(data) < 4 {
		return nil
	}
	// Only version 1 and 2 responses share this layout.
	if data[0] != 0x01 && data[0] != 0x02 {
		return nil
	}

	payloadLen := int(binary.BigEndian.Uint16(data[2:4]))
	body := data[4:]
	if payloadLen > 0 && payloadLen <= len(body) {
		body = body[:payloadLen]
	}

	info := &UbiquitiInfo{}
	found := false

	for pos := 0; pos+3 <= len(body); {
		tag := body[pos]
		length := int(binary.BigEndian.Uint16(body[pos+1 : pos+3]))
		pos += 3
		if pos+length > len(body) {
			break
		}
		value := body[pos : pos+length]
		pos += length
		found = true

		switch tag {
		case ubntTagMAC:
			if len(value) >= 6 {
				info.MAC = formatMAC(value[:6])
			}
		case ubntTagMACIP:
			if len(value) >= 6 && info.MAC == "" {
				info.MAC = formatMAC(value[:6])
			}
		case ubntTagFirmware:
			info.Firmware = printableString(value)
		case ubntTagModelV1, ubntTagModelV2:
			if m := printableString(value); m != "" {
				info.Model = m
			}
		case ubntTagName:
			if s := printableString(value); s != "" && info.Hostname == "" {
				info.Hostname = s
			}
		}
	}

	if !found {
		return nil
	}
	return info
}

func formatMAC(b []byte) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = fmt.Sprintf("%02x", v)
	}
	return strings.Join(parts, ":")
}

// printableString keeps only printable ASCII, so a binary field misread as
// text can't put control characters into a report someone opens later.
func printableString(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c >= 0x20 && c < 0x7f {
			sb.WriteByte(c)
		}
	}
	return strings.TrimSpace(sb.String())
}

// unifiProvider recognizes Ubiquiti UniFi equipment.
type unifiProvider struct{}

func (unifiProvider) Name() string { return "UniFi" }

func (unifiProvider) Detect(_ context.Context, _ Facts, gw GatewayIdentity) Detection {
	d := Detection{}

	// Strongest signal: the device answered the discovery probe and told us
	// what it is.
	if gw.Ubiquiti != nil {
		d.Found = true
		d.Confidence = Definite
		d.Product = unifiProductName(gw.Ubiquiti)
		d.Evidence = append(d.Evidence, "answered Ubiquiti discovery on UDP 10001")
		if gw.Ubiquiti.Firmware != "" {
			d.Evidence = append(d.Evidence, "firmware "+gw.Ubiquiti.Firmware)
		}
	}

	// Next: the MAC prefix is registered to Ubiquiti.
	if gw.Vendor == "Ubiquiti" {
		if !d.Found {
			d.Found = true
			d.Confidence = Likely
			d.Product = "UniFi"
		}
		d.Evidence = append(d.Evidence, "gateway MAC "+gw.MAC+" is a Ubiquiti prefix")
	}

	// Weakest: the management certificate names itself.
	if names := unifiCertNames(gw.TLSNames); names != "" {
		if !d.Found {
			d.Found = true
			d.Confidence = Possible
			d.Product = "UniFi"
		}
		d.Evidence = append(d.Evidence, "management certificate names "+names)
	}

	if d.Found {
		d.Endpoint = unifiEndpoint(gw)
	}
	return d
}

// unifiProductName builds the most specific description the discovery reply
// supports, e.g. "UniFi UDM Pro (firmware 4.2.12)".
func unifiProductName(u *UbiquitiInfo) string {
	name := "UniFi"
	if u.Model != "" {
		name += " " + u.Model
	}
	if u.Hostname != "" && u.Hostname != u.Model {
		name += " (" + u.Hostname + ")"
	}
	return name
}

func unifiCertNames(names []string) string {
	var hits []string
	for _, n := range names {
		lower := strings.ToLower(n)
		if strings.Contains(lower, "unifi") || strings.Contains(lower, "ubnt") || strings.Contains(lower, "ubiquiti") {
			hits = append(hits, n)
		}
	}
	return strings.Join(hits, ", ")
}

func unifiEndpoint(gw GatewayIdentity) string {
	port := gw.TLSPort
	if port == 0 {
		port = 443
	}
	if port == 443 {
		return "https://" + gw.Addr.String()
	}
	return fmt.Sprintf("https://%s:%d", gw.Addr, port)
}

// SetupHelp is what makes the detection actionable. It has to be exact enough
// to follow standing in someone's utility room with a phone in one hand.
func (unifiProvider) SetupHelp(d Detection) string {
	endpoint := d.Endpoint
	if endpoint == "" {
		endpoint = "your controller"
	}

	return strings.Join([]string{
		"A deeper look needs read-only API access. About a minute to set up:",
		"  1. Open " + endpoint + " and sign in as an admin",
		"  2. Settings → Control Plane → Integrations",
		"  3. Create New API Key, name it \"ctdev-doctor\"",
		"  4. Copy the key now — UniFi shows it exactly once",
		"Then: CTDEV_UNIFI_API_KEY=<key> ctdev doctor --deep",
		"The controller defaults to the gateway; --unifi <url> overrides it.",
		"Legacy controllers on :8443 predate API keys — use --unifi-user instead.",
	}, "\n")
}
