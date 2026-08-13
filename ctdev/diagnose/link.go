package diagnose

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

// Wi-Fi signal thresholds in dBm. These are the numbers a wireless installer
// works to: -60 is a good link, -70 still works, -80 is where throughput
// collapses and clients start dropping off entirely.
const (
	rssiGood = -60
	rssiFair = -70
	rssiPoor = -80
)

// WiFiInfo is the current wireless association, as much of it as the platform
// will tell us without root.
type WiFiInfo struct {
	Associated bool
	SSID       string
	BSSID      string
	// RSSI is signal strength in dBm. Zero means unknown.
	RSSI int
	// RSSIApprox marks a value converted from a percentage (Windows reports
	// signal quality, not dBm), so the report can say so rather than imply a
	// precision it doesn't have.
	RSSIApprox bool
	// Noise in dBm, where the platform reports it. Zero means unknown.
	Noise   int
	Channel int
	Band    string
	TxMbps  float64
}

// SNR is the margin between signal and noise, which predicts usable throughput
// better than signal alone. Zero when we don't have a noise figure.
func (w WiFiInfo) SNR() int {
	if w.Noise == 0 || w.RSSI == 0 {
		return 0
	}
	return w.RSSI - w.Noise
}

func wifiInfo(ctx context.Context, info platform.Info, iface string) WiFiInfo {
	switch info.OS {
	case platform.Linux:
		if commandExists("iw") {
			return parseIWLink(capture(ctx, "iw", "dev", iface, "link"))
		}
	case platform.MacOS:
		return parseAirportData(capture(ctx, "system_profiler", "SPAirPortDataType"))
	case platform.Windows:
		return parseNetshWlan(capture(ctx, "netsh", "wlan", "show", "interfaces"))
	}
	return WiFiInfo{}
}

func checkWiFi(ctx context.Context, f Facts) Result {
	w := wifiInfo(ctx, f.Platform, f.Iface)
	if !w.Associated {
		return skipf("could not read the wireless association")
	}

	desc := describeWiFi(w)
	if w.RSSI == 0 {
		return infof("%s", desc)
	}

	res := wifiSignalVerdict(w.RSSI)
	res.Detail = desc
	res.Data = map[string]string{
		"ssid":    w.SSID,
		"bssid":   w.BSSID,
		"rssi":    strconv.Itoa(w.RSSI) + " dBm",
		"channel": strconv.Itoa(w.Channel),
		"band":    w.Band,
	}
	return res
}

// wifiSignalVerdict maps signal strength to a severity and the advice that
// goes with it. Split out from formatting so the thresholds are testable.
func wifiSignalVerdict(rssi int) Result {
	switch {
	case rssi >= rssiGood:
		return Result{Severity: OK}
	case rssi >= rssiFair:
		return Result{Severity: Info}
	case rssi >= rssiPoor:
		return Result{
			Severity: Warn,
			Advice:   "Usable but lossy — expect stalls on calls and large downloads. Move closer to the access point, or add one nearer to here.",
		}
	default:
		return Result{
			Severity: Fail,
			Advice:   "Too weak to work reliably. Move closer to the access point, or use a cable to confirm everything else is fine.",
		}
	}
}

func describeWiFi(w WiFiInfo) string {
	var b strings.Builder
	if w.SSID != "" {
		b.WriteString(`"` + w.SSID + `"`)
	} else {
		b.WriteString("connected")
	}
	if w.RSSI != 0 {
		fmt.Fprintf(&b, ", %d dBm", w.RSSI)
		if w.RSSIApprox {
			b.WriteString(" (approx)")
		}
		if snr := w.SNR(); snr != 0 {
			fmt.Fprintf(&b, ", %d dB SNR", snr)
		}
	}
	if w.Channel != 0 {
		fmt.Fprintf(&b, ", ch %d", w.Channel)
		if w.Band != "" {
			b.WriteString(" (" + w.Band + ")")
		}
	}
	if w.TxMbps > 0 {
		fmt.Fprintf(&b, ", %.0f Mbps", w.TxMbps)
	}
	return b.String()
}

