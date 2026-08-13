package diagnose

import (
	"strings"
	"testing"
)

func TestUnifiDeviceVerdict(t *testing.T) {
	healthy := []UnifiDevice{
		{Name: "Living Room", Type: "uap", State: unifiStateConnected, Uplink: uplink("wire")},
		{Name: "Garage", Type: "uap", State: unifiStateConnected, Uplink: uplink("wire")},
	}
	if got := unifiDeviceVerdict(healthy).Severity; got != OK {
		t.Errorf("healthy site = %v, want OK", got)
	}

	offline := append([]UnifiDevice{}, healthy...)
	offline[1].State = unifiStateDisconnected
	res := unifiDeviceVerdict(offline)
	if res.Severity != Fail {
		t.Errorf("offline AP = %v, want Fail", res.Severity)
	}
	if !strings.Contains(res.Detail, "Garage") {
		t.Errorf("detail should name the offline device: %q", res.Detail)
	}

	// A mesh uplink works, so nobody questions it — while the AP spends half
	// its airtime relaying its own backhaul.
	mesh := append([]UnifiDevice{}, healthy...)
	mesh[1].Uplink = uplink("wireless")
	res = unifiDeviceVerdict(mesh)
	if res.Severity != Warn {
		t.Errorf("mesh uplink = %v, want Warn", res.Severity)
	}
	if !strings.Contains(strings.ToLower(res.Advice), "cable") {
		t.Errorf("advice should suggest a wired uplink: %q", res.Advice)
	}

	// An AP mid-upgrade is transient, not a fault.
	upgrading := append([]UnifiDevice{}, healthy...)
	upgrading[1].State = unifiStateUpgrading
	if got := unifiDeviceVerdict(upgrading).Severity; got != Info {
		t.Errorf("upgrading AP = %v, want Info", got)
	}
}

func uplink(kind string) struct {
	Type string `json:"type"`
} {
	return struct {
		Type string `json:"type"`
	}{Type: kind}
}

// Airtime is the measurement that explains slow Wi-Fi on a client showing full
// signal, and it exists nowhere on the client.
func TestUnifiAirtimeVerdict(t *testing.T) {
	quiet := []UnifiDevice{{Name: "AP", RadioStats: []UnifiRadioStat{
		{Radio: "na", Channel: 36, CuTotal: 12, CuSelfRx: 5, CuSelfTx: 4, NumSta: 6},
	}}}
	if got := unifiAirtimeVerdict(quiet).Severity; got != OK {
		t.Errorf("quiet channel = %v, want OK", got)
	}

	saturated := []UnifiDevice{{Name: "AP", RadioStats: []UnifiRadioStat{
		{Radio: "na", Channel: 36, CuTotal: 88, CuSelfRx: 40, CuSelfTx: 38, NumSta: 24},
	}}}
	res := unifiAirtimeVerdict(saturated)
	if res.Severity != Fail {
		t.Errorf("saturated channel = %v, want Fail", res.Severity)
	}
	// Self-inflicted congestion needs different advice from a crowded band.
	if !strings.Contains(strings.ToLower(res.Advice), "saturating its own") {
		t.Errorf("advice should identify self-inflicted load: %q", res.Advice)
	}

	crowded := []UnifiDevice{{Name: "AP", RadioStats: []UnifiRadioStat{
		{Radio: "ng", Channel: 6, CuTotal: 85, CuSelfRx: 5, CuSelfTx: 5, NumSta: 3},
	}}}
	res = unifiAirtimeVerdict(crowded)
	if !strings.Contains(strings.ToLower(res.Advice), "other people's networks") {
		t.Errorf("advice should identify neighbouring interference: %q", res.Advice)
	}
	if !strings.Contains(res.Advice, "2.4 GHz") {
		t.Errorf("a congested 2.4 GHz channel deserves the band-specific advice: %q", res.Advice)
	}
}

// The finding that justifies the whole integration: from the client, a DFS
// eviction just looks like the Wi-Fi randomly died.
func TestUnifiRadarVerdict(t *testing.T) {
	quiet := []UnifiEvent{{Key: "EVT_AP_Connected", AP: "aa:bb"}}
	if got := unifiRadarVerdict(quiet).Severity; got != OK {
		t.Errorf("no radar = %v, want OK", got)
	}

	radar := []UnifiEvent{
		{Key: "EVT_AP_RadarDetected", AP: "Living Room", Msg: "detected radar on channel 100"},
		{Key: "EVT_AP_RadarDetected", AP: "Living Room", Msg: "detected radar on channel 104"},
	}
	res := unifiRadarVerdict(radar)
	if res.Severity != Fail {
		t.Errorf("radar events = %v, want Fail", res.Severity)
	}
	if !strings.Contains(res.Advice, "non-DFS") {
		t.Errorf("advice should name non-DFS channels: %q", res.Advice)
	}
	if !strings.Contains(res.Detail, "Living Room") {
		t.Errorf("detail should name the affected AP: %q", res.Detail)
	}

	// Repeated dropouts are a different problem with different advice.
	var drops []UnifiEvent
	for range 6 {
		drops = append(drops, UnifiEvent{Key: "EVT_AP_Lost_Contact", AP: "Garage"})
	}
	res = unifiRadarVerdict(drops)
	if res.Severity != Warn {
		t.Errorf("repeated dropouts = %v, want Warn", res.Severity)
	}
	if !strings.Contains(strings.ToLower(res.Advice), "poe") {
		t.Errorf("advice should raise power as a cause: %q", res.Advice)
	}

	// A single dropout is noise, not a finding.
	if got := unifiRadarVerdict([]UnifiEvent{{Key: "EVT_AP_Lost_Contact", AP: "Garage"}}).Severity; got != OK {
		t.Errorf("one dropout = %v, want OK", got)
	}
}

