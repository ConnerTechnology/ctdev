package diagnose

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestEncodeDNSQueryRecursiveWithDO(t *testing.T) {
	msg := encodeDNSQuery(0x1234, dnsQuery{Name: "dnssec-failed.org", Type: dnsTypeA, Class: dnsClassIN, RD: true, DO: true})

	header := []byte{
		0x12, 0x34, // ID
		0x01, 0x00, // flags: RD
		0x00, 0x01, // QDCOUNT
		0x00, 0x00, // ANCOUNT
		0x00, 0x00, // NSCOUNT
		0x00, 0x01, // ARCOUNT: the OPT record
	}
	if !bytes.HasPrefix(msg, header) {
		t.Fatalf("header = % x", msg[:12])
	}
	question := []byte("\x0ddnssec-failed\x03org\x00" + "\x00\x01" + "\x00\x01")
	if !bytes.Equal(msg[12:12+len(question)], question) {
		t.Errorf("question = % x", msg[12:12+len(question)])
	}
	// OPT: root name, type 41, UDP size 1232, extended RCODE 0, version 0,
	// flags with the DO bit, no data.
	opt := []byte{0x00, 0x00, 0x29, 0x04, 0xd0, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00}
	if !bytes.HasSuffix(msg, opt) {
		t.Errorf("OPT = % x", msg[len(msg)-len(opt):])
	}
}

func TestEncodeDNSQueryRootNonRecursive(t *testing.T) {
	msg := encodeDNSQuery(0x0001, dnsQuery{Name: ".", Type: dnsTypeNS, Class: dnsClassIN})
	want := []byte{
		0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00,       // root name
		0x00, 0x02, // NS
		0x00, 0x01, // IN
	}
	if !bytes.Equal(msg, want) {
		t.Errorf("msg = % x, want % x", msg, want)
	}
}

func TestParseDNSResponse(t *testing.T) {
	tests := []struct {
		name  string
		flags [2]byte
		want  dnsHeader
	}{
		{"authoritative validated answer", [2]byte{0x85, 0xa0}, dnsHeader{ID: 0x1234, AA: true, AD: true, RCode: dnsRCodeNoError}},
		{"servfail from a validating resolver", [2]byte{0x81, 0x82}, dnsHeader{ID: 0x1234, RCode: dnsRCodeServFail}},
		{"plain recursive answer", [2]byte{0x81, 0x80}, dnsHeader{ID: 0x1234, RCode: dnsRCodeNoError}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := []byte{0x12, 0x34, tt.flags[0], tt.flags[1], 0, 1, 0, 1, 0, 0, 0, 0}
			got, err := parseDNSResponse(0x1234, raw)
			if err != nil {
				t.Fatal(err)
			}
			tt.want.Answers = 1
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseDNSResponseRejectsBadMessages(t *testing.T) {
	if _, err := parseDNSResponse(1, []byte{0, 1, 0x81}); err == nil {
		t.Error("short message accepted")
	}
	// A reply carrying another query's ID is not ours, however well-formed.
	if _, err := parseDNSResponse(1, []byte{0, 2, 0x81, 0x80, 0, 1, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Error("mismatched ID accepted")
	}
	// A response that isn't one (QR clear) is a reflected query.
	if _, err := parseDNSResponse(1, []byte{0, 1, 0x01, 0x00, 0, 1, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Error("query with QR clear accepted as a response")
	}
}

func TestDNSSECVerdict(t *testing.T) {
	ok := dnsAnswer{Header: dnsHeader{RCode: dnsRCodeNoError, Answers: 2}}
	servfail := dnsAnswer{Header: dnsHeader{RCode: dnsRCodeServFail}}
	timeout := dnsAnswer{Err: errors.New("i/o timeout")}

	tests := []struct {
		name          string
		good, bogus   dnsAnswer
		localRecursor bool
		severity      Severity
		detail        string
	}{
		{"bogus name refused means validating", ok, servfail, true, OK, "validates"},
		{"bogus name answered on a recursor node is a fault", ok, ok, true, Warn, "not validating"},
		{"bogus name answered elsewhere is only informational", ok, ok, false, Info, "not validating"},
		{"good name unreachable proves nothing", timeout, servfail, true, Skipped, "could not"},
		{"bogus name unreachable proves nothing", ok, timeout, true, Skipped, "could not"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dnssecVerdict(tt.good, tt.bogus, tt.localRecursor)
			if got.Severity != tt.severity || !strings.Contains(got.Detail, tt.detail) {
				t.Errorf("got %s %q, want %s containing %q", got.Severity, got.Detail, tt.severity, tt.detail)
			}
		})
	}
}

func TestRootReachVerdict(t *testing.T) {
	direct := dnsAnswer{Header: dnsHeader{AA: true, Answers: 13}}
	proxied := dnsAnswer{Header: dnsHeader{Answers: 13}}
	dead := dnsAnswer{Err: errors.New("i/o timeout")}

	if got := rootReachVerdict(direct); got.Severity != OK {
		t.Errorf("authoritative answer: %s %q", got.Severity, got.Detail)
	}
	got := rootReachVerdict(proxied)
	if got.Severity != Fail || !strings.Contains(got.Detail, "intercept") {
		t.Errorf("non-authoritative answer: %s %q", got.Severity, got.Detail)
	}
	if !strings.Contains(got.Advice, "DNS-over-TLS") {
		t.Errorf("advice should name the workaround: %q", got.Advice)
	}
	if got := rootReachVerdict(dead); got.Severity != Warn {
		t.Errorf("no answer: %s %q", got.Severity, got.Detail)
	}
}

func TestHasLoopbackUpstream(t *testing.T) {
	if !hasLoopbackUpstream([]string{"127.0.0.1#5335"}) {
		t.Error("Unbound on loopback not recognised")
	}
	if hasLoopbackUpstream([]string{"1.1.1.1", "9.9.9.9"}) {
		t.Error("public resolvers counted as a local recursor")
	}
	if hasLoopbackUpstream(nil) {
		t.Error("no upstreams counted as a local recursor")
	}
}
