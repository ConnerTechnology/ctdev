package diagnose

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

// Confidence is how sure a fingerprint is. A guess presented as a fact wastes
// somebody's afternoon, so this travels with every detection.
type Confidence int

const (
	// Possible: circumstantial, e.g. a port that many products use.
	Possible Confidence = iota
	// Likely: a strong signal such as a registered MAC prefix.
	Likely
	// Definite: the device identified itself.
	Definite
)

func (c Confidence) String() string {
	switch c {
	case Definite:
		return "confirmed"
	case Likely:
		return "likely"
	default:
		return "possible"
	}
}

// Detection is what a zero-credential fingerprint concluded about a system on
// this network.
type Detection struct {
	Found      bool
	Confidence Confidence
	// Product is what it appears to be, as specifically as we can tell —
	// "UDM Pro, UniFi OS 4.2.12" rather than just "UniFi".
	Product string
	// Endpoint is where its management interface appears to live.
	Endpoint string
	// Evidence lists what led to this conclusion, so a wrong guess can be
	// argued with rather than just disbelieved.
	Evidence []string
}

// Provider is a known system that can be recognized on this network and, where
// access is available, asked for detail the local machine cannot see on its own.
//
// The interface is read-only by construction. There is no method here that
// changes anything, and there deliberately never will be: ctdev will not
// restart an access point or alter a setting on infrastructure it was merely
// pointed at, from a command called "diagnose".
type Provider interface {
	// Name is the product family, e.g. "UniFi".
	Name() string
	// Detect fingerprints without credentials. It never fails the run — an
	// absent system is simply not found.
	Detect(ctx context.Context, f Facts, gw GatewayIdentity) Detection
	// SetupHelp returns copy-pasteable instructions for granting the
	// read-only access a deeper look would need.
	SetupHelp(d Detection) string
}

// providers returns the known providers. A plain function rather than an
// init-time registry, matching how package cleanup builds its catalog.
func providers() []Provider {
	return []Provider{unifiProvider{}}
}

// GatewayIdentity is the shared fingerprint work, done once and handed to every
// provider so they don't each re-probe the same router.
type GatewayIdentity struct {
	Addr netip.Addr
	MAC  string
	// Vendor is the OUI registrant, when we recognize it.
	Vendor string
	// TLSNames are the subject and SAN names on the management interface's
	// certificate. Appliances name themselves here surprisingly often.
	TLSNames []string
	// TLSPort is the port the certificate came from.
	TLSPort int
	// Ubiquiti holds the reply to the UDP discovery probe, when there was one.
	Ubiquiti *UbiquitiInfo
}

// identifyGateway probes the default gateway without credentials. Everything
// here is a read: an ARP lookup, a TLS handshake, and one UDP datagram.
func identifyGateway(ctx context.Context, f Facts) GatewayIdentity {
	gw := GatewayIdentity{Addr: f.Gateway}
	if !f.Gateway.IsValid() {
		return gw
	}

	gw.MAC = neighborMAC(ctx, f.Platform, f.Gateway)
	gw.Vendor = LookupOUI(gw.MAC)
	gw.TLSNames, gw.TLSPort = tlsIdentity(ctx, f.Gateway)
	gw.Ubiquiti = ubiquitiDiscover(ctx, f.Gateway)
	return gw
}

// neighborMAC reads the gateway's hardware address out of the ARP/neighbour
// table. It's already there — we just pinged it — so this costs nothing.
func neighborMAC(ctx context.Context, info platform.Info, addr netip.Addr) string {
	switch info.OS {
	case platform.Linux:
		return parseIPNeigh(capture(ctx, "ip", "neigh", "show", addr.String()))
	case platform.MacOS:
		return parseArpOutput(capture(ctx, "arp", "-n", addr.String()))
	case platform.Windows:
		out := powershell(ctx,
			`Get-NetNeighbor -IPAddress '`+escapePS(addr.String())+`' -ErrorAction SilentlyContinue |`+
				` Select-Object -First 1 -ExpandProperty LinkLayerAddress`)
		return normalizeMAC(firstLine(out))
	}
	return ""
}

// parseIPNeigh reads `ip neigh show <addr>`:
//
//	10.2.2.1 dev wlp5s0 lladdr 22:0b:8b:c2:7d:db REACHABLE
func parseIPNeigh(out string) string {
	fields := strings.Fields(firstLine(out))
	for i, fld := range fields {
		if fld == "lladdr" && i+1 < len(fields) {
			return normalizeMAC(fields[i+1])
		}
	}
	return ""
}

// parseArpOutput reads BSD `arp -n <addr>`:
//
//	? (192.168.1.1) at 78:8a:20:1a:2b:3c on en0 ifscope [ethernet]
func parseArpOutput(out string) string {
	fields := strings.Fields(firstLine(out))
	for i, fld := range fields {
		if fld == "at" && i+1 < len(fields) {
			return normalizeMAC(fields[i+1])
		}
	}
	return ""
}

