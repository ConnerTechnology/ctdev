package diagnose

import (
	"fmt"
	"slices"
	"strconv"
	"time"
)

// Diagnose turns a set of check results into verdicts.
//
// This is the part that earns the command. A column of red marks tells you
// twelve things are wrong; a verdict tells you the one thing that is wrong and
// the eleven things that are downstream of it. "The router is fine, the line to
// the ISP is down" is not visible in any single check — it only exists in the
// combination.
//
// It is pure by design: map in, slice out, no I/O. Every row of the table below
// has a test.
func Diagnose(results map[string]Result, f Facts) []Finding {
	rs := resultSet(results)
	var findings []Finding

	// The network story has exactly one root cause, and everything downstream
	// of it is noise. Emit only the first layer that's broken.
	if fd := networkVerdict(rs, f); fd != nil {
		findings = append(findings, *fd)
	}

	// Hardware and system faults are independent of each other and of the
	// network, so these accumulate.
	findings = append(findings, hardwareVerdicts(rs)...)

	slices.SortStableFunc(findings, func(a, b Finding) int {
		return a.Severity.Rank() - b.Severity.Rank()
	})
	return findings
}

// resultSet gives the rules a vocabulary that treats a missing check as
// "unknown" rather than as pass or fail — a check that never ran must not be
// evidence for anything.
type resultSet map[string]Result

func (rs resultSet) severity(id string) (Severity, bool) {
	res, present := rs[id]
	if !present {
		return Skipped, false
	}
	return res.Severity, true
}

// broken reports a check that actively found a problem.
func (rs resultSet) broken(id string) bool {
	s, present := rs.severity(id)
	return present && (s == Fail || s == Warn)
}

// failed is the stricter form: outright broken, not merely degraded.
func (rs resultSet) failed(id string) bool {
	s, present := rs.severity(id)
	return present && s == Fail
}

// healthy reports a check that ran and found nothing wrong. A skipped check is
// never healthy — that distinction is what stops the engine from concluding
// things it couldn't actually observe.
func (rs resultSet) healthy(id string) bool {
	s, present := rs.severity(id)
	return present && (s == OK || s == Info)
}

func (rs resultSet) data(id, key string) string {
	res, present := rs[id]
	if !present || res.Data == nil {
		return ""
	}
	return res.Data[key]
}

func (rs resultSet) detail(id string) string { return rs[id].Detail }