// parseIWLink reads `iw dev <iface> link`:
//
//	Connected to 22:0b:8b:c2:7d:db (on wlp5s0)
//		SSID: Conner Trusted
//		freq: 5785.0
//		signal: -59 dBm
//		tx bitrate: 458.8 MBit/s 40MHz EHT-MCS 9 EHT-NSS 2
//
// When the radio is up but unassociated it prints "Not connected."
func parseIWLink(out string) WiFiInfo {
	var w WiFiInfo
	for _, line := range lines(out) {
		if rest, found := strings.CutPrefix(line, "Connected to "); found {
			w.Associated = true
			w.BSSID, _, _ = strings.Cut(rest, " ")
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "SSID":
			w.SSID = value
		case "freq":
			// Newer iw prints a fractional MHz ("5785.0").
			if mhz, err := strconv.ParseFloat(value, 64); err == nil {
				w.Channel, w.Band = channelFromFreq(mhz)
			}
		case "signal":
			w.RSSI = parseLeadingInt(value)
		case "tx bitrate":
			w.TxMbps = parseLeadingFloat(value)
		}
	}
	return w
}

// parseNetshWlan reads `netsh wlan show interfaces`. Windows reports signal as
// a quality percentage rather than dBm.
func parseNetshWlan(out string) WiFiInfo {
	var w WiFiInfo
	for _, line := range lines(out) {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "State":
			w.Associated = strings.EqualFold(value, "connected")
		case "SSID":
			w.SSID = value
		case "BSSID":
			w.BSSID = value
		case "Channel":
			w.Channel, _ = strconv.Atoi(value)
		case "Band":
			w.Band = normalizeBand(value)
		case "Signal":
			if pct, err := strconv.Atoi(strings.TrimSuffix(value, "%")); err == nil {
				w.RSSI = qualityToDBm(pct)
				w.RSSIApprox = true
			}
		case "Transmit rate (Mbps)":
			w.TxMbps = parseLeadingFloat(value)
		}
	}
	return w
}

// qualityToDBm converts the Windows WLAN signal-quality percentage back to
// dBm. The scale is defined as linear from -100 dBm (0%) to -50 dBm (100%),
// so this recovers the original figure to within a dB.
func qualityToDBm(pct int) int {
	if pct <= 0 {
		return -100
	}
	if pct >= 100 {
		return -50
	}
	return pct/2 - 100
}

// parseAirportData reads `system_profiler SPAirPortDataType`:
//
//	Current Network Information:
//	    MyNetwork:
//	      PHY Mode: 802.11ax
//	      Channel: 44 (5GHz, 80MHz)
//	      Signal / Noise: -58 dBm / -92 dBm
//	      Transmit Rate: 780
//
// Recent macOS withholds the BSSID unless the caller holds location
// permission, so that field is usually empty here.
func parseAirportData(out string) WiFiInfo {
	var w WiFiInfo
	inCurrent := false
	for _, line := range lines(out) {
		if strings.HasPrefix(line, "Current Network Information:") {
			inCurrent = true
			continue
		}
		if !inCurrent {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)

		// Inside the block, a heading with no value is the network name —
		// and the next one after that starts "Other Local Wi-Fi Networks",
		// so it marks the end of what we care about. SSIDs routinely contain
		// spaces and colons, so the position is the only reliable signal.
		if value == "" {
			if w.SSID != "" {
				break
			}
			w.SSID, w.Associated = key, true
			continue
		}
		switch key {
		case "Channel":
			// "44 (5GHz, 80MHz)"
			w.Channel = parseLeadingInt(value)
			if _, inner, ok := strings.Cut(value, "("); ok {
				band, _, _ := strings.Cut(inner, ",")
				w.Band = normalizeBand(band)
			}
			w.Associated = true
		case "Signal / Noise":
			// "-58 dBm / -92 dBm"
			sig, noise, _ := strings.Cut(value, "/")
			w.RSSI = parseLeadingInt(sig)
			w.Noise = parseLeadingInt(noise)
		case "Transmit Rate":
			w.TxMbps = parseLeadingFloat(value)
		}
	}
	return w
}