// A minimum-RSSI threshold set too aggressively disconnects people who are
// working fine, and presents as "the Wi-Fi randomly drops at the edge".
func TestUnifiWLANVerdict(t *testing.T) {
	sane := []UnifiWLAN{{Name: "Home", Enabled: true, MinRSSIEnabled: false}}
	if got := unifiWLANVerdict(sane).Severity; got != OK {
		t.Errorf("no min-RSSI = %v, want OK", got)
	}

	// -80 and below is a reasonable floor; -70 kicks usable clients.
	aggressive := []UnifiWLAN{{Name: "Home", Enabled: true, MinRSSIEnabled: true, MinRSSI: -70}}
	res := unifiWLANVerdict(aggressive)
	if res.Severity != Warn {
		t.Errorf("aggressive min-RSSI = %v, want Warn", res.Severity)
	}
	if !strings.Contains(res.Detail, "Home") {
		t.Errorf("detail should name the network: %q", res.Detail)
	}

	conservative := []UnifiWLAN{{Name: "Home", Enabled: true, MinRSSIEnabled: true, MinRSSI: -85}}
	if got := unifiWLANVerdict(conservative).Severity; got != OK {
		t.Errorf("conservative min-RSSI = %v, want OK", got)
	}

	// A disabled network's settings don't affect anybody.
	disabled := []UnifiWLAN{{Name: "Old", Enabled: false, MinRSSIEnabled: true, MinRSSI: -60}}
	if got := unifiWLANVerdict(disabled).Severity; got != OK {
		t.Errorf("disabled network = %v, want OK", got)
	}
}

func TestUnifiHealthVerdict(t *testing.T) {
	up := []UnifiHealth{{Subsystem: "wan", Status: "ok", WANIP: "203.0.113.9"}}
	if got := unifiHealthVerdict(up).Severity; got != OK {
		t.Errorf("healthy WAN = %v, want OK", got)
	}

	down := []UnifiHealth{{Subsystem: "wan", Status: "error"}}
	if got := unifiHealthVerdict(down).Severity; got != Fail {
		t.Errorf("WAN down = %v, want Fail", got)
	}

	// No WAN subsystem reported means no opinion, not a pass.
	if got := unifiHealthVerdict([]UnifiHealth{{Subsystem: "wlan", Status: "ok"}}); got.Detail != "" {
		t.Errorf("absent WAN subsystem should produce no row, got %q", got.Detail)
	}
}

// A self-signed controller certificate is the norm, and the report has to say
// verification was skipped rather than quietly implying it wasn't.
func TestUnifiResultsReportsUnverifiedTLS(t *testing.T) {
	snap := &UnifiSnapshot{
		Devices:         []UnifiDevice{{Name: "AP", State: unifiStateConnected, Type: "uap", Uplink: uplink("wire")}},
		TLSVerified:     false,
		CertFingerprint: "abcdef0123456789abcdef0123456789",
	}
	results := unifiResults(snap)
	tlsRow, present := results["unifi.tls"]
	if !present {
		t.Fatal("expected a row noting the unverified certificate")
	}
	if !strings.Contains(tlsRow.Detail, "verification was skipped") {
		t.Errorf("row should state plainly that verification was skipped: %q", tlsRow.Detail)
	}

	// A properly verified controller needs no such note.
	snap.TLSVerified = true
	if _, present := unifiResults(snap)["unifi.tls"]; present {
		t.Error("a verified certificate should not produce a note")
	}
}

func TestUnifiChecksAreDeepAndNetworked(t *testing.T) {
	snap := &UnifiSnapshot{Devices: []UnifiDevice{{Name: "AP", State: unifiStateConnected}}}
	checks := unifiChecks(snap)
	if len(checks) == 0 {
		t.Fatal("expected vendor checks")
	}
	for _, c := range checks {
		// These call somebody else's infrastructure, so they must never run
		// in a default pass.
		if !c.Deep || !c.Network {
			t.Errorf("%s must be marked Deep and Network, got deep=%v network=%v", c.ID, c.Deep, c.Network)
		}
		if c.Name == "" || c.Name == c.ID {
			t.Errorf("%s has no human-readable name", c.ID)
		}
	}
}