// normalizeMAC pads single-digit octets, which BSD arp prints unpadded
// ("0:1a:2b" for "00:1a:2b") and which would otherwise miss every OUI lookup.
func normalizeMAC(mac string) string {
	mac = strings.TrimSpace(strings.ToLower(mac))
	if mac == "" || strings.Contains(mac, "incomplete") {
		return ""
	}
	mac = strings.ReplaceAll(mac, "-", ":")
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return ""
	}
	for i, p := range parts {
		if len(p) == 1 {
			parts[i] = "0" + p
		} else if len(p) != 2 {
			return ""
		}
	}
	return strings.Join(parts, ":")
}

// managementPorts are where consumer and prosumer gear puts a web UI.
var managementPorts = []int{443, 8443, 11443}

// tlsIdentity collects the names on the management interface's certificate.
// Verification is deliberately skipped — every one of these is self-signed by
// design — but only ever against a private address, checked by the caller.
func tlsIdentity(ctx context.Context, addr netip.Addr) ([]string, int) {
	if !IsPrivate(addr) {
		// Never disable certificate verification against something out on the
		// internet. A gateway that isn't on a private address isn't a gateway
		// we should be fingerprinting this way.
		return nil, 0
	}

	for _, port := range managementPorts {
		conn, err := tls.DialWithDialer(
			&net.Dialer{Timeout: 2 * time.Second},
			"tcp",
			net.JoinHostPort(addr.String(), fmt.Sprint(port)),
			// Verification is skipped on purpose: every one of these
			// appliances ships a self-signed certificate, and reading the
			// names off it is the entire point. Nothing is sent over this
			// connection — it is opened, read, and closed.
			//nolint:gosec // Gated to private addresses by the caller above.
			&tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12},
		)
		if err != nil {
			continue
		}

		var names []string
		for _, cert := range conn.ConnectionState().PeerCertificates {
			if cert.Subject.CommonName != "" {
				names = append(names, cert.Subject.CommonName)
			}
			names = append(names, cert.DNSNames...)
			for _, org := range cert.Subject.Organization {
				names = append(names, org)
			}
		}
		conn.Close()
		if len(names) > 0 {
			return dedupeStrings(names), port
		}
	}
	return nil, 0
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// Integrations carries the credentials a deep run may use. Empty is the normal
// case, and means detection only.
//
// Nothing here is ever written to disk by this package. Where these came from
// — an environment variable, a config file on your own machine, or a prompt
// answered once and kept in memory — is the command layer's decision.
type Integrations struct {
	UniFi UnifiCreds
}

// CollectIntegrations runs the vendor probes that credentials unlock, returning
// extra checks and their results to fold into the report.
//
// Every call underneath is a GET. There is no path through this function that
// changes a setting on anyone's infrastructure.
func CollectIntegrations(ctx context.Context, in Integrations) ([]Check, map[string]Result) {
	if !in.UniFi.usable() {
		return nil, nil
	}

	snap, err := collectUnifi(ctx, in.UniFi)
	if err != nil {
		// A controller that refuses us is worth one honest row, not a failed
		// run — the local diagnosis stands on its own.
		return []Check{{
				ID: "unifi.access", Name: "UniFi controller",
				Group: GroupNetwork, Network: true, Deep: true,
			}}, map[string]Result{
				"unifi.access": skipf("could not read the controller: %v", err),
			}
	}

	results := unifiResults(snap)
	checks := unifiChecks(snap)
	if len(checks) > 0 {
		first := results[checks[0].ID]
		if first.Data == nil {
			first.Data = map[string]string{}
		}
		for k, v := range unifiDataFor(snap) {
			first.Data[k] = v
		}
		results[checks[0].ID] = first
	}
	return checks, results
}

// checkNetworkGear identifies the network equipment and, when it recognizes
// something it could look deeper into, says how to enable that.
//
// Deep-only: it sends a discovery datagram and opens TLS connections to the
// router, which is more than a default run should do uninvited.
func checkNetworkGear(ctx context.Context, f Facts) Result {
	if !f.Gateway.IsValid() {
		return skipf("no gateway to identify")
	}
	gw := identifyGateway(ctx, f)

	var found []Detection
	for _, p := range providers() {
		if d := p.Detect(ctx, f, gw); d.Found {
			found = append(found, d)
		}
	}

	if len(found) == 0 {
		if gw.Vendor != "" {
			return infof("gateway looks like %s hardware (%s) — no deep integration for it yet",
				gw.Vendor, gw.MAC)
		}
		return infof("gateway at %s not recognized", f.Gateway)
	}

	d := found[0]
	res := infof("%s detected — %s (%s)", d.Product, d.Endpoint, d.Confidence)
	res.Data = map[string]string{
		"product":    d.Product,
		"endpoint":   d.Endpoint,
		"confidence": d.Confidence.String(),
		"evidence":   strings.Join(d.Evidence, "; "),
	}
	// The setup instructions ride along as advice, so a run that finds UniFi
	// but has no credentials still tells you exactly how to unlock the rest.
	for _, p := range providers() {
		if p.Name() == d.Product || strings.HasPrefix(d.Product, p.Name()) {
			res.Advice = p.SetupHelp(d)
			break
		}
	}
	return res
}
