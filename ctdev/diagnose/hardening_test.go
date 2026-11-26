package diagnose

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

func TestCollectUnifi_APIKeyCapturesTLSCertFingerprint(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "example-api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
	}))
	defer srv.Close()

	snap, err := collectUnifi(context.Background(), UnifiCreds{
		Endpoint: srv.URL,
		APIKey:   "example-api-key",
	})
	if err != nil {
		t.Fatalf("collectUnifi: %v", err)
	}
	if snap.CertFingerprint == "" {
		t.Fatal("expected CertFingerprint populated after API-key GET request")
	}
	if snap.TLSVerified {
		t.Error("httptest self-signed cert should not report TLSVerified=true")
	}
	results := unifiResults(snap)
	if _, ok := results["unifi.tls"]; !ok {
		t.Errorf("expected unifi.tls row for self-signed API-key controller; got keys: %v", results)
	}
}

func TestCrossHostRedirectRefused(t *testing.T) {
	// Target server that must NEVER receive credentials.
	secretHit := false
	evil := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secretHit = true
		_, _ = w.Write([]byte(`{"meta":{"rc":"ok"},"data":[]}`))
	}))
	defer evil.Close()

	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/steal", http.StatusFound)
	}))
	defer redirector.Close()

	t.Run("UniFi client blocks cross-host redirect", func(t *testing.T) {
		_, err := collectUnifi(context.Background(), UnifiCreds{
			Endpoint: redirector.URL,
			APIKey:   "secret-api-key",
		})
		if err == nil || !errors.Is(err, errUnifiPublic) {
			t.Errorf("expected cross-host redirect refusal wrapping errUnifiPublic, got %v", err)
		}
		if secretHit {
			t.Error("UniFi client followed cross-host redirect and leaked request")
		}
	})

	t.Run("Proxmox privateAPIClient blocks cross-host redirect", func(t *testing.T) {
		secretHit = false
		_, err := collectProxmox(context.Background(), ProxmoxCreds{
			Endpoint: redirector.URL,
			TokenID:  "user@pam!tok",
			Secret:   "super-secret",
		})
		if err == nil || !strings.Contains(err.Error(), "refusing cross-host redirect") {
			t.Errorf("expected cross-host redirect refusal, got %v", err)
		}
		if secretHit {
			t.Error("Proxmox client followed cross-host redirect and leaked token")
		}
	})
}

