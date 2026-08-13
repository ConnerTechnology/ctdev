package diagnose

import (
	"net/netip"
	"strings"
	"testing"
)

// res builds a result set concisely for the table below.
func res(severity Severity, detail string) Result {
	return Result{Severity: severity, Detail: detail}
}

func withData(r Result, kv ...string) Result {
	r.Data = map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		r.Data[kv[i]] = kv[i+1]
	}
	return r
}

func wifiFacts() Facts {
	return Facts{Iface: "wlan0", IsWiFi: true, LocalIP: netip.MustParseAddr("192.168.1.42")}
}

func wiredFacts() Facts {
	return Facts{Iface: "eth0", LocalIP: netip.MustParseAddr("192.168.1.42")}
}

// Each case is one row of the verdict table: a plausible combination of check
// results, and the single conclusion a technician would draw from it.
func TestDiagnoseVerdicts(t *testing.T) {
	tests := []struct {
		name    string
		facts   Facts
		results map[string]Result
		// wantTitle is a distinctive fragment of the expected verdict.
		wantTitle string
		wantSev   Severity
	}{
		{
			name:  "no address at all",
			facts: Facts{Iface: "eth0"},
			results: map[string]Result{
				"link.ip":      res(Fail, "no usable address"),
				"link.gateway": res(Fail, "no default route"),
				"net.icmp":     res(Fail, "cannot reach"),
			},
			wantTitle: "isn't on a network",
			wantSev:   Fail,
		},
		{
			name:  "APIPA means DHCP failed",
			facts: Facts{Iface: "wlan0", IsWiFi: true, LocalIP: netip.MustParseAddr("169.254.9.12")},
			results: map[string]Result{
				"link.ip":      res(Fail, "self-assigned"),
				"link.gateway": res(Fail, "no default route"),
			},
			wantTitle: "isn't handing out addresses",
			wantSev:   Fail,
		},
		{
			name:  "weak wifi, router unreachable",
			facts: wifiFacts(),
			results: map[string]Result{
				"link.ip":      res(OK, "192.168.1.42"),
				"link.wifi":    withData(res(Fail, "-88 dBm"), DataRSSI, "-88"),
				"link.gateway": res(Fail, "not answering"),
			},
			wantTitle: "too far from the access point",
			wantSev:   Fail,
		},
		{
			name:  "router unreachable on a cable",
			facts: wiredFacts(),
			results: map[string]Result{
				"link.ip":      res(OK, "192.168.1.42"),
				"link.gateway": res(Fail, "not answering"),
			},
			wantTitle: "router isn't responding",
			wantSev:   Fail,
		},
		{
			name:  "captive portal",
			facts: wifiFacts(),
			results: map[string]Result{
				"link.ip":      res(OK, "192.168.1.42"),
				"link.gateway": res(OK, "answering"),
				"net.captive":  withData(res(Fail, "redirected"), DataCaptivePortal, "yes"),
			},
			wantTitle: "sign in through a browser",
			wantSev:   Fail,
		},
		{
			name:  "wrong clock breaks TLS",
			facts: wiredFacts(),
			results: map[string]Result{
				"link.ip":      res(OK, "192.168.1.42"),
				"link.gateway": res(OK, "answering"),
				"sys.clock":    withData(res(Fail, "clock is wrong"), DataClockSkewSec, "7200"),
				"net.https":    res(Fail, "certificate rejected"),
			},
			wantTitle: "clock is wrong",
			wantSev:   Fail,
		},
		{
			name:  "router fine, ISP down",
			facts: wiredFacts(),
			results: map[string]Result{
				"link.ip":      res(OK, "192.168.1.42"),
				"link.gateway": res(OK, "answering in 2ms"),
				"net.icmp":     res(Fail, "cannot reach 1.1.1.1"),
			},
			wantTitle: "connection to the internet is down",
			wantSev:   Fail,
		},
		{
			name:  "their DNS server is down",
			facts: wiredFacts(),
			results: map[string]Result{
				"link.ip":       res(OK, "192.168.1.42"),
				"link.gateway":  res(OK, "answering"),
				"net.icmp":      res(OK, "1.1.1.1 in 17ms"),
				"dns.resolvers": res(Fail, "no configured resolver answered"),
				"dns.public":    res(OK, "1.1.1.1 answering"),
			},
			wantTitle: "DNS server is down",
			wantSev:   Fail,
		},
		{
			name:  "local resolver service broken",
			facts: wiredFacts(),
			results: map[string]Result{
				"link.ip":       res(OK, "192.168.1.42"),
				"link.gateway":  res(OK, "answering"),
				"dns.resolvers": res(OK, "answering"),
				"dns.system":    res(Fail, "the system cannot resolve"),
			},
			wantTitle: "own DNS service is broken",
			wantSev:   Fail,
		},
		{
			name:  "broken IPv6 stalls pages",
			facts: wiredFacts(),
			results: map[string]Result{
				"link.ip":      res(OK, "192.168.1.42"),
				"link.gateway": res(OK, "answering"),
				"net.icmp":     res(OK, "reachable"),
				"net.ipv6":     res(Fail, "cannot reach the IPv6 internet"),
			},
			wantTitle: "Broken IPv6",
			wantSev:   Warn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := Diagnose(tt.results, tt.facts)
			if len(findings) == 0 {
				t.Fatalf("no verdict reached")
			}
			top := findings[0]
			if !strings.Contains(top.Title, tt.wantTitle) {
				t.Errorf("verdict = %q, want it to mention %q", top.Title, tt.wantTitle)
			}
			if top.Severity != tt.wantSev {
				t.Errorf("severity = %v, want %v", top.Severity, tt.wantSev)
			}
			// A verdict without an action is just a restatement of the problem.
			if top.Action == "" {
				t.Error("verdict carries no action")
			}
			if len(top.Because) == 0 {
				t.Error("verdict cites no checks")
			}
		})
	}
}

