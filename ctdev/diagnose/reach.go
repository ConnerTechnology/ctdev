package diagnose

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Ping thresholds. A gateway sits one hop away on the local segment, so the
// bar is much higher than for anything out on the internet: a wired gateway
// answering in 40ms is not a healthy wired gateway.
const (
	pingCount = 5
	// lossWarn is where intermittent loss starts costing real usability —
	// video calls stutter and TCP throughput collapses well before 100%.
	lossWarn = 5.0
	// gatewayRTTWiredWarn / gatewayRTTWiFiWarn: Wi-Fi legitimately costs a few
	// milliseconds more, and much more than this means airtime contention.
	gatewayRTTWiredWarn = 10 * time.Millisecond
	gatewayRTTWiFiWarn  = 50 * time.Millisecond
	// internetRTTWarn is generous: satellite and rural links are slow but fine.
	internetRTTWarn = 150 * time.Millisecond
)

// publicIPs are probed by raw address so the result is independent of DNS —
// that separation is the whole point of running both checks.
var publicIPs = []string{"1.1.1.1", "8.8.8.8"}

func checkGateway(ctx context.Context, f Facts) Result {
	if !f.Gateway.IsValid() {
		return failf("Reconnect to the network. If it doesn't come back, reboot the router.",
			"no default route — this machine doesn't know where to send traffic")
	}
	p, err := ping(ctx, f.Platform, f.Gateway.String(), pingCount)
	if err != nil {
		return skipf("no usable ping on this machine")
	}
	return gatewayVerdict(p, f.Gateway.String(), f.IsWiFi)
}

// gatewayVerdict is where the diagnosis forks. If the router answers, whatever
// is broken is upstream of it; if it doesn't, the problem is between this
// machine and the router — which on Wi-Fi usually means signal.
func gatewayVerdict(p PingResult, gw string, isWiFi bool) Result {
	rttWarn := gatewayRTTWiredWarn
	if isWiFi {
		rttWarn = gatewayRTTWiFiWarn
	}

	switch {
	case p.Lost() && isWiFi:
		return failf("Move closer to the router, or plug in with a cable to confirm. If other devices reach it, this machine's Wi-Fi is the problem.",
			"router at %s is not answering — %d/%d packets lost", gw, p.Sent, p.Sent)

	case p.Lost():
		return failf("Check both ends of the cable and try a different port on the router.",
			"router at %s is not answering — %d/%d packets lost", gw, p.Sent, p.Sent)

	case p.LossPercent >= lossWarn:
		return warnf("On Wi-Fi this is usually interference or distance. On a cable it's usually the cable.",
			"router at %s is dropping %.0f%% of packets", gw, p.LossPercent)

	case p.AvgRTT > rttWarn:
		return warnf("Something is saturating the local network, or the Wi-Fi channel is crowded.",
			"router at %s is slow to answer — %s average", gw, round(p.AvgRTT))

	default:
		return okf("%s answering in %s", gw, round(p.AvgRTT))
	}
}

func checkInternetICMP(ctx context.Context, f Facts) Result {
	var best PingResult
	var reached string
	for _, host := range publicIPs {
		p, err := ping(ctx, f.Platform, host, pingCount)
		if err != nil {
			return skipf("no usable ping on this machine")
		}
		if !p.Lost() && (reached == "" || p.AvgRTT < best.AvgRTT) {
			best, reached = p, host
		}
	}
	if reached == "" {
		return failf("If the router itself answers, the problem is upstream — power-cycle the modem, then call the ISP.",
			"cannot reach %s by address — this is past the router", strings.Join(publicIPs, " or "))
	}
	return internetVerdict(best, reached)
}

func internetVerdict(p PingResult, host string) Result {
	switch {
	case p.LossPercent >= lossWarn:
		return warnf("Intermittent loss on the way out. Worth a modem reboot, and worth telling the ISP if it persists.",
			"%s reachable but dropping %.0f%% of packets", host, p.LossPercent)

	case p.AvgRTT > internetRTTWarn:
		return warnf("Fine for browsing, poor for calls and games. Normal on satellite or a congested link.",
			"%s reachable but slow — %s average, %s jitter", host, round(p.AvgRTT), round(p.Jitter))

	default:
		return okf("%s in %s", host, round(p.AvgRTT))
	}
}

