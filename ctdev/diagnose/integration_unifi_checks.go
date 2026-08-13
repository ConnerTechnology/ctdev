package diagnose

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Airtime thresholds. Channel utilization is the share of time the channel is
// busy; above roughly 60% clients start waiting to transmit, and the symptom
// is "the Wi-Fi is slow" on devices whose signal looks perfect.
const (
	airtimeWarnPct = 60
	airtimeFailPct = 80
)

// UniFi device states, from the controller's own vocabulary.
const (
	unifiStateDisconnected = 0
	unifiStateConnected    = 1
	unifiStateUpgrading    = 4
	unifiStateProvisioning = 5
	unifiStateHeartbeatGap = 6
	unifiStateIsolated     = 11
)

// unifiResults turns a snapshot into report rows. Split from collection so it
// is pure and testable against recorded controller output.
func unifiResults(snap *UnifiSnapshot) map[string]Result {
	out := map[string]Result{
		"unifi.devices": unifiDeviceVerdict(snap.Devices),
		"unifi.airtime": unifiAirtimeVerdict(snap.Devices),
		"unifi.radar":   unifiRadarVerdict(snap.Events),
		"unifi.wlan":    unifiWLANVerdict(snap.WLANs),
	}

	if health := unifiHealthVerdict(snap.Health); health.Detail != "" {
		out["unifi.health"] = health
	}

	// The certificate story rides along with the data it protected, so a
	// report never quietly implies the connection was verified when it wasn't.
	if !snap.TLSVerified && snap.CertFingerprint != "" {
		out["unifi.tls"] = infof(
			"controller certificate is self-signed (sha256 %s…) — verification was skipped, as it must be for UniFi",
			snap.CertFingerprint[:16])
	}
	return out
}

// unifiDeviceVerdict reports access points that are not simply up and wired.
//
// A wireless mesh uplink is the quiet one: it works, so nobody questions it,
// but the AP is spending half its airtime relaying its own backhaul and every
// client behind it pays for that.
func unifiDeviceVerdict(devices []UnifiDevice) Result {
	if len(devices) == 0 {
		return skipf("the controller reported no adopted devices")
	}

	var down, mesh, busy []string
	for _, d := range devices {
		name := unifiDeviceName(d)
		switch d.State {
		case unifiStateConnected:
		case unifiStateUpgrading, unifiStateProvisioning:
			busy = append(busy, name)
			continue
		default:
			down = append(down, name)
			continue
		}
		if d.Type == "uap" && d.Uplink.Type == "wireless" {
			mesh = append(mesh, name)
		}
	}

	switch {
	case len(down) > 0:
		return failf("Check power and cabling for these. Clients that were using them have moved to whatever is left in range.",
			"%d of %d devices are offline: %s", len(down), len(devices), strings.Join(down, ", "))

	case len(mesh) > 0:
		return warnf("A wireless uplink halves usable throughput and adds latency for every client behind it. Run a cable to these if you can.",
			"%s on a wireless mesh uplink", strings.Join(mesh, ", "))

	case len(busy) > 0:
		return infof("%s currently upgrading or provisioning", strings.Join(busy, ", "))

	default:
		return okf("%d devices adopted and connected", len(devices))
	}
}

func unifiDeviceName(d UnifiDevice) string {
	if d.Name != "" {
		return d.Name
	}
	if d.Model != "" {
		return d.Model + " " + d.MAC
	}
	return d.MAC
}

// unifiAirtimeVerdict finds the busiest radio. This is the measurement that
// explains slow Wi-Fi on a client showing full signal, and it exists nowhere
// on the client.
func unifiAirtimeVerdict(devices []UnifiDevice) Result {
	type radio struct {
		device string
		stat   UnifiRadioStat
	}

	var worst radio
	found := false
	for _, d := range devices {
		for _, rs := range d.RadioStats {
			if !found || rs.CuTotal > worst.stat.CuTotal {
				worst = radio{device: unifiDeviceName(d), stat: rs}
				found = true
			}
		}
	}
	if !found {
		return skipf("the controller reported no radio statistics")
	}

	s := worst.stat
	// Separating our own traffic from the rest is what distinguishes "this AP
	// is working hard" from "somebody else's network is sitting on top of us".
	interference := s.CuTotal - s.CuSelfRx - s.CuSelfTx
	detail := fmt.Sprintf("%s ch %d is %d%% busy (%d%% from neighbouring networks), %d clients",
		worst.device, s.Channel, s.CuTotal, max(interference, 0), s.NumSta)

	switch {
	case s.CuTotal >= airtimeFailPct:
		return failf(unifiAirtimeAdvice(interference, s.Channel), "%s", detail)
	case s.CuTotal >= airtimeWarnPct:
		return warnf(unifiAirtimeAdvice(interference, s.Channel), "%s", detail)
	default:
		return okf("%s", detail)
	}
}

