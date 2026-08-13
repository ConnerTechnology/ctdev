package diagnose

import (
	"context"
	"net/netip"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

func macChecks(info platform.Info, f Facts) []Check {
	return nil
}

func macGateway(ctx context.Context) netip.Addr {
	return parseRouteGetDefault(capture(ctx, "route", "-n", "get", "default"))
}

// parseRouteGetDefault reads `route -n get default`, which prints an indented
// key/value block:
//
//	   route to: default
//	destination: default
//	    gateway: 192.168.1.1
//	  interface: en0
func parseRouteGetDefault(out string) netip.Addr {
	for _, line := range lines(out) {
		key, value, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(key) != "gateway" {
			continue
		}
		if addr, err := netip.ParseAddr(strings.TrimSpace(value)); err == nil {
			return addr.Unmap()
		}
	}
	return netip.Addr{}
}

func macResolvers(ctx context.Context) []netip.Addr {
	return parseScutilDNS(capture(ctx, "scutil", "--dns"))
}

// parseScutilDNS reads `scutil --dns`, which prints a resolver block per
// search domain:
//
//	resolver #1
//	  search domain[0] : lan
//	  nameserver[0] : 192.168.1.1
//	  nameserver[1] : 1.1.1.1
//
// Later resolvers are scoped to specific domains or interfaces; the first
// block is the system default, which is what a diagnosis cares about.
func parseScutilDNS(out string) []netip.Addr {
	var fields []string
	seenFirst := false
	for _, line := range lines(out) {
		if strings.HasPrefix(line, "resolver #") {
			if seenFirst {
				break
			}
			seenFirst = true
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found || !strings.HasPrefix(strings.TrimSpace(key), "nameserver[") {
			continue
		}
		fields = append(fields, strings.TrimSpace(value))
	}
	return parseAddrList(fields)
}

// macIfaceIsWireless asks networksetup which BSD device backs the Wi-Fi port,
// since the name varies across models (en0 on laptops, en1 elsewhere).
func macIfaceIsWireless(ctx context.Context, iface string) bool {
	if iface == "" {
		return false
	}
	return parseHardwarePortDevice(capture(ctx, "networksetup", "-listallhardwareports"), "Wi-Fi") == iface
}

// parseHardwarePortDevice pulls the device for a named hardware port out of
// `networksetup -listallhardwareports`:
//
//	Hardware Port: Wi-Fi
//	Device: en0
//	Ethernet Address: ...
func parseHardwarePortDevice(out, port string) string {
	matched := false
	for _, line := range lines(out) {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "Hardware Port":
			matched = value == port
		case "Device":
			if matched {
				return value
			}
		}
	}
	return ""
}