func TestSanitizeAPIErrorAndProxmoxQueryScrubbing(t *testing.T) {
	raw := errors.New(`Get "https://10.0.0.5:8006/api2/json/nodes?ticket=secret123&foo=bar": dial tcp 10.0.0.5:8006: refused`)
	got := sanitizeAPIError(raw)
	if strings.Contains(got.Error(), "secret123") || strings.Contains(got.Error(), "foo=bar") {
		t.Errorf("sanitizeAPIError leaked query string: %q", got.Error())
	}
	if !strings.Contains(got.Error(), "?[redacted]") {
		t.Errorf("expected ?[redacted] marker, got %q", got.Error())
	}

	// proxmoxGet routes client.Do errors through sanitizeAPIError.
	client, err := privateAPIClient("https://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	err = proxmoxGet(context.Background(), client, "https://127.0.0.1:1/api2/json/nodes?token=leaked-val", "PVEAPIToken=a=b", &[]ProxmoxNode{})
	if err == nil {
		t.Fatal("expected connection refused error")
	}
	if strings.Contains(err.Error(), "leaked-val") {
		t.Errorf("proxmoxGet leaked query param in error: %v", err)
	}
}

func TestRedact_PublicLocalIP_UnifiWAN_ShortSSID(t *testing.T) {
	rep := Report{
		Facts: Facts{
			Hostname: "example-box",
			LocalIP:  netip.MustParseAddr("198.51.100.42"), // public IP on interface
		},
		Results: map[string]Result{
			"link.ip":      okf("198.51.100.42 on eth0"),
			"link.wifi":    {Severity: OK, Detail: `connected to "net" on ch 6, internet reachable`, Data: map[string]string{"ssid": "net", "bssid": "00:00:5e:00:53:01"}},
			"unifi.health": unifiHealthVerdict([]UnifiHealth{{Subsystem: "wan", Status: "ok", WANIP: "203.0.113.9"}}),
		},
		Findings: []Finding{
			{Title: "Host 198.51.100.42 on net cannot reach internet via 203.0.113.9"},
		},
	}

	redacted := Redact(rep)
	if redacted.Facts.LocalIP.IsValid() {
		t.Errorf("public Facts.LocalIP was not cleared: %v", redacted.Facts.LocalIP)
	}
	if strings.Contains(redacted.Results["link.ip"].Detail, "198.51.100.42") {
		t.Errorf("public LocalIP leaked in link.ip: %q", redacted.Results["link.ip"].Detail)
	}
	if strings.Contains(redacted.Results["unifi.health"].Detail, "203.0.113.9") {
		t.Errorf("UniFi WANIP leaked in unifi.health: %q", redacted.Results["unifi.health"].Detail)
	}
	if got := redacted.Results["link.wifi"].Data["ssid"]; got != masked {
		t.Errorf("short SSID Data[ssid] = %q, want %q", got, masked)
	}
	// Word-boundary match for short SSID "net" must not corrupt "internet".
	if got := redacted.Results["link.wifi"].Detail; !strings.Contains(got, "internet") {
		t.Errorf("short SSID corrupted word 'internet': %q", got)
	}
	if got := redacted.Findings[0].Title; !strings.Contains(got, "internet") || strings.Contains(got, "198.51.100.42") || strings.Contains(got, "203.0.113.9") {
		t.Errorf("finding redaction unexpected: %q", got)
	}
}

func TestMarkdown_EscapePipesNewlinesAndSecretishKeys(t *testing.T) {
	if got := escapePipes("a|b\nc\r\nd"); got != `a\|b c d` {
		t.Errorf("escapePipes() = %q, want %q", got, `a\|b c d`)
	}

	data := map[string]string{
		"ok_field":     "visible",
		"session_id":   "s",
		"auth_cookie":  "c",
		"bearer_val":   "b",
		"x_csrf":       "x",
		"private_pem":  "p",
		"empty_hidden": "",
	}
	vis := visibleData(data)
	if len(vis) != 1 || vis["ok_field"] != "visible" {
		t.Errorf("visibleData() = %v, want only ok_field", vis)
	}
}

func TestParseSizeSuffixKB_Fractional(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1.50G", 1572864},
		{"512.50M", 524800},
		{"100.00K", 100},
		{"", 0},
		{"invalid", 0},
	}
	for _, tc := range cases {
		if got := parseSizeSuffixKB(tc.in); got != tc.want {
			t.Errorf("parseSizeSuffixKB(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseAirportData_SSIDWithColon(t *testing.T) {
	sample := `Wi-Fi:
      Interfaces:
        en0:
          Current Network Information:
            Home:5G:
              Channel: 44 (5GHz, 80MHz)
              Signal / Noise: -58 dBm / -92 dBm
              Transmit Rate: 866
          Other Local Wi-Fi Networks:
            Neighbor:2G:
              Channel: 6
`
	w := parseAirportData(sample)
	if !w.Associated || w.SSID != "Home:5G" || w.Channel != 44 || w.RSSI != -58 {
		t.Errorf("parseAirportData(colon SSID) = %+v, want SSID=Home:5G ch=44 rssi=-58", w)
	}
}

func TestDiagnose_MissingCorrelationRows(t *testing.T) {
	t.Run("dns.resolvers failed + net.icmp healthy without dns.public", func(t *testing.T) {
		findings := Diagnose(map[string]Result{
			"link.ip":       okf("10.0.0.5"),
			"link.gateway":  okf("1 ms"),
			"net.icmp":      okf("10 ms"),
			"dns.resolvers": failf("fix", "timeout"),
		}, Facts{LocalIP: netip.MustParseAddr("10.0.0.5")})
		if len(findings) != 1 || !strings.Contains(findings[0].Title, "Name resolution is broken") {
			t.Errorf("findings = %+v, want Name resolution is broken", findings)
		}
	})

	t.Run("hw.memory + sys.oom emits OOM finding", func(t *testing.T) {
		findings := Diagnose(map[string]Result{
			"hw.memory": warnf("a", "95%% used"),
			"sys.oom":   failf("b", "3 kills"),
		}, Facts{})
		if len(findings) != 1 || !strings.Contains(findings[0].Title, "out of memory") {
			t.Errorf("findings = %+v, want out of memory", findings)
		}
	})

	t.Run("healthy wifi + failed gateway blames router not signal", func(t *testing.T) {
		findings := Diagnose(map[string]Result{
			"link.ip":      okf("10.0.0.5"),
			"link.wifi":    okf("-50 dBm"),
			"link.gateway": failf("a", "no reply"),
		}, Facts{LocalIP: netip.MustParseAddr("10.0.0.5"), IsWiFi: true})
		if len(findings) != 1 || findings[0].Title != "The router isn't responding." {
			t.Errorf("findings = %+v, want router not responding", findings)
		}
	})

	t.Run("all healthy emits zero findings", func(t *testing.T) {
		findings := Diagnose(map[string]Result{
			"link.ip":      okf("10.0.0.5"),
			"link.gateway": okf("1 ms"),
			"net.icmp":     okf("10 ms"),
			"hw.memory":    okf("40%%"),
		}, Facts{LocalIP: netip.MustParseAddr("10.0.0.5")})
		if len(findings) != 0 {
			t.Errorf("expected 0 findings for healthy system, got %+v", findings)
		}
	})
}

func TestRunAll_HonorsCallerCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	checks := []Check{{
		ID: "slow.check",
		Run: func(c context.Context, _ Facts) Result {
			close(started)
			<-c.Done()
			return skipf("cancelled")
		},
	}}

	done := make(chan map[string]Result, 1)
	go func() {
		done <- RunAll(ctx, checks, Facts{})
	}()

	<-started
	cancel()

	select {
	case res := <-done:
		if res["slow.check"].Severity != Skipped {
			t.Errorf("expected Skipped after cancel, got %+v", res["slow.check"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunAll did not return promptly after caller context cancellation")
	}
}

func TestSmartAndOSSupportEdgeCases(t *testing.T) {
	if got := smartVerdict(""); got != smartUnknown {
		t.Errorf("smartVerdict(empty) = %v, want smartUnknown", got)
	}
	if got := smartVerdict("Smartctl open device: /dev/sda failed: Permission denied\n"); got != smartUnknown {
		t.Errorf("smartVerdict(permission denied) = %v, want smartUnknown", got)
	}

	if res := checkOSSupport(context.Background(), Facts{
		Platform: platform.Info{OS: platform.Linux, Distro: "ubuntu", DistroVersion: "20.04"},
	}); res.Severity != Fail {
		t.Errorf("EOL ubuntu 20.04 = %+v, want Fail", res)
	}
	if res := checkOSSupport(context.Background(), Facts{
		Platform: platform.Info{OS: platform.Linux, Distro: "ubuntu", DistroVersion: "24.04"},
	}); res.Severity != OK {
		t.Errorf("supported ubuntu 24.04 = %+v, want OK", res)
	}
	if res := checkOSSupport(context.Background(), Facts{
		Platform: platform.Info{OS: platform.MacOS},
	}); res.Severity != Skipped {
		t.Errorf("macOS checkOSSupport = %+v, want Skipped", res)
	}
}