// The whole point of layering the network rules is that one root cause
// produces one verdict. A machine with an unplugged cable fails every network
// check there is, and reporting six findings for it would be worse than none.
func TestDiagnoseReportsOneNetworkRootCause(t *testing.T) {
	everythingBroken := map[string]Result{
		"link.ip":       res(Fail, "no usable address"),
		"link.gateway":  res(Fail, "no default route"),
		"dns.resolvers": res(Fail, "no resolver answered"),
		"dns.system":    res(Fail, "cannot resolve"),
		"net.icmp":      res(Fail, "cannot reach"),
		"net.captive":   res(Fail, "no answer"),
		"net.https":     res(Fail, "cannot connect"),
		"net.ipv6":      res(Fail, "unreachable"),
	}

	findings := Diagnose(everythingBroken, Facts{Iface: "eth0"})
	if len(findings) != 1 {
		var titles []string
		for _, f := range findings {
			titles = append(titles, f.Title)
		}
		t.Fatalf("got %d verdicts, want exactly 1 root cause: %v", len(findings), titles)
	}
}

// A plain connection failure fails the captive-portal check too. Only an
// actual redirect means there's a sign-in page, and telling someone to look
// for one that doesn't exist wastes the call.
func TestDiagnoseDoesNotInventCaptivePortal(t *testing.T) {
	findings := Diagnose(map[string]Result{
		"link.ip":      res(OK, "192.168.1.42"),
		"link.gateway": res(OK, "answering"),
		"net.captive":  res(Fail, "no answer from the internet"), // no portal marker
		"net.icmp":     res(Fail, "cannot reach"),
	}, wiredFacts())

	for _, f := range findings {
		if strings.Contains(f.Title, "sign in") {
			t.Errorf("claimed a captive portal without a redirect: %q", f.Title)
		}
	}
}

// A check that never ran is not evidence. Skipped results must not be read as
// either healthy or broken, or the engine starts concluding things it never
// observed.
func TestDiagnoseIgnoresSkippedChecks(t *testing.T) {
	findings := Diagnose(map[string]Result{
		"link.ip":      res(OK, "192.168.1.42"),
		"link.gateway": res(Skipped, "no usable ping"),
		"net.icmp":     res(Skipped, "no usable ping"),
	}, wiredFacts())

	if len(findings) != 0 {
		t.Errorf("skipped checks produced verdicts: %+v", findings)
	}
}

func TestDiagnoseHealthyMachineIsQuiet(t *testing.T) {
	findings := Diagnose(map[string]Result{
		"link.ip":       res(OK, "192.168.1.42"),
		"link.gateway":  res(OK, "answering in 2ms"),
		"dns.resolvers": res(OK, "answering"),
		"dns.system":    res(OK, "resolves"),
		"net.icmp":      res(OK, "reachable"),
		"net.captive":   res(OK, "reachable"),
		"net.https":     res(OK, "verified"),
		"hw.disk":       res(OK, "16% full"),
		"hw.memory":     res(OK, "plenty"),
	}, wiredFacts())

	if len(findings) != 0 {
		t.Errorf("a healthy machine should produce no verdicts, got %+v", findings)
	}
}

// Hardware faults are independent of each other and of the network, so unlike
// the network rules these accumulate.
func TestDiagnoseHardwareVerdictsAccumulate(t *testing.T) {
	findings := Diagnose(map[string]Result{
		"link.ip":      res(OK, "192.168.1.42"),
		"link.gateway": res(OK, "answering"),
		"hw.smart":     res(Fail, "reports failing health"),
		"hw.disk":      res(Fail, "98% full"),
		"hw.raid":      res(Fail, "md0 degraded"),
	}, wiredFacts())

	if len(findings) != 3 {
		t.Fatalf("got %d verdicts, want 3 independent hardware faults", len(findings))
	}
}