// networkVerdict walks the stack from the bottom up and returns the first
// layer that's broken. Order is the whole design: a machine with no IP address
// also fails DNS, HTTPS, and every reachability probe, and reporting five
// findings for one unplugged cable would be worse than reporting none.
func networkVerdict(rs resultSet, f Facts) *Finding {
	switch {
	// Layer 1: is there an address at all?
	case rs.failed("link.ip") && !f.LocalIP.IsValid():
		return &Finding{
			Severity: Fail,
			Title:    "This machine isn't on a network at all.",
			Detail:   "No usable address on any interface, so nothing else could be tested.",
			Action:   "Check that Wi-Fi is switched on, or that the cable is seated at both ends.",
			Because:  []string{"link.ip"},
		}

	case LinkLocalIPv4(f.LocalIP):
		return &Finding{
			Severity: Fail,
			Title:    "The router isn't handing out addresses.",
			Detail: fmt.Sprintf("This machine gave up waiting for DHCP and assigned itself %s, "+
				"which can reach nothing beyond the local segment.", f.LocalIP),
			Action:  "Reboot the router. If other devices work, forget this network on this machine and rejoin it.",
			Because: []string{"link.ip"},
		}

	// Layer 2: can we reach the router?
	case rs.failed("link.gateway") && f.IsWiFi && rs.broken("link.wifi"):
		return &Finding{
			Severity: Fail,
			Title:    "Connected to Wi-Fi, but too far from the access point to use it.",
			Detail: fmt.Sprintf("The signal is %s and the router doesn't answer at all. "+
				"Being associated to a network is not the same as being able to reach it.", signalText(rs)),
			Action:  "Move closer to the access point, or plug in with a cable to confirm the rest of the network is fine.",
			Because: []string{"link.gateway", "link.wifi"},
		}

	case rs.failed("link.gateway"):
		return &Finding{
			Severity: Fail,
			Title:    "The router isn't responding.",
			Detail:   "This machine has an address but gets no reply from the gateway, so the fault is between here and the router.",
			Action:   wiredOrWirelessAdvice(f),
			Because:  []string{"link.gateway"},
		}

	// Layer 3: something between here and the internet is intercepting.
	case rs.data("net.captive", DataCaptivePortal) == "yes":
		return &Finding{
			Severity: Fail,
			Title:    "This network wants you to sign in through a browser first.",
			Detail:   "The connectivity probe was redirected to a sign-in page, which is how hotel, café, and guest Wi-Fi gate access.",
			Action:   "Open any http:// page in a browser, complete the sign-in, then run this again.",
			Because:  []string{"net.captive"},
		}

	// A wrong clock breaks TLS everywhere and presents as a dead internet.
	case clockBreaksTLS(rs):
		return &Finding{
			Severity: Fail,
			Title:    "The clock is wrong, which breaks every secure website.",
			Detail: fmt.Sprintf("The system clock is off by %s. Certificates are only valid between two dates, "+
				"so a machine that disagrees about the date rejects all of them.", skewText(rs)),
			Action:  "Turn on automatic date and time. Everything else will start working once the clock is right.",
			Because: []string{"sys.clock", "net.https"},
		}

	// Layer 4: the router answers but the world doesn't.
	case rs.healthy("link.gateway") && rs.failed("net.icmp"):
		return &Finding{
			Severity: Fail,
			Title:    "The router is fine; the connection to the internet is down.",
			Detail:   "The gateway answers normally, but nothing past it does. That places the fault at the modem or the ISP, not inside the house.",
			Action:   "Power-cycle the modem and wait two minutes. If it doesn't come back, this is a call to the ISP.",
			Because:  []string{"link.gateway", "net.icmp"},
		}

	// Layer 5: the route is fine, name resolution isn't.
	case rs.failed("dns.resolvers") && rs.healthy("dns.public"):
		return &Finding{
			Severity: Fail,
			Title:    "This network's DNS server is down.",
			Detail: "Public resolvers answer normally, but the ones this network hands out don't. " +
				"The connection is fine — nothing can look up a name.",
			Action:  "Set DNS to 1.1.1.1 to get working immediately. If this network runs a Pi-hole, check that it's still running.",
			Because: []string{"dns.resolvers", "dns.public"},
		}

	case rs.failed("dns.resolvers") && rs.healthy("net.icmp"):
		return &Finding{
			Severity: Fail,
			Title:    "Name resolution is broken, but the connection itself is fine.",
			Detail: "Addresses are reachable directly while no name resolves, and outbound DNS to public " +
				"resolvers is blocked too — so there's no working resolver to fall back to.",
			Action:  "Reconnect to the network to pick up fresh DNS settings. If that fails, the network's resolver needs attention.",
			Because: []string{"dns.resolvers", "dns.public", "net.icmp"},
		}

	// The per-server checks pass but the system still can't resolve: the
	// fault is this machine's own resolver plumbing, not the network's.
	case rs.failed("dns.system") && rs.healthy("dns.resolvers"):
		return &Finding{
			Severity: Fail,
			Title:    "This machine's own DNS service is broken.",
			Detail: "The configured DNS servers answer when asked directly, but the system resolver can't. " +
				"That points at systemd-resolved, an /etc/hosts override, or a VPN client that didn't clean up.",
			Action:  "Restart the resolver service (on Linux: systemctl restart systemd-resolved), then try again.",
			Because: []string{"dns.system", "dns.resolvers"},
		}

	case rs.failed("net.ipv6"):
		return &Finding{
			Severity: Warn,
			Title:    "Broken IPv6 is making pages slow to load.",
			Detail: "This machine has an IPv6 address, so it tries IPv6 first for every connection — but the IPv6 path " +
				"doesn't work. Each request stalls until it gives up and falls back, with no error to show for it.",
			Action:  "Disable IPv6 on this machine or at the router until the ISP fixes it.",
			Because: []string{"net.ipv6"},
		}
	}
	return nil
}

