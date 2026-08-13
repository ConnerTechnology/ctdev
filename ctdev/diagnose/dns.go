package diagnose

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"slices"
	"strings"
	"time"
)

const (
	// dnsProbeName is resolved against every configured resolver. It has to be
	// a name that always exists and is unlikely to be cached to death.
	dnsProbeName = "cloudflare.com"
	// dnsNXName should never resolve. If it does, something is answering for
	// names it has no business answering for.
	dnsNXName = "ctdev-should-not-exist.invalid"

	dnsTimeout = 3 * time.Second
	// dnsSlowWarn is where a resolver starts being noticeable. Every page load
	// pays this before a single byte moves.
	dnsSlowWarn = 250 * time.Millisecond
)

// publicResolvers are the control group: if these answer and the configured
// ones don't, the fault is the resolver, not the connection.
var publicResolvers = []string{"1.1.1.1", "8.8.8.8"}

// dnsProbe is one resolver's answer.
type dnsProbe struct {
	Server  netip.Addr
	Elapsed time.Duration
	Addrs   []string
	Err     error
}

func (p dnsProbe) OK() bool { return p.Err == nil && len(p.Addrs) > 0 }

// queryResolver asks one specific server, bypassing the system resolver
// entirely. Asking the system would tell us only whether the whole stack
// works; asking each server tells us which one is broken.
func queryResolver(ctx context.Context, server netip.Addr, name string) dnsProbe {
	p := dnsProbe{Server: server}
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, net.JoinHostPort(server.String(), "53"))
		},
	}

	ctx, cancel := context.WithTimeout(ctx, dnsTimeout)
	defer cancel()

	start := time.Now()
	p.Addrs, p.Err = r.LookupHost(ctx, name)
	p.Elapsed = time.Since(start)
	return p
}

// checkResolvers probes each configured resolver individually. A resolver
// that's configured but dead is one of the most common causes of "the internet
// is broken" — a stale DHCP lease, or a Pi-hole that stopped.
func checkResolvers(ctx context.Context, f Facts) Result {
	if len(f.DNS) == 0 {
		return fail("Reconnect to the network so it hands out DNS servers again, or set 1.1.1.1 manually.",
			"no DNS servers configured — nothing will resolve by name")
	}

	probes := make([]dnsProbe, len(f.DNS))
	for i, server := range f.DNS {
		probes[i] = queryResolver(ctx, server, dnsProbeName)
	}
	return resolverVerdict(probes)
}

func resolverVerdict(probes []dnsProbe) Result {
	var working, broken []dnsProbe
	for _, p := range probes {
		if p.OK() {
			working = append(working, p)
		} else {
			broken = append(broken, p)
		}
	}

	data := map[string]string{}
	for _, p := range probes {
		if p.OK() {
			data[p.Server.String()] = round(p.Elapsed)
		} else {
			data[p.Server.String()] = "no answer"
		}
	}

	switch {
	case len(working) == 0:
		res := fail("Set DNS to 1.1.1.1 to confirm. If that fixes it, the network's DNS server is down — on a Pi-hole, check the container is running.",
			"no configured resolver answered (%s)", serverList(broken))
		res.Data = data
		return res

	case len(broken) > 0:
		// Partial failure is worth flagging precisely because it mostly
		// works: lookups intermittently stall on the dead server first.
		res := warn("Remove the dead server from the network's DHCP settings — lookups stall on it before falling back.",
			"%s not answering; %s working", serverList(broken), serverList(working))
		res.Data = data
		return res
	}

	slowest := working[0]
	for _, p := range working {
		if p.Elapsed > slowest.Elapsed {
			slowest = p
		}
	}
	if slowest.Elapsed > dnsSlowWarn {
		res := warn("Every page load waits on this before anything else happens. Consider 1.1.1.1 or 9.9.9.9.",
			"%s is slow to answer — %s", slowest.Server, round(slowest.Elapsed))
		res.Data = data
		return res
	}

	res := ok("%s answering (%s)", serverList(working), round(slowest.Elapsed))
	res.Data = data
	return res
}

func serverList(probes []dnsProbe) string {
	names := make([]string, 0, len(probes))
	for _, p := range probes {
		names = append(names, p.Server.String())
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// checkPublicDNS is the control group for checkResolvers. Read together, the
// two say whether to blame the resolver or the connection.
func checkPublicDNS(ctx context.Context, _ Facts) Result {
	for _, s := range publicResolvers {
		server := netip.MustParseAddr(s)
		if p := queryResolver(ctx, server, dnsProbeName); p.OK() {
			return ok("%s answering (%s)", server, round(p.Elapsed))
		}
	}
	// Not a fault on its own. Plenty of networks block outbound 53 on purpose
	// — a Pi-hole or firewall forcing every client through the local resolver
	// looks exactly like this, and so does a hotel captive network. It only
	// becomes a problem in combination with a failing local resolver, which is
	// a judgement for the correlation pass, not for this check.
	return info("blocked or unreachable (%s) — normal when a Pi-hole or firewall forces local DNS",
		strings.Join(publicResolvers, ", "))
}

// checkDNSHijack looks for a resolver that invents answers for names that
// don't exist. ISPs do it to serve search pages, captive portals do it to
// force you to their splash, and both break anything that relies on a real
// NXDOMAIN.
func checkDNSHijack(ctx context.Context, f Facts) Result {
	if len(f.DNS) == 0 {
		return skip("no resolver to test")
	}
	p := queryResolver(ctx, f.DNS[0], dnsNXName)
	if p.OK() {
		return warn("Usually an ISP search page or a captive portal. It breaks VPN split-DNS and some installers.",
			"%s invents an answer (%s) for a name that doesn't exist", f.DNS[0], strings.Join(p.Addrs, ", "))
	}

	// A lookup that failed is not the same as a lookup that correctly said
	// "no such name". Without this distinction a machine with no network at
	// all reports healthy DNS, which is worse than reporting nothing.
	var dnsErr *net.DNSError
	if errors.As(p.Err, &dnsErr) && dnsErr.IsNotFound {
		return ok("names that don't exist correctly return no answer")
	}
	return skip("could not reach %s to test", f.DNS[0])
}

// checkSystemDNS resolves through the system stack rather than a specific
// server. It catches the layer the per-server checks skip over: a broken
// /etc/hosts, an nsswitch misconfiguration, or a stub resolver that's down
// while the upstreams it points at are fine.
func checkSystemDNS(ctx context.Context, _ Facts) Result {
	ctx, cancel := context.WithTimeout(ctx, dnsTimeout)
	defer cancel()

	start := time.Now()
	addrs, err := net.DefaultResolver.LookupHost(ctx, dnsProbeName)
	elapsed := time.Since(start)

	if err != nil {
		return fail("If individual DNS servers answer but this doesn't, the resolver service on this machine is the problem (systemd-resolved, or a hosts-file override).",
			"the system cannot resolve %s — %s", dnsProbeName, netReason(err))
	}
	return ok("%s resolves in %s (%d addresses)", dnsProbeName, round(elapsed), len(addrs))
}
