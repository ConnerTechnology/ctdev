package diagnose

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

func sampleReport() Report {
	return Report{
		Version: "12.12.1",
		Started: time.Date(2026, 8, 13, 9, 15, 53, 0, time.UTC),
		Facts: Facts{
			Hostname: "client-laptop",
			Iface:    "wlan0",
			IsWiFi:   true,
			LocalIP:  netip.MustParseAddr("192.168.1.42"),
			Gateway:  netip.MustParseAddr("192.168.1.1"),
			DNS:      []netip.Addr{netip.MustParseAddr("192.168.1.1")},
		},
		Checks: []Check{
			{ID: "link.wifi", Name: "Wi-Fi signal", Group: GroupNetwork},
			{ID: "net.public", Name: "Public address", Group: GroupInternet},
		},
		Results: map[string]Result{
			"link.wifi": {
				Severity: Warn,
				Detail:   `"Hotel Guest", -78 dBm, ch 6 (2.4 GHz)`,
				Advice:   "Move closer to the access point.",
				Data: map[string]string{
					"ssid":   "Hotel Guest",
					"bssid":  "aa:bb:cc:dd:ee:ff",
					DataRSSI: "-78",
				},
			},
			"net.public": {
				Severity: OK,
				Detail:   "203.0.113.9 (US)",
				Data:     map[string]string{"public_ip": "203.0.113.9", "country": "US"},
			},
		},
		Findings: []Finding{{
			Severity: Warn,
			Title:    "The Wi-Fi signal is too weak to be reliable.",
			Detail:   `"Hotel Guest" is reaching this machine at -78 dBm.`,
			Action:   "Move closer to the access point.",
			Because:  []string{"link.wifi"},
		}},
	}
}

func TestMarkdownStructure(t *testing.T) {
	md := Markdown(sampleReport())

	for _, want := range []string{
		"# Diagnostic report — client-laptop",
		"2026-08-13 09:15 UTC",
		"**ctdev:** 12.12.1",
		"## What needs attention",
		"The Wi-Fi signal is too weak to be reliable.",
		"**What to do:** Move closer to the access point.",
		"## Network",
		"| ⚠ | Wi-Fi signal |",
		"read-only",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report is missing %q", want)
		}
	}
}

func TestMarkdownHealthyReportSaysSo(t *testing.T) {
	r := sampleReport()
	r.Findings = nil
	md := Markdown(r)
	if !strings.Contains(md, "Nothing.") {
		t.Error("a report with no findings should say so explicitly rather than leaving an empty section")
	}
}

// A stray pipe in a detail string would silently break the table it lands in.
func TestMarkdownEscapesPipes(t *testing.T) {
	r := sampleReport()
	r.Results["link.wifi"] = Result{Severity: OK, Detail: "channel 6 | 20MHz"}
	if !strings.Contains(Markdown(r), `channel 6 \| 20MHz`) {
		t.Error("pipes in a detail string must be escaped so the table survives")
	}
}

// A report is a file people forward. Nothing that looks like a credential may
// reach it, whoever added the check that produced it — which is why this is
// enforced at the render boundary rather than by convention.
func TestMarkdownNeverRendersCredentials(t *testing.T) {
	const secret = "s3cr3t-unifi-api-key-abc123"

	r := sampleReport()
	r.Results["integration.unifi"] = Result{
		Severity: OK,
		Detail:   "UniFi controller reachable",
		Data: map[string]string{
			"api_key":     secret,
			"token":       secret,
			"password":    secret,
			"Secret":      secret,
			"auth_header": secret,
			// A legitimate value alongside them must still come through.
			"controller": "https://192.168.1.1",
		},
	}
	r.Checks = append(r.Checks, Check{ID: "integration.unifi", Name: "UniFi", Group: GroupNetwork})

	md := Markdown(r)
	if strings.Contains(md, secret) {
		t.Fatal("a credential reached the rendered report")
	}
	if !strings.Contains(md, "https://192.168.1.1") {
		t.Error("filtering credentials must not drop ordinary measurements")
	}
}