// channelFromFreq converts a centre frequency in MHz to its channel number and
// band. The three bands use different arithmetic, and 6 GHz overlaps 5 GHz
// numerically, so the band has to travel with the channel to mean anything.
func channelFromFreq(mhz float64) (channel int, band string) {
	f := int(mhz)
	switch {
	case f == 2484:
		return 14, "2.4 GHz"
	case f >= 2412 && f <= 2472:
		return (f - 2407) / 5, "2.4 GHz"
	case f >= 5925 && f <= 7125:
		return (f - 5950) / 5, "6 GHz"
	case f >= 5000 && f < 5925:
		return (f - 5000) / 5, "5 GHz"
	}
	return 0, ""
}

func normalizeBand(s string) string {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, "2.4"):
		return "2.4 GHz"
	case strings.HasPrefix(s, "5"):
		return "5 GHz"
	case strings.HasPrefix(s, "6"):
		return "6 GHz"
	}
	return s
}

// checkRadioBlocked catches the answer that costs nothing to find and solves
// the call outright: the wireless radio is switched off.
func checkRadioBlocked(ctx context.Context, f Facts) Result {
	if f.Platform.OS != platform.Linux || !commandExists("rfkill") {
		return skipf("no rfkill on this machine")
	}
	soft, hard := parseRfkill(capture(ctx, "rfkill", "list"))
	switch {
	case hard:
		return failf("Find the physical Wi-Fi switch or key (often Fn+F2) and turn the radio back on.",
			"the wireless radio is blocked by a hardware switch")
	case soft:
		return failf("Turn Wi-Fi back on: 'rfkill unblock wifi', or the Wi-Fi toggle in system settings.",
			"the wireless radio is switched off in software")
	default:
		return okf("radio enabled")
	}
}

// parseRfkill reads `rfkill list`, reporting the state of the wireless LAN
// entries only — Bluetooth being blocked says nothing about Wi-Fi.
func parseRfkill(out string) (soft, hard bool) {
	inWLAN := false
	for _, line := range lines(out) {
		if strings.Contains(line, ": ") && !strings.HasPrefix(line, "Soft") && !strings.HasPrefix(line, "Hard") {
			inWLAN = strings.Contains(line, "Wireless LAN")
			continue
		}
		if !inWLAN {
			continue
		}
		switch {
		case strings.HasPrefix(line, "Soft blocked:"):
			soft = soft || strings.Contains(line, "yes")
		case strings.HasPrefix(line, "Hard blocked:"):
			hard = hard || strings.Contains(line, "yes")
		}
	}
	return soft, hard
}

// checkLinkSpeed catches a gigabit port negotiated down to 100 Mb, which is
// nearly always a damaged cable or a dirty port — and presents as "the
// internet got slow" long after anyone would think to check the cable.
func checkLinkSpeed(_ context.Context, f Facts) Result {
	if f.Platform.OS != platform.Linux {
		return skipf("link speed is only read on Linux so far")
	}
	data, err := os.ReadFile("/sys/class/net/" + f.Iface + "/speed")
	if err != nil {
		return skipf("the driver does not report a link speed")
	}
	mbps, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || mbps <= 0 {
		return skipf("the driver does not report a link speed")
	}

	switch {
	case mbps <= 100:
		return warnf("Almost always a damaged cable or a dirty port. Swap the cable before anything else.",
			"negotiated %d Mb/s — well below what the port supports", mbps)
	default:
		return okf("%d Mb/s", mbps)
	}
}

// parseLeadingInt reads the first signed integer in s, e.g. "-59 dBm" or
// "44 (5GHz, 80MHz)".
func parseLeadingInt(s string) int {
	return int(parseLeadingFloat(s))
}

// parseLeadingFloat reads the first number in s, tolerating a sign, a decimal
// point, and whatever units follow.
func parseLeadingFloat(s string) float64 {
	s = strings.TrimSpace(s)
	end := 0
	for i, r := range s {
		if r >= '0' && r <= '9' || r == '.' || (i == 0 && (r == '-' || r == '+')) {
			end = i + 1
			continue
		}
		break
	}
	f, err := strconv.ParseFloat(s[:end], 64)
	if err != nil {
		return 0
	}
	return f
}
