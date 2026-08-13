package diagnose

import (
	"context"
	"net/netip"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

func macChecks(info platform.Info, f Facts) []Check {
	return nil
}

// macMemory assembles the memory picture from two sources, because macOS
// exposes no single one: the physical total from sysctl, and the page counts
// from vm_stat.
func macMemory(ctx context.Context) MemoryUsage {
	var m MemoryUsage
	if total, err := strconv.ParseInt(firstLine(capture(ctx, "sysctl", "-n", "hw.memsize")), 10, 64); err == nil {
		m.TotalKB = total / 1024
	}
	m.AvailableKB = parseVMStatAvailableKB(capture(ctx, "vm_stat"))

	swapTotal, swapUsed := parseSwapusage(capture(ctx, "sysctl", "-n", "vm.swapusage"))
	m.SwapTotalKB = swapTotal
	m.SwapFreeKB = swapTotal - swapUsed
	return m
}

// parseVMStatAvailableKB sums the page classes the kernel can hand to a new
// allocation without swapping — free, inactive, speculative, and purgeable.
// "Free" alone reads near zero on a healthy Mac, because unused memory is
// spent on cache by design.
func parseVMStatAvailableKB(out string) int64 {
	pageSize := int64(4096)
	if _, rest, found := strings.Cut(out, "page size of "); found {
		if n, _, ok := strings.Cut(rest, " bytes"); ok {
			if v, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64); err == nil && v > 0 {
				pageSize = v
			}
		}
	}

	available := map[string]bool{
		"Pages free":        true,
		"Pages inactive":    true,
		"Pages speculative": true,
		"Pages purgeable":   true,
	}

	var pages int64
	for _, line := range lines(out) {
		key, value, found := strings.Cut(line, ":")
		if !found || !available[strings.TrimSpace(key)] {
			continue
		}
		v, err := strconv.ParseInt(strings.TrimRight(strings.TrimSpace(value), "."), 10, 64)
		if err != nil {
			continue
		}
		pages += v
	}
	return pages * pageSize / 1024
}

// parseSwapusage reads `sysctl -n vm.swapusage`:
//
//	total = 2048.00M  used = 512.00M  free = 1536.00M  (encrypted)
func parseSwapusage(out string) (totalKB, usedKB int64) {
	// Dropping the '=' leaves alternating key/value fields to walk pairwise.
	fields := strings.Fields(strings.ReplaceAll(out, "=", " "))
	for i := 0; i+1 < len(fields); i += 2 {
		kb := parseSizeSuffixKB(fields[i+1])
		switch fields[i] {
		case "total":
			totalKB = kb
		case "used":
			usedKB = kb
		}
	}
	return totalKB, usedKB
}

// parseSizeSuffixKB reads a macOS size like "512.00M" or "2.00G" into KB.
func parseSizeSuffixKB(s string) int64 {
	if s == "" {
		return 0
	}
	mult := int64(1)
	switch s[len(s)-1] {
	case 'K':
		s = s[:len(s)-1]
	case 'M':
		s, mult = s[:len(s)-1], 1024
	case 'G':
		s, mult = s[:len(s)-1], 1024*1024
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(v) * mult
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