// "My computer is slow" is a thermal problem more often than anyone expects,
// but heat alone under no load is just a warm room.
func TestDiagnoseThermalNeedsLoad(t *testing.T) {
	hot := map[string]Result{
		"hw.temp": withData(res(Warn, "CPU at 84°C"), DataTempC, "84"),
		"hw.load": res(Warn, "58.0 across 8 cores"),
	}
	if got := Diagnose(hot, wiredFacts()); len(got) != 1 {
		t.Fatalf("hot under load should produce a verdict, got %+v", got)
	}

	hotIdle := map[string]Result{
		"hw.temp": withData(res(Warn, "CPU at 84°C"), DataTempC, "84"),
		"hw.load": res(OK, "0.3 across 8 cores"),
	}
	if got := Diagnose(hotIdle, wiredFacts()); len(got) != 0 {
		t.Errorf("hot but idle should not claim throttling, got %+v", got)
	}
}

func TestDiagnoseOrdersBySeverity(t *testing.T) {
	findings := Diagnose(map[string]Result{
		"link.ip":      res(OK, "192.168.1.42"),
		"link.gateway": res(OK, "answering"),
		"net.icmp":     res(OK, "reachable"),
		"net.ipv6":     res(Fail, "unreachable"), // produces a Warn verdict
		"hw.smart":     res(Fail, "failing"),     // produces a Fail verdict
	}, wiredFacts())

	if len(findings) < 2 {
		t.Fatalf("expected both verdicts, got %+v", findings)
	}
	if findings[0].Severity != Fail {
		t.Errorf("most severe verdict should lead, got %v first", findings[0].Severity)
	}
}

func TestSkewText(t *testing.T) {
	rs := resultSet{"sys.clock": withData(res(Fail, ""), DataClockSkewSec, "-7200")}
	// Direction is stated separately; the magnitude should read plainly.
	if got := skewText(rs); got != "2 hours" {
		t.Errorf("skewText = %q, want %q", got, "2 hours")
	}
	// Missing data must degrade to prose rather than printing a bare zero.
	if got := skewText(resultSet{}); got != "more than five minutes" {
		t.Errorf("skewText with no data = %q", got)
	}
}

// These are the conclusions no client-side probe can reach — the whole reason
// the vendor integration exists.
func TestDiagnoseInfrastructureVerdicts(t *testing.T) {
	base := map[string]Result{
		"link.ip":      res(OK, "192.168.1.42"),
		"link.gateway": res(OK, "answering"),
		"net.icmp":     res(OK, "reachable"),
	}
	with := func(extra map[string]Result) map[string]Result {
		out := map[string]Result{}
		for k, v := range base {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}

	tests := []struct {
		name      string
		results   map[string]Result
		wantTitle string
	}{
		{
			name:      "radar eviction",
			results:   with(map[string]Result{"unifi.radar": res(Fail, "2 radar detections on Living Room")}),
			wantTitle: "knocked off its channel by radar",
		},
		{
			// The finding the whole integration exists for: the laptop blames
			// itself, the controller shows the access point is the problem.
			name: "weak signal explained by a saturated mesh AP",
			results: with(map[string]Result{
				"link.wifi":     res(Warn, "-76 dBm"),
				"unifi.airtime": res(Fail, "ch 36 is 88% busy"),
				"unifi.devices": res(Warn, "Garage on a wireless mesh uplink"),
			}),
			wantTitle: "access point serving this machine is the bottleneck",
		},
		{
			name: "weak signal explained by congestion alone",
			results: with(map[string]Result{
				"link.wifi":     res(Warn, "-76 dBm"),
				"unifi.airtime": res(Warn, "ch 6 is 72% busy"),
			}),
			wantTitle: "congested channel, not just distance",
		},
		{
			name:      "gateway reports its own WAN down",
			results:   with(map[string]Result{"unifi.health": res(Fail, "WAN status \"error\"")}),
			wantTitle: "gateway reports the internet connection as down",
		},
		{
			name:      "min-RSSI kicks working clients",
			results:   with(map[string]Result{"unifi.wlan": res(Warn, "minimum-RSSI set aggressively on Home")}),
			wantTitle: "disconnect distant clients",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := Diagnose(tt.results, wiredFacts())
			var titles []string
			matched := false
			for _, f := range findings {
				titles = append(titles, f.Title)
				if strings.Contains(f.Title, tt.wantTitle) {
					matched = true
					if f.Action == "" {
						t.Error("verdict carries no action")
					}
				}
			}
			if !matched {
				t.Errorf("no verdict mentioned %q; got %v", tt.wantTitle, titles)
			}
		})
	}
}

// Without controller access there are no vendor rows at all, and the local
// diagnosis must stand on its own without inventing infrastructure findings.
func TestDiagnoseWithoutControllerAccess(t *testing.T) {
	findings := Diagnose(map[string]Result{
		"link.ip":      res(OK, "192.168.1.42"),
		"link.gateway": res(OK, "answering"),
		"link.wifi":    res(Warn, "-76 dBm"),
		"unifi.access": res(Skipped, "could not read the controller"),
	}, wifiFacts())

	for _, f := range findings {
		if strings.Contains(f.Title, "access point serving this machine") {
			t.Error("claimed an infrastructure verdict with no controller data")
		}
	}
}
