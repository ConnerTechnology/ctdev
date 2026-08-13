package diagnose

import (
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestParsePingUnixLinux(t *testing.T) {
	// Real iputils output.
	const out = `--- 1.1.1.1 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2003ms
rtt min/avg/max/mdev = 15.529/16.635/17.705/0.888 ms`

	p, err := parsePingUnix(out)
	if err != nil {
		t.Fatalf("parsePingUnix: %v", err)
	}
	if p.Sent != 3 || p.Received != 3 {
		t.Errorf("sent/received = %d/%d, want 3/3", p.Sent, p.Received)
	}
	if p.LossPercent != 0 {
		t.Errorf("loss = %v, want 0", p.LossPercent)
	}
	if p.AvgRTT != 16635*time.Microsecond {
		t.Errorf("avg = %v, want 16.635ms", p.AvgRTT)
	}
	if p.Jitter != 888*time.Microsecond {
		t.Errorf("jitter = %v, want 0.888ms", p.Jitter)
	}
}

func TestParsePingUnixBSD(t *testing.T) {
	// macOS says "packets received" and "round-trip ... stddev", where Linux
	// says "received" and "rtt ... mdev". One parser has to take both.
	const out = `--- 1.1.1.1 ping statistics ---
5 packets transmitted, 4 packets received, 20.0% packet loss
round-trip min/avg/max/stddev = 12.345/14.567/18.901/2.345 ms`

	p, err := parsePingUnix(out)
	if err != nil {
		t.Fatalf("parsePingUnix: %v", err)
	}
	if p.Sent != 5 || p.Received != 4 {
		t.Errorf("sent/received = %d/%d, want 5/4", p.Sent, p.Received)
	}
	if p.LossPercent != 20 {
		t.Errorf("loss = %v, want 20", p.LossPercent)
	}
	if p.AvgRTT != 14567*time.Microsecond {
		t.Errorf("avg = %v, want 14.567ms", p.AvgRTT)
	}
}

func TestParsePingUnixTotalLoss(t *testing.T) {
	// With every packet lost there is no RTT line at all — the parser has to
	// return the loss rather than fail, because total loss is the finding.
	const out = `--- 192.168.1.1 ping statistics ---
5 packets transmitted, 0 received, 100% packet loss, time 4098ms`

	p, err := parsePingUnix(out)
	if err != nil {
		t.Fatalf("parsePingUnix: %v", err)
	}
	if !p.Lost() {
		t.Error("expected Lost() to be true")
	}
	if p.LossPercent != 100 {
		t.Errorf("loss = %v, want 100", p.LossPercent)
	}
}

func TestParsePingUnixUnusable(t *testing.T) {
	if _, err := parsePingUnix("ping: connect: Network is unreachable"); err == nil {
		t.Error("expected an error for output with no statistics")
	}
}

func TestParsePingWindows(t *testing.T) {
	const out = `Pinging 1.1.1.1 with 32 bytes of data:
Reply from 1.1.1.1: bytes=32 time=14ms TTL=57

Ping statistics for 1.1.1.1:
    Packets: Sent = 4, Received = 4, Lost = 0 (0% loss),
Approximate round trip times in milli-seconds:
    Minimum = 14ms, Maximum = 17ms, Average = 15ms`

	p, err := parsePingWindows(out)
	if err != nil {
		t.Fatalf("parsePingWindows: %v", err)
	}
	if p.Sent != 4 || p.Received != 4 {
		t.Errorf("sent/received = %d/%d, want 4/4", p.Sent, p.Received)
	}
	if p.AvgRTT != 15*time.Millisecond {
		t.Errorf("avg = %v, want 15ms", p.AvgRTT)
	}
}

func TestParsePingWindowsTotalLoss(t *testing.T) {
	const out = `Ping statistics for 192.168.1.1:
    Packets: Sent = 4, Received = 0, Lost = 4 (100% loss),`

	p, err := parsePingWindows(out)
	if err != nil {
		t.Fatalf("parsePingWindows: %v", err)
	}
	if !p.Lost() || p.LossPercent != 100 {
		t.Errorf("expected total loss, got %+v", p)
	}
}

// The gateway result is the fork the whole diagnosis turns on, so its
// thresholds get spelled out here.
func TestGatewayVerdict(t *testing.T) {
	tests := []struct {
		name   string
		p      PingResult
		isWiFi bool
		want   Severity
	}{
		{"healthy wired", PingResult{Sent: 5, Received: 5, AvgRTT: 2 * time.Millisecond}, false, OK},
		{"healthy wifi", PingResult{Sent: 5, Received: 5, AvgRTT: 8 * time.Millisecond}, true, OK},
		{"router silent on wifi", PingResult{Sent: 5, LossPercent: 100}, true, Fail},
		{"router silent on cable", PingResult{Sent: 5, LossPercent: 100}, false, Fail},
		{"partial loss", PingResult{Sent: 5, Received: 4, LossPercent: 20, AvgRTT: 3 * time.Millisecond}, true, Warn},
		// A wired gateway answering in 40ms is not a healthy wired gateway,
		// but the same figure over Wi-Fi is unremarkable.
		{"slow for a cable", PingResult{Sent: 5, Received: 5, AvgRTT: 40 * time.Millisecond}, false, Warn},
		{"fine for wifi", PingResult{Sent: 5, Received: 5, AvgRTT: 40 * time.Millisecond}, true, OK},
		{"slow even for wifi", PingResult{Sent: 5, Received: 5, AvgRTT: 90 * time.Millisecond}, true, Warn},
	}
	for _, tt := range tests {
		res := gatewayVerdict(tt.p, "192.168.1.1", tt.isWiFi)
		if res.Severity != tt.want {
			t.Errorf("%s: severity = %v, want %v (%s)", tt.name, res.Severity, tt.want, res.Detail)
		}
		if (tt.want == Warn || tt.want == Fail) && res.Advice == "" {
			t.Errorf("%s: no advice given", tt.name)
		}
	}
}

// The advice has to differ by link type: "move closer to the router" is
// useless to someone on a cable, and "check the cable" is useless on Wi-Fi.
func TestGatewayVerdictAdviceMatchesLinkType(t *testing.T) {
	lost := PingResult{Sent: 5, LossPercent: 100}

	wifi := gatewayVerdict(lost, "192.168.1.1", true)
	if !strings.Contains(strings.ToLower(wifi.Advice), "closer") {
		t.Errorf("Wi-Fi advice = %q, want it to mention distance", wifi.Advice)
	}

	wired := gatewayVerdict(lost, "192.168.1.1", false)
	if !strings.Contains(strings.ToLower(wired.Advice), "cable") {
		t.Errorf("wired advice = %q, want it to mention the cable", wired.Advice)
	}
}

func TestInternetVerdict(t *testing.T) {
	tests := []struct {
		name string
		p    PingResult
		want Severity
	}{
		{"healthy", PingResult{Sent: 5, Received: 5, AvgRTT: 17 * time.Millisecond}, OK},
		{"lossy", PingResult{Sent: 5, Received: 4, LossPercent: 20, AvgRTT: 20 * time.Millisecond}, Warn},
		// Slow but working is normal on satellite and rural links — worth
		// saying, not worth calling broken.
		{"high latency", PingResult{Sent: 5, Received: 5, AvgRTT: 400 * time.Millisecond}, Warn},
	}
	for _, tt := range tests {
		if got := internetVerdict(tt.p, "1.1.1.1").Severity; got != tt.want {
			t.Errorf("%s: severity = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestClockVerdict(t *testing.T) {
	tests := []struct {
		name string
		skew time.Duration
		want Severity
	}{
		{"accurate", 200 * time.Millisecond, OK},
		{"a few seconds", 3 * time.Second, OK},
		{"drifting", 5 * time.Minute, Fail},
		{"a minute out", 90 * time.Second, Warn},
		{"badly wrong", 3 * time.Hour, Fail},
		// Direction must not change severity — a clock an hour behind breaks
		// exactly as much as one an hour ahead.
		{"behind", -3 * time.Hour, Fail},
		{"slightly behind", -90 * time.Second, Warn},
	}
	for _, tt := range tests {
		if got := clockVerdict(tt.skew).Severity; got != tt.want {
			t.Errorf("%s: clockVerdict(%v) = %v, want %v", tt.name, tt.skew, got, tt.want)
		}
	}
}

func TestClockVerdictNamesTheConsequence(t *testing.T) {
	// A wrong clock presents to the user as "the internet is broken", so the
	// report has to make that connection for them.
	res := clockVerdict(2 * time.Hour)
	if !strings.Contains(res.Detail, "HTTPS") {
		t.Errorf("detail = %q, want it to explain the HTTPS consequence", res.Detail)
	}
	if !strings.Contains(clockVerdict(2*time.Hour).Detail, "ahead") {
		t.Error("expected the direction of the skew to be stated")
	}
	if !strings.Contains(clockVerdict(-2*time.Hour).Detail, "behind") {
		t.Error("expected a negative skew to read as behind")
	}
}

func TestResolverVerdict(t *testing.T) {
	working := dnsProbe{Server: netip.MustParseAddr("192.168.1.1"), Addrs: []string{"1.2.3.4"}, Elapsed: 20 * time.Millisecond}
	slow := dnsProbe{Server: netip.MustParseAddr("192.168.1.1"), Addrs: []string{"1.2.3.4"}, Elapsed: 900 * time.Millisecond}
	dead := dnsProbe{Server: netip.MustParseAddr("192.168.1.9"), Err: errPingUnavailable}

	tests := []struct {
		name   string
		probes []dnsProbe
		want   Severity
	}{
		{"all healthy", []dnsProbe{working}, OK},
		{"every resolver dead", []dnsProbe{dead}, Fail},
		// A partly-dead set still resolves, but every lookup stalls on the
		// dead server first — which is why it's worth flagging.
		{"one of two dead", []dnsProbe{working, dead}, Warn},
		{"slow but working", []dnsProbe{slow}, Warn},
	}
	for _, tt := range tests {
		res := resolverVerdict(tt.probes)
		if res.Severity != tt.want {
			t.Errorf("%s: severity = %v, want %v (%s)", tt.name, res.Severity, tt.want, res.Detail)
		}
		// Every resolver's state belongs in the report data, working or not.
		if len(res.Data) != len(tt.probes) {
			t.Errorf("%s: Data has %d entries, want %d", tt.name, len(res.Data), len(tt.probes))
		}
	}
}

func TestParseCloudflareTrace(t *testing.T) {
	const body = `fl=123abc
h=www.cloudflare.com
ip=203.0.113.9
ts=1755100000.123
visit_scheme=https
uag=curl/8.5.0
colo=DFW
loc=US
tls=TLSv1.3`

	got := parseCloudflareTrace(body)
	if got["ip"] != "203.0.113.9" {
		t.Errorf("ip = %q, want 203.0.113.9", got["ip"])
	}
	if got["loc"] != "US" {
		t.Errorf("loc = %q, want US", got["loc"])
	}
}

func TestRound(t *testing.T) {
	tests := map[time.Duration]string{
		0:                        "0ms",
		500 * time.Microsecond:   "0.5ms",
		16635 * time.Microsecond: "17ms",
		2 * time.Second:          "2s",
	}
	for in, want := range tests {
		if got := round(in); got != want {
			t.Errorf("round(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestNetReason(t *testing.T) {
	// Go wraps network errors several layers deep and puts the actual cause
	// last. A report handed to someone else should carry the cause, not the
	// wrapping.
	tests := []struct {
		err  error
		want string
	}{
		{&net.OpError{Op: "dial", Net: "udp", Err: errors.New("connect: network is unreachable")}, "no route to the network"},
		{&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}, "the connection was refused"},
		{&net.DNSError{Name: "example.invalid", IsNotFound: true}, "the name does not resolve"},
		{&net.DNSError{Name: "example.com", Err: "server misbehaving"}, "the name could not be resolved"},
		{errors.New("context deadline exceeded"), "the connection timed out"},
		{nil, ""},
	}
	for _, tt := range tests {
		if got := netReason(tt.err); got != tt.want {
			t.Errorf("netReason(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}