func TestIsSecretish(t *testing.T) {
	for _, key := range []string{"api_key", "API_KEY", "token", "Bearer_Token", "password", "passwd", "client_secret", "credential", "auth_header"} {
		if !isSecretish(key) {
			t.Errorf("%q should be treated as a credential", key)
		}
	}
	for _, key := range []string{"ssid", "channel", "rssi_dbm", "public_ip", "country", "controller"} {
		if isSecretish(key) {
			t.Errorf("%q is not a credential and should be rendered", key)
		}
	}
}

func TestReportFilename(t *testing.T) {
	got := ReportFilename(sampleReport())
	want := "ctdev-diagnose-client-laptop-20260813-091553.md"
	if got != want {
		t.Errorf("ReportFilename = %q, want %q", got, want)
	}

	// A hostname with characters no filesystem wants must not produce a path
	// that fails to write — or worse, one that escapes the directory.
	r := sampleReport()
	r.Facts.Hostname = "../../etc/pass wd"
	got = ReportFilename(r)
	if strings.Contains(got, "/") || strings.Contains(got, " ") {
		t.Errorf("ReportFilename = %q, want path separators and spaces sanitized", got)
	}
}

func TestRedact(t *testing.T) {
	out := Redact(sampleReport())
	md := Markdown(out)

	for _, secret := range []string{"Hotel Guest", "aa:bb:cc:dd:ee:ff", "203.0.113.9"} {
		if strings.Contains(md, secret) {
			t.Errorf("%q survived redaction", secret)
		}
	}

	// The signal strength is the diagnostically useful part and identifies
	// nobody — masking it would leave a report that can't be acted on.
	if !strings.Contains(md, "-78") {
		t.Error("redaction removed the signal strength, which is not an identifier")
	}
	// The hostname is how you tell reports from different machines apart.
	if !strings.Contains(md, "client-laptop") {
		t.Error("redaction removed the hostname, which is needed to tell reports apart")
	}
}

// Masking every public address would scrub 1.1.1.1 and 8.8.8.8 out of the very
// comparison that makes a DNS diagnosis legible.
func TestRedactKeepsPublicResolvers(t *testing.T) {
	r := sampleReport()
	r.Checks = append(r.Checks, Check{ID: "dns.public", Name: "Public DNS", Group: GroupInternet})
	r.Results["dns.public"] = Result{Severity: OK, Detail: "1.1.1.1 answering (12ms)"}

	md := Markdown(Redact(r))
	if !strings.Contains(md, "1.1.1.1") {
		t.Error("public resolvers must survive redaction — the comparison is the diagnosis")
	}
	if strings.Contains(md, "203.0.113.9") {
		t.Error("this connection's own public address should still be masked")
	}
}

// Redaction has to reach the findings too. The verdict block quotes the SSID
// back at you, and a report where the table is masked but the headline isn't
// would be worse than one that never claimed to redact at all.
func TestRedactCoversFindings(t *testing.T) {
	out := Redact(sampleReport())
	if len(out.Findings) == 0 {
		t.Fatal("findings were dropped")
	}
	if strings.Contains(out.Findings[0].Detail, "Hotel Guest") {
		t.Error("the SSID survived in the verdict block")
	}
}

func TestRedactLeavesOriginalUntouched(t *testing.T) {
	original := sampleReport()
	_ = Redact(original)
	if original.Results["net.public"].Detail != "203.0.113.9 (US)" {
		t.Error("Redact mutated the report it was given instead of copying it")
	}
	if !strings.Contains(original.Findings[0].Detail, "Hotel Guest") {
		t.Error("Redact mutated the original findings")
	}
}

func TestRedactCatchesUncollectedMACs(t *testing.T) {
	r := sampleReport()
	r.Checks = append(r.Checks, Check{ID: "link.arp", Name: "Neighbours", Group: GroupNetwork})
	// A MAC the identifier collector never saw must still be masked.
	r.Results["link.arp"] = Result{Severity: Info, Detail: "gateway is 11:22:33:44:55:66"}

	if strings.Contains(Markdown(Redact(r)), "11:22:33:44:55:66") {
		t.Error("a MAC address not in the collected set survived redaction")
	}
}