func unifiAirtimeAdvice(interference, channel int) string {
	if interference >= 30 {
		if channel > 0 && channel <= 14 {
			return "Most of that congestion is other people's networks. 2.4 GHz is crowded almost everywhere — move what you can to 5 GHz."
		}
		return "Most of that congestion is other people's networks. Try a different channel."
	}
	return "This access point is saturating its own channel. Move some clients to another band, or add an access point."
}

// unifiRadarVerdict surfaces DFS radar evictions.
//
// This is the finding that justifies the whole integration. On a DFS channel,
// an access point that hears radar must vacate immediately — every client
// drops at once and reconnects elsewhere. From the client it looks like the
// Wi-Fi randomly died. The event only exists in the controller's log.
func unifiRadarVerdict(events []UnifiEvent) Result {
	radar := map[string]int{}
	disconnects := map[string]int{}

	for _, e := range events {
		switch {
		case strings.Contains(e.Key, "RadarDetected"):
			radar[eventTarget(e)]++
		case strings.Contains(e.Key, "AP_Lost_Contact"), strings.Contains(e.Key, "AP_Disconnected"):
			disconnects[eventTarget(e)]++
		}
	}

	if len(radar) > 0 {
		return failf("Pin those access points to non-DFS channels (36-48 or 149-165 in most regions). Radar always wins, and every client drops when it happens.",
			"%d radar detection(s) in the last 24h on %s — clients are being thrown off the channel",
			countValues(radar), strings.Join(sortedKeys(radar), ", "))
	}

	if total := countValues(disconnects); total >= 5 {
		return warnf("Repeated dropouts usually mean power (check PoE budget) or a flaky uplink, not Wi-Fi.",
			"%d access point disconnections in the last 24h on %s",
			total, strings.Join(sortedKeys(disconnects), ", "))
	}

	return okf("no radar events or repeated dropouts in the last 24h")
}

func eventTarget(e UnifiEvent) string {
	if e.AP != "" {
		return e.AP
	}
	return "an access point"
}

// unifiWLANVerdict flags configuration that causes the exact complaint people
// call about. A minimum-RSSI kick threshold is the standout: it is meant to
// push distant clients to a nearer AP, and set too aggressively it simply
// disconnects people who are working fine.
func unifiWLANVerdict(wlans []UnifiWLAN) Result {
	if len(wlans) == 0 {
		return skipf("the controller reported no wireless networks")
	}

	var aggressive []string
	for _, w := range wlans {
		if w.Enabled && w.MinRSSIEnabled && w.MinRSSI > rssiPoor {
			aggressive = append(aggressive, fmt.Sprintf("%s (kicks below %d dBm)", w.Name, w.MinRSSI))
		}
	}
	if len(aggressive) > 0 {
		return warnf("This disconnects clients on purpose. If people report random dropouts at the edge of the house, this is why — lower it or turn it off.",
			"minimum-RSSI is set aggressively on %s", strings.Join(aggressive, ", "))
	}

	enabled := 0
	for _, w := range wlans {
		if w.Enabled {
			enabled++
		}
	}
	return okf("%d wireless network(s) configured", enabled)
}

func unifiHealthVerdict(health []UnifiHealth) Result {
	for _, h := range health {
		if h.Subsystem != "wan" {
			continue
		}
		if h.Status != "ok" {
			return failf("The controller itself reports the internet connection as down. That's upstream of every device on this network.",
				"the gateway reports WAN status %q", h.Status)
		}
		if h.WANIP != "" {
			return okf("WAN up, public address %s", h.WANIP)
		}
		return okf("WAN up")
	}
	return Result{}
}

func countValues(m map[string]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// unifiChecks returns the report rows for a controller, collected once.
func unifiChecks(snap *UnifiSnapshot) []Check {
	names := map[string]string{
		"unifi.devices": "UniFi devices",
		"unifi.airtime": "Wi-Fi airtime",
		"unifi.radar":   "Radar events",
		"unifi.wlan":    "Wi-Fi settings",
		"unifi.health":  "UniFi WAN",
		"unifi.tls":     "Controller certificate",
	}

	results := unifiResults(snap)
	checks := make([]Check, 0, len(results))
	for id := range results {
		name := names[id]
		if name == "" {
			name = id
		}
		checks = append(checks, Check{ID: id, Name: name, Group: GroupNetwork, Network: true, Deep: true})
	}
	sort.Slice(checks, func(i, j int) bool { return checks[i].ID < checks[j].ID })
	return checks
}

// unifiDataFor exposes a snapshot's numbers for the Markdown report.
func unifiDataFor(snap *UnifiSnapshot) map[string]string {
	data := map[string]string{
		"devices": strconv.Itoa(len(snap.Devices)),
		"wlans":   strconv.Itoa(len(snap.WLANs)),
	}
	if snap.CertFingerprint != "" {
		data["cert_sha256"] = snap.CertFingerprint
	}
	return data
}
