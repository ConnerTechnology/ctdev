package diagnose

import (
	"testing"
)

func TestParseIWLink(t *testing.T) {
	// Real `iw dev wlp5s0 link` output from a Wi-Fi 7 association. Note the
	// fractional frequency, which older iw did not print.
	const out = `Connected to 22:0b:8b:c2:7d:db (on wlp5s0)
	SSID: Conner Trusted
	freq: 5785.0
	RX: 2352163431 bytes (1700886 packets)
	TX: 154056729 bytes (699876 packets)
	signal: -59 dBm
	rx bitrate: 413.0 MBit/s 40MHz EHT-MCS 8 EHT-NSS 2 EHT-GI 0
	tx bitrate: 458.8 MBit/s 40MHz EHT-MCS 9 EHT-NSS 2 EHT-GI 0
	bss flags: short-slot-time
	dtim period: 3
	beacon int: 100`

	w := parseIWLink(out)
	if !w.Associated {
		t.Fatal("expected an association")
	}
	if w.SSID != "Conner Trusted" {
		t.Errorf("SSID = %q, want %q (SSIDs contain spaces)", w.SSID, "Conner Trusted")
	}
	if w.BSSID != "22:0b:8b:c2:7d:db" {
		t.Errorf("BSSID = %q", w.BSSID)
	}
	if w.RSSI != -59 {
		t.Errorf("RSSI = %d, want -59", w.RSSI)
	}
	if w.Channel != 157 || w.Band != "5 GHz" {
		t.Errorf("channel/band = %d/%s, want 157/5 GHz", w.Channel, w.Band)
	}
	if w.TxMbps != 458.8 {
		t.Errorf("TxMbps = %v, want 458.8", w.TxMbps)
	}
}

func TestParseIWLinkNotConnected(t *testing.T) {
	if w := parseIWLink("Not connected."); w.Associated {
		t.Error("an unassociated radio should not report an association")
	}
}

func TestParseNetshWlan(t *testing.T) {
	const out = `There is 1 interface on the system:

    Name                   : Wi-Fi
    Description            : Intel(R) Wi-Fi 6 AX201 160MHz
    Physical address       : aa:bb:cc:dd:ee:ff
    State                  : connected
    SSID                   : Conner Trusted
    BSSID                  : 11:22:33:44:55:66
    Network type           : Infrastructure
    Radio type             : 802.11ax
    Band                   : 5 GHz
    Channel                : 157
    Receive rate (Mbps)    : 780
    Transmit rate (Mbps)   : 780
    Signal                 : 84%
    Profile                : Conner Trusted`

	w := parseNetshWlan(out)
	if !w.Associated {
		t.Fatal("expected an association")
	}
	if w.SSID != "Conner Trusted" || w.BSSID != "11:22:33:44:55:66" {
		t.Errorf("SSID/BSSID = %q/%q", w.SSID, w.BSSID)
	}
	if w.Channel != 157 || w.Band != "5 GHz" {
		t.Errorf("channel/band = %d/%s", w.Channel, w.Band)
	}
	// Windows reports quality, not dBm, so the value must be marked approximate
	// rather than presented with a precision it doesn't have.
	if w.RSSI != -58 {
		t.Errorf("RSSI = %d, want -58 (84%% quality)", w.RSSI)
	}
	if !w.RSSIApprox {
		t.Error("a percentage-derived RSSI must be flagged approximate")
	}
	if w.TxMbps != 780 {
		t.Errorf("TxMbps = %v, want 780", w.TxMbps)
	}
}

func TestParseNetshWlanDisconnected(t *testing.T) {
	const out = `    Name                   : Wi-Fi
    State                  : disconnected`
	if w := parseNetshWlan(out); w.Associated {
		t.Error("a disconnected interface should not report an association")
	}
}

func TestQualityToDBm(t *testing.T) {
	// The Windows scale is linear from -100 dBm at 0% to -50 dBm at 100%.
	tests := map[int]int{0: -100, 50: -75, 84: -58, 100: -50, 120: -50, -5: -100}
	for pct, want := range tests {
		if got := qualityToDBm(pct); got != want {
			t.Errorf("qualityToDBm(%d) = %d, want %d", pct, got, want)
		}
	}
}

