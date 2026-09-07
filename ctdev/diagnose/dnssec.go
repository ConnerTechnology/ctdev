package diagnose

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/netip"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// Two checks the standard library's resolver cannot express: net.Resolver
// hides the response flags, and these checks are entirely about the flags.
// So the query goes over the wire by hand — a header, one question, and for
// DNSSEC an OPT record asking for signatures — and only the header of the
// reply is read. Nothing here parses answer records; the verdict never needs
// them.

const (
	// dnssecBogusName is signed with a deliberately broken chain and kept that
	// way on purpose (Comcast runs it for exactly this test). A validating
	// resolver must refuse it; one that answers isn't validating.
	dnssecBogusName = "dnssec-failed.org"
	// dnssecGoodName is the control: signed, and must resolve, or the bogus
	// name's failure could just be the network.
	dnssecGoodName = dnsProbeName

	// rootServerA and rootServerC are two of the thirteen root servers, asked
	// directly. A transparent DNS proxy answers in their place, and the
	// difference shows in one bit: only a real root server sets AA.
	rootServerA = "198.41.0.4"
	rootServerC = "192.33.4.12"

	dnsTypeA    uint16 = 1
	dnsTypeNS   uint16 = 2
	dnsTypeOPT  uint16 = 41
	dnsClassIN  uint16 = 1
	dnsEDNSSize uint16 = 1232 // the DNS Flag Day 2020 value Unbound and FTL use

	dnsRCodeNoError  = 0
	dnsRCodeServFail = 2
)

// dnsQuery is one question to put on the wire.
type dnsQuery struct {
	Name  string
	Type  uint16
	Class uint16
	// RD asks the server to recurse. Off when asking a root server directly,
	// which is the point of that check.
	RD bool
	// DO adds an OPT record with the DNSSEC OK bit, so a validating resolver
	// answers with AD set.
	DO bool
}

// dnsHeader is the part of a reply the checks read.
type dnsHeader struct {
	ID      uint16
	AA      bool // authoritative answer
	AD      bool // authenticated data: the resolver validated DNSSEC
	RCode   int
	Answers int
}

// dnsAnswer is a header, or the reason there isn't one.
type dnsAnswer struct {
	Header dnsHeader
	Err    error
}

func (a dnsAnswer) resolved() bool { return a.Err == nil && a.Header.RCode == dnsRCodeNoError }

// encodeDNSQuery builds the wire form of a query (RFC 1035 §4.1, RFC 6891 for
// the OPT record).
func encodeDNSQuery(id uint16, q dnsQuery) []byte {
	var flags uint16
	if q.RD {
		flags |= 1 << 8
	}
	var arcount uint16
	if q.DO {
		arcount = 1
	}
	msg := binary.BigEndian.AppendUint16(nil, id)
	msg = binary.BigEndian.AppendUint16(msg, flags)
	msg = binary.BigEndian.AppendUint16(msg, 1) // QDCOUNT
	msg = binary.BigEndian.AppendUint16(msg, 0) // ANCOUNT
	msg = binary.BigEndian.AppendUint16(msg, 0) // NSCOUNT
	msg = binary.BigEndian.AppendUint16(msg, arcount)

	for _, label := range strings.Split(strings.Trim(q.Name, "."), ".") {
		if label == "" {
			continue
		}
		msg = append(msg, byte(len(label)))
		msg = append(msg, label...)
	}
	msg = append(msg, 0) // root label
	msg = binary.BigEndian.AppendUint16(msg, q.Type)
	msg = binary.BigEndian.AppendUint16(msg, q.Class)

	if q.DO {
		msg = append(msg, 0) // OPT owner is the root name
		msg = binary.BigEndian.AppendUint16(msg, dnsTypeOPT)
		msg = binary.BigEndian.AppendUint16(msg, dnsEDNSSize) // CLASS carries the UDP payload size
		msg = append(msg, 0, 0)                               // extended RCODE, EDNS version
		msg = binary.BigEndian.AppendUint16(msg, 1<<15)       // DO bit
		msg = binary.BigEndian.AppendUint16(msg, 0)           // RDLENGTH
	}
	return msg
}

// parseDNSResponse reads the header of a reply to the query with the given ID.
func parseDNSResponse(id uint16, msg []byte) (dnsHeader, error) {
	if len(msg) < 12 {
		return dnsHeader{}, fmt.Errorf("short DNS message (%d bytes)", len(msg))
	}
	h := dnsHeader{ID: binary.BigEndian.Uint16(msg[0:2])}
	if h.ID != id {
		return dnsHeader{}, fmt.Errorf("DNS reply for another query (id %#x, want %#x)", h.ID, id)
	}
	flags := binary.BigEndian.Uint16(msg[2:4])
	if flags&(1<<15) == 0 {
		return dnsHeader{}, errors.New("DNS message is a query, not a response")
	}
	h.AA = flags&(1<<10) != 0
	h.AD = flags&(1<<5) != 0
	h.RCode = int(flags & 0xf)
	h.Answers = int(binary.BigEndian.Uint16(msg[6:8]))
	return h, nil
}

