package diagnose

import "strings"

// ouiVendors maps the first three octets of a MAC address to the company that
// registered them. This is a curated subset covering the gear that actually
// turns up in homes and small offices, not a copy of the IEEE registry — the
// full list is megabytes and the long tail buys us nothing here.
//
// A miss is not a failure. It means "we don't recognize this router", which is
// an honest and common answer.
var ouiVendors = map[string]string{
	// Ubiquiti — the reason this table exists. They hold many prefixes.
	"00:15:6D": "Ubiquiti", "00:27:22": "Ubiquiti", "04:18:D6": "Ubiquiti",
	"18:E8:29": "Ubiquiti", "24:5A:4C": "Ubiquiti", "24:A4:3C": "Ubiquiti",
	"28:70:4E": "Ubiquiti", "44:D9:E7": "Ubiquiti", "60:22:32": "Ubiquiti",
	"68:72:51": "Ubiquiti", "68:D7:9A": "Ubiquiti", "70:A7:41": "Ubiquiti",
	"74:83:C2": "Ubiquiti", "74:AC:B9": "Ubiquiti", "78:8A:20": "Ubiquiti",
	"80:2A:A8": "Ubiquiti", "94:2A:6F": "Ubiquiti", "9C:05:D6": "Ubiquiti",
	"AC:8B:A9": "Ubiquiti", "B4:A5:EF": "Ubiquiti", "B4:FB:E4": "Ubiquiti",
	"D0:21:F9": "Ubiquiti", "DC:9F:DB": "Ubiquiti", "E0:63:DA": "Ubiquiti",
	"E4:38:83": "Ubiquiti", "F0:9F:C2": "Ubiquiti", "F4:92:BF": "Ubiquiti",
	"FC:EC:DA": "Ubiquiti",

	// Synology
	"00:11:32": "Synology", "90:09:D0": "Synology",

	// eero
	"24:5E:BE": "eero", "74:6F:F7": "eero", "A0:21:B7": "eero", "F8:BB:BF": "eero",

	// TP-Link, including Omada and Deco
	"14:CC:20": "TP-Link", "50:C7:BF": "TP-Link", "60:32:B1": "TP-Link",
	"9C:53:22": "TP-Link", "EC:08:6B": "TP-Link",

	// Netgear
	"00:14:6C": "Netgear", "20:4E:7F": "Netgear", "9C:3D:CF": "Netgear",
	"A0:40:A0": "Netgear",

	// MikroTik
	"00:0C:42": "MikroTik", "2C:C8:1B": "MikroTik", "48:8F:5A": "MikroTik",
	"4C:5E:0C": "MikroTik", "DC:2C:6E": "MikroTik",

	// AVM (Fritz!Box)
	"00:04:0E": "AVM", "34:31:C4": "AVM", "38:10:D5": "AVM", "C8:0E:14": "AVM",

	// Google / Nest Wifi
	"00:1A:11": "Google", "3C:5A:B4": "Google", "54:60:09": "Google",
	"6C:AD:F8": "Google", "F4:F5:E8": "Google",

	// ASUS
	"00:1F:C6": "ASUS", "04:D9:F5": "ASUS", "2C:56:DC": "ASUS", "AC:9E:17": "ASUS",

	// Cisco Meraki
	"00:18:0A": "Meraki", "0C:8D:DB": "Meraki", "88:15:44": "Meraki",
	"E0:CB:BC": "Meraki",

	// Aruba / HPE
	"00:0B:86": "Aruba", "24:DE:C6": "Aruba", "6C:F3:7F": "Aruba", "94:B4:0F": "Aruba",

	// Ruckus
	"00:1D:2E": "Ruckus", "2C:C5:D3": "Ruckus", "58:93:96": "Ruckus", "8C:0C:90": "Ruckus",

	// Apple, for the AirPort era and Macs acting as gateways
	"00:1B:63": "Apple", "3C:07:54": "Apple", "A4:5E:60": "Apple", "F0:18:98": "Apple",
}

// LookupOUI returns the vendor that registered a MAC address prefix, or "" if
// it isn't one we recognize.
func LookupOUI(mac string) string {
	prefix := normalizeOUI(mac)
	if prefix == "" {
		return ""
	}
	return ouiVendors[prefix]
}

// normalizeOUI reduces a MAC in any common spelling to upper-case
// colon-separated first three octets.
func normalizeOUI(mac string) string {
	mac = strings.ToUpper(strings.TrimSpace(mac))
	mac = strings.ReplaceAll(mac, "-", ":")
	mac = strings.ReplaceAll(mac, ".", ":")

	parts := strings.Split(mac, ":")
	if len(parts) < 3 {
		return ""
	}
	for _, p := range parts[:3] {
		if len(p) != 2 || !isHexPair(p) {
			return ""
		}
	}
	return parts[0] + ":" + parts[1] + ":" + parts[2]
}

func isHexPair(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// LocallyAdministered reports whether a MAC has the locally-administered bit
// set. Multi-SSID access points mint a virtual BSSID per network this way, so
// a BSSID often carries no vendor prefix at all — which is why the gateway's
// MAC is the more reliable thing to fingerprint.
func LocallyAdministered(mac string) bool {
	prefix := normalizeOUI(mac)
	if prefix == "" {
		return false
	}
	// Second-least-significant bit of the first octet.
	first := hexVal(prefix[0])*16 + hexVal(prefix[1])
	return first&0x02 != 0
}

func hexVal(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10
	}
	return 0
}