// clockBreaksTLS is the specific combination worth calling out: a badly wrong
// clock *and* HTTPS actually failing. Skew alone is worth a warning row, not a
// headline verdict.
func clockBreaksTLS(rs resultSet) bool {
	return rs.failed("sys.clock") && rs.broken("net.https")
}

func skewText(rs resultSet) string {
	secs := rs.data("sys.clock", DataClockSkewSec)
	n, err := strconv.Atoi(secs)
	if err != nil {
		return "more than five minutes"
	}
	if n < 0 {
		n = -n
	}
	return humanDuration(durationSeconds(n))
}

func signalText(rs resultSet) string {
	if dbm := rs.data("link.wifi", DataRSSI); dbm != "" {
		return dbm + " dBm"
	}
	return "weak"
}

func wiredOrWirelessAdvice(f Facts) string {
	if f.IsWiFi {
		return "Move closer to the access point. If other devices reach the router from here, this machine's Wi-Fi is the problem."
	}
	return "Check both ends of the cable and try a different port on the router."
}

// hardwareVerdicts are independent of each other: a failing drive and a full
// disk are two separate problems that happen to be on the same machine.
func hardwareVerdicts(rs resultSet) []Finding {
	var out []Finding

	if rs.failed("hw.smart") {
		out = append(out, Finding{
			Severity: Fail,
			Title:    "A drive is failing.",
			Detail:   "The drive's own health self-assessment reports a failure. This is the drive saying it is dying, not a prediction.",
			Action:   "Back up everything important now, before replacing it. Drives in this state can go from working to unreadable without further warning.",
			Because:  []string{"hw.smart"},
		})
	}

	if rs.failed("hw.disk") {
		out = append(out, Finding{
			Severity: Fail,
			Title:    "The disk is full.",
			Detail:   rs.detail("hw.disk") + ". At this level updates fail, applications won't start, and some desktops won't even log in.",
			Action:   "Empty the trash and the Downloads folder, then run 'ctdev cleanup' for the rest.",
			Because:  []string{"hw.disk"},
		})
	}

	// Out of memory is one story told by two checks: what's happening now,
	// and what already got killed because of it.
	if rs.broken("hw.memory") && rs.broken("sys.oom") {
		out = append(out, Finding{
			Severity: Fail,
			Title:    "The machine is running out of memory.",
			Detail: "Memory is exhausted and the kernel has already killed programs to reclaim some. " +
				"Anything killed this way vanishes without an error message, which is why it gets reported as \"apps just close\".",
			Action:  "Close the heaviest applications, or add memory. A reboot clears it temporarily.",
			Because: []string{"hw.memory", "sys.oom"},
		})
	}

	// "My computer got slow" is a thermal problem far more often than anyone
	// expects, and the machine never says so.
	if rs.broken("hw.temp") && rs.broken("hw.load") {
		out = append(out, Finding{
			Severity: Warn,
			Title:    "The CPU is overheating and slowing itself down.",
			Detail: fmt.Sprintf("%s while under sustained load. Silicon protects itself by cutting its own clock speed, "+
				"so the machine gets slower rather than failing outright.", rs.detail("hw.temp")),
			Action:  "Clean the fans and vents. On a laptop, check it isn't sitting on something soft that blocks the intake.",
			Because: []string{"hw.temp", "hw.load"},
		})
	}

	if rs.failed("hw.raid") {
		out = append(out, Finding{
			Severity: Fail,
			Title:    "A RAID array is running without redundancy.",
			Detail:   rs.detail("hw.raid") + ". It still works perfectly, and will keep doing so right up until the remaining disk fails.",
			Action:   "Replace the failed disk and let the array rebuild. Don't wait for a second failure.",
			Because:  []string{"hw.raid"},
		})
	}

	return out
}

func durationSeconds(n int) time.Duration { return time.Duration(n) * time.Second }