// askDNS sends one query over UDP and reads the reply header. Truncated
// replies are fine: the flags are in the first twelve bytes regardless.
func askDNS(ctx context.Context, server netip.Addr, q dnsQuery) dnsAnswer {
	ctx, cancel := context.WithTimeout(ctx, dnsTimeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", net.JoinHostPort(server.String(), "53"))
	if err != nil {
		return dnsAnswer{Err: err}
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	id := uint16(rand.Uint32())
	if _, err := conn.Write(encodeDNSQuery(id, q)); err != nil {
		return dnsAnswer{Err: err}
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return dnsAnswer{Err: err}
	}
	h, err := parseDNSResponse(id, buf[:n])
	return dnsAnswer{Header: h, Err: err}
}

// hasLoopbackUpstream reports whether Pi-hole forwards to a resolver on this
// machine — Unbound at 127.0.0.1#5335 in the ctdev stack — which is the case
// where validation and root reachability are this host's responsibility.
func hasLoopbackUpstream(upstreams []string) bool {
	for _, u := range upstreams {
		host, _, _ := strings.Cut(u, "#")
		if addr, err := netip.ParseAddr(host); err == nil && addr.IsLoopback() {
			return true
		}
	}
	return false
}

func localRecursor(ctx context.Context) bool {
	if !sysutil.PiholeAvailable() {
		return false
	}
	raw, err := sysutil.PiholeCapture(ctx, "pihole-FTL", "--config", "dns.upstreams")
	if err != nil {
		return false
	}
	return hasLoopbackUpstream(parsePiholeUpstreams(raw))
}

// checkDNSSEC asks the first configured resolver for a name whose DNSSEC
// chain is deliberately broken. A validating resolver refuses it with
// SERVFAIL; a resolver that hands back an address is not validating, and
// every signature Unbound is supposed to check is being taken on trust.
//
// The verdict rests on the bogus name, not on the AD flag of the good one:
// stub resolvers in the path (systemd-resolved, MagicDNS) strip AD, but
// they pass SERVFAIL through untouched.
func checkDNSSEC(ctx context.Context, f Facts) Result {
	if len(f.DNS) == 0 {
		return skipf("no resolver to test")
	}
	server := f.DNS[0]
	good := askDNS(ctx, server, dnsQuery{Name: dnssecGoodName, Type: dnsTypeA, Class: dnsClassIN, RD: true, DO: true})
	bogus := askDNS(ctx, server, dnsQuery{Name: dnssecBogusName, Type: dnsTypeA, Class: dnsClassIN, RD: true, DO: true})
	return dnssecVerdict(good, bogus, localRecursor(ctx))
}

func dnssecVerdict(good, bogus dnsAnswer, localRecursor bool) Result {
	if !good.resolved() {
		return skipf("could not resolve %s to run the test", dnssecGoodName)
	}
	if bogus.Err != nil {
		return skipf("could not test — no reply for %s", dnssecBogusName)
	}
	data := map[string]string{
		"bogus_rcode": fmt.Sprint(bogus.Header.RCode),
		"good_ad":     fmt.Sprint(good.Header.AD),
	}

	if bogus.Header.RCode != dnsRCodeNoError {
		res := okf("resolver validates DNSSEC (refuses %s)", dnssecBogusName)
		res.Data = data
		return res
	}
	if localRecursor {
		res := warnf("Unbound should refuse this name. If it stopped validating, the usual cause is port 53 being intercepted upstream — see the root server check.",
			"resolver is not validating DNSSEC — %s resolved", dnssecBogusName)
		res.Data = data
		return res
	}
	res := infof("resolver is not validating DNSSEC — normal for many ISP resolvers")
	res.Data = data
	return res
}

// checkRootReach only matters on a node that recurses for itself. It asks a
// root server, non-recursively, for the root NS set. A root server answers
// authoritatively; a transparent proxy that grabbed the packet answers from
// its own cache and cannot set AA. That one bit is the difference between a
// resolver that walks the DNS tree and one that only thinks it does — the
// failure that quietly breaks DNSSEC on ISPs that intercept port 53.
func checkRootReach(ctx context.Context, _ Facts) Result {
	if !localRecursor(ctx) {
		return skipf("no local recursive resolver on this machine")
	}
	q := dnsQuery{Name: ".", Type: dnsTypeNS, Class: dnsClassIN}
	ans := askDNS(ctx, netip.MustParseAddr(rootServerA), q)
	if ans.Err != nil {
		// One root's outage is not the internet's; ask another before
		// blaming the path.
		ans = askDNS(ctx, netip.MustParseAddr(rootServerC), q)
	}
	return rootReachVerdict(ans)
}

func rootReachVerdict(ans dnsAnswer) Result {
	if ans.Err != nil {
		return warnf("Unbound cannot walk the DNS tree without the root servers. If the internet is otherwise fine, something is blocking outbound port 53.",
			"root servers not answering on port 53 — %s", netReason(ans.Err))
	}
	if !ans.Header.AA {
		return failf("Something between this machine and the internet is answering DNS in the root servers' place, so Unbound's recursion and DNSSEC are being answered by the interceptor. Forward Unbound over DNS-over-TLS (port 853) to a resolver such as Quad9 instead.",
			"port 53 to the root servers is being intercepted — a non-authoritative answer came back")
	}
	return okf("root servers answer directly (%d name servers)", ans.Header.Answers)
}