// checkHTTPS separates "the connection is broken" from "the certificate is
// broken", because the second is almost always a wrong clock or an
// intercepting proxy — and reads to the user as "the internet is down".
func checkHTTPS(ctx context.Context, _ Facts) Result {
	const target = "https://www.cloudflare.com"

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return skipf("could not build the probe request: %v", err)
	}
	resp, err := probeClient().Do(req)
	if err == nil {
		resp.Body.Close()
		return okf("TLS to cloudflare.com verified")
	}

	var unknownAuthority x509.UnknownAuthorityError
	var invalidCert x509.CertificateInvalidError
	var hostnameErr x509.HostnameError

	switch {
	case errors.As(err, &invalidCert) && invalidCert.Reason == x509.Expired:
		// Nothing expires a valid Cloudflare certificate except our own clock.
		return failf("Fix the system clock — check the date and time settings and re-enable automatic time.",
			"certificate rejected as expired, which almost always means this machine's clock is wrong")

	case errors.As(err, &unknownAuthority):
		return failf("Expected on a corporate or school network that inspects traffic. Anywhere else, treat it as untrusted.",
			"certificate signed by an unknown authority — something is intercepting HTTPS")

	case errors.As(err, &hostnameErr):
		return failf("Something is impersonating the site. Don't enter passwords on this network.",
			"certificate is for the wrong host — %v", hostnameErr)

	default:
		return failf("If plain HTTP works but this doesn't, a firewall is blocking port 443.",
			"cannot complete an HTTPS connection — %s", netReason(err))
	}
}

// cloudflareTrace is plain text (key=value per line) and served by the same
// anycast edge as the rest of Cloudflare, so it answers from wherever we are.
const cloudflareTrace = "https://www.cloudflare.com/cdn-cgi/trace"

func checkPublicIP(ctx context.Context, f Facts) Result {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cloudflareTrace, nil)
	if err != nil {
		return skipf("could not build the probe request: %v", err)
	}
	resp, err := probeClient().Do(req)
	if err != nil {
		return skipf("could not look up the public address — %s", netReason(err))
	}
	defer resp.Body.Close()

	buf := make([]byte, 1024)
	n, _ := resp.Body.Read(buf)
	trace := parseCloudflareTrace(string(buf[:n]))

	ip := trace["ip"]
	if ip == "" {
		return skipf("no address in the lookup response")
	}

	data := map[string]string{"public_ip": ip}
	if loc := trace["loc"]; loc != "" {
		data["country"] = loc
	}

	// A CGNAT lease means the ISP is sharing one public address across many
	// customers. Everything browsing-shaped works; inbound connections,
	// port forwarding, and some VPNs do not.
	if IsCGNAT(f.LocalIP) {
		res := warnf("Ask the ISP for a real public address if you need port forwarding or to host anything.",
			"public address %s, but this machine holds a carrier-grade NAT lease (%s)", ip, f.LocalIP)
		res.Data = data
		return res
	}

	res := okf("%s%s", ip, countrySuffix(trace["loc"]))
	res.Data = data
	return res
}

func countrySuffix(loc string) string {
	if loc == "" {
		return ""
	}
	return " (" + loc + ")"
}

// parseCloudflareTrace reads the key=value lines the trace endpoint returns.
func parseCloudflareTrace(body string) map[string]string {
	out := make(map[string]string)
	for _, line := range lines(body) {
		if k, v, found := strings.Cut(line, "="); found {
			out[k] = v
		}
	}
	return out
}

func checkIPv6(ctx context.Context, _ Facts) Result {
	if !hasGlobalIPv6() {
		// The overwhelmingly common case, and not a problem.
		return infof("not configured on this network")
	}

	// A machine with a global IPv6 address will *prefer* it. When the address
	// exists but the path doesn't, every connection stalls on IPv6 first and
	// falls back — which the user experiences as pages that take forever to
	// start loading, with no error to show for it.
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp6", "[2606:4700:4700::1111]:443")
	if err != nil {
		return failf("Turn IPv6 off on this machine or on the router until the ISP fixes it — pages will load normally again.",
			"this machine has an IPv6 address but cannot reach the IPv6 internet, so connections stall before falling back")
	}
	conn.Close()
	return okf("reachable")
}

func hasGlobalIPv6() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.To4() != nil {
			continue
		}
		if ipnet.IP.IsGlobalUnicast() && !ipnet.IP.IsPrivate() {
			return true
		}
	}
	return false
}

// netReason condenses a Go network error to the part a person can act on.
// Raw errors read like "Head \"https://host\": dial tcp: lookup host on
// 10.2.2.144:53: dial udp 10.2.2.144:53: connect: network is unreachable",
// which buries the one useful word at the end and makes a shared report look
// like a stack trace.
func netReason(err error) string {
	if err == nil {
		return ""
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			return "the name does not resolve"
		}
		return "the name could not be resolved"
	}

	s := err.Error()
	switch {
	case strings.Contains(s, "network is unreachable"):
		return "no route to the network"
	case strings.Contains(s, "no route to host"):
		return "no route to that host"
	case strings.Contains(s, "connection refused"):
		return "the connection was refused"
	case strings.Contains(s, "connection reset"):
		return "the connection was reset"
	case strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded"):
		return "the connection timed out"
	}

	// Anything unrecognized keeps its innermost clause, which is where Go
	// puts the actual cause.
	if i := strings.LastIndex(s, ": "); i >= 0 {
		return s[i+2:]
	}
	return s
}

// round trims a duration to something a person would say out loud.
func round(d time.Duration) string {
	switch {
	case d == 0:
		return "0ms"
	case d < time.Millisecond:
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	default:
		return d.Round(time.Millisecond).String()
	}
}