func TestParseAirportData(t *testing.T) {
	const out = `Wi-Fi:

      Software Versions:
          CoreWLAN: 16.0

      Interfaces:
        en0:
          Card Type: Wi-Fi
          Status: Connected
          Current Network Information:
            Conner Trusted:
              PHY Mode: 802.11ax
              Channel: 157 (5GHz, 80MHz)
              Country Code: US
              Network Type: Infrastructure
              Security: WPA2 Personal
              Signal / Noise: -58 dBm / -92 dBm
              Transmit Rate: 780
          Other Local Wi-Fi Networks:
            NeighbourNet:
              PHY Mode: 802.11n
              Channel: 6 (2GHz, 20MHz)
              Signal / Noise: -85 dBm / -92 dBm`

	w := parseAirportData(out)
	if !w.Associated {
		t.Fatal("expected an association")
	}
	// The SSID contains a space, and the parser must not stop at the first
	// neighbouring network's numbers.
	if w.SSID != "Conner Trusted" {
		t.Errorf("SSID = %q, want %q", w.SSID, "Conner Trusted")
	}
	if w.RSSI != -58 || w.Noise != -92 {
		t.Errorf("signal/noise = %d/%d, want -58/-92", w.RSSI, w.Noise)
	}
	if got := w.SNR(); got != 34 {
		t.Errorf("SNR = %d, want 34", got)
	}
	if w.Channel != 157 || w.Band != "5 GHz" {
		t.Errorf("channel/band = %d/%s, want 157/5 GHz", w.Channel, w.Band)
	}
	if w.TxMbps != 780 {
		t.Errorf("TxMbps = %v, want 780", w.TxMbps)
	}
}

func TestChannelFromFreq(t *testing.T) {
	tests := []struct {
		mhz     float64
		channel int
		band    string
	}{
		{2412, 1, "2.4 GHz"},
		{2437, 6, "2.4 GHz"},
		{2472, 13, "2.4 GHz"},
		{2484, 14, "2.4 GHz"}, // Japan's channel 14 breaks the arithmetic
		{5180, 36, "5 GHz"},
		{5785.0, 157, "5 GHz"},
		// 6 GHz channel numbers restart, so the band has to travel with them.
		{5955, 1, "6 GHz"},
		{6175, 45, "6 GHz"},
		{900, 0, ""},
	}
	for _, tt := range tests {
		ch, band := channelFromFreq(tt.mhz)
		if ch != tt.channel || band != tt.band {
			t.Errorf("channelFromFreq(%v) = %d/%q, want %d/%q", tt.mhz, ch, band, tt.channel, tt.band)
		}
	}
}

func TestWiFiSignalVerdict(t *testing.T) {
	tests := []struct {
		rssi int
		want Severity
	}{
		{-35, OK},
		{-59, OK},
		{-60, OK},
		{-61, Info},
		{-70, Info},
		{-71, Warn},
		{-80, Warn},
		{-81, Fail},
		{-92, Fail},
	}
	for _, tt := range tests {
		res := wifiSignalVerdict(tt.rssi)
		if res.Severity != tt.want {
			t.Errorf("wifiSignalVerdict(%d) = %v, want %v", tt.rssi, res.Severity, tt.want)
		}
		// A weak signal is only useful if it says what to do about it.
		if (tt.want == Warn || tt.want == Fail) && res.Advice == "" {
			t.Errorf("wifiSignalVerdict(%d) gave no advice", tt.rssi)
		}
	}
}

func TestParseRfkill(t *testing.T) {
	const enabled = `1: phy0: Wireless LAN
	Soft blocked: no
	Hard blocked: no
2: hci0: Bluetooth
	Soft blocked: no
	Hard blocked: no`
	if soft, hard := parseRfkill(enabled); soft || hard {
		t.Errorf("enabled radio read as soft=%v hard=%v", soft, hard)
	}

	const softBlocked = `0: phy0: Wireless LAN
	Soft blocked: yes
	Hard blocked: no`
	if soft, hard := parseRfkill(softBlocked); !soft || hard {
		t.Errorf("soft-blocked radio read as soft=%v hard=%v", soft, hard)
	}

	// Bluetooth being blocked says nothing about Wi-Fi, and reporting it as
	// "your Wi-Fi is off" would send someone hunting a switch that's fine.
	const btOnly = `1: phy0: Wireless LAN
	Soft blocked: no
	Hard blocked: no
2: hci0: Bluetooth
	Soft blocked: yes
	Hard blocked: yes`
	if soft, hard := parseRfkill(btOnly); soft || hard {
		t.Errorf("Bluetooth block leaked into the Wi-Fi verdict: soft=%v hard=%v", soft, hard)
	}
}

func TestParseLeadingFloat(t *testing.T) {
	tests := map[string]float64{
		"-59 dBm":            -59,
		"458.8 MBit/s 40MHz": 458.8,
		"157 (5GHz, 80MHz)":  157,
		"780":                780,
		"":                   0,
		"n/a":                0,
	}
	for in, want := range tests {
		if got := parseLeadingFloat(in); got != want {
			t.Errorf("parseLeadingFloat(%q) = %v, want %v", in, got, want)
		}
	}
}
