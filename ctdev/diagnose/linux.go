package diagnose

import (
	"context"
	"net/netip"
	"os"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

func linuxChecks(info platform.Info, f Facts) []Check {
	return nil
}

func linuxGateway(ctx context.Context) netip.Addr {
	return parseIPRouteDefault(capture(ctx, "ip", "-4", "route", "show", "default"))
}

// parseIPRouteDefault pulls the next hop out of `ip route show default`, e.g.
//
//	default via 192.168.1.1 dev wlp3s0 proto dhcp src 192.168.1.42 metric 600
//
// When several default routes exist (a VPN alongside the physical link), the
// kernel lists them in preference order, so the first "via" is the one in use.
func parseIPRouteDefault(out string) netip.Addr {
	for _, line := range lines(out) {
		fields := strings.Fields(line)
		for i, fld := range fields {
			if fld == "via" && i+1 < len(fields) {
				if addr, err := netip.ParseAddr(fields[i+1]); err == nil {
					return addr.Unmap()
				}
			}
		}
	}
	return netip.Addr{}
}

func linuxResolvers(ctx context.Context, iface string) []netip.Addr {
	// systemd-resolved is authoritative where it runs: /etc/resolv.conf then
	// points at the 127.0.0.53 stub, which tells us nothing about the real
	// upstreams.
	if commandExists("resolvectl") {
		links := parseResolvectlLinks(capture(ctx, "resolvectl", "dns"))
		if addrs := resolversForLink(links, iface); len(addrs) > 0 {
			return addrs
		}
	}
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	return parseResolvConf(string(data))
}

// parseResolvectlLinks reads `resolvectl dns`, which prints one line per link:
//
//	Global:
//	Link 2 (enp7s0):
//	Link 3 (wlp5s0): 10.2.2.144
//	Link 4 (tailscale0): 100.100.100.100 fd7a:115c:a1e0::53
//
// The result is keyed by interface name, with "" for the Global block. Keeping
// them separate matters: resolved routes queries per link, so a VPN's private
// resolver is not a resolver the rest of the system uses.
func parseResolvectlLinks(out string) map[string][]netip.Addr {
	links := make(map[string][]netip.Addr)
	for _, line := range lines(out) {
		head, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		addrs := parseAddrList(strings.Fields(rest))
		if len(addrs) == 0 {
			continue
		}
		name := ""
		if open := strings.IndexByte(head, '('); open >= 0 {
			if close := strings.IndexByte(head[open:], ')'); close > 0 {
				name = head[open+1 : open+close]
			}
		}
		links[name] = append(links[name], addrs...)
	}
	return links
}

// resolversForLink picks the resolvers that queries from iface will actually
// reach: the link's own, else the global ones.
func resolversForLink(links map[string][]netip.Addr, iface string) []netip.Addr {
	if addrs := links[iface]; len(addrs) > 0 {
		return addrs
	}
	return links[""]
}

// parseResolvConf reads nameserver lines out of resolv.conf format.
func parseResolvConf(content string) []netip.Addr {
	var fields []string
	for _, line := range lines(content) {
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		rest, found := strings.CutPrefix(line, "nameserver")
		if !found {
			continue
		}
		fields = append(fields, strings.Fields(rest)...)
	}
	return parseAddrList(fields)
}

// linuxIfaceIsWireless checks sysfs rather than shelling out: every wireless
// netdev has a wireless/ directory, whether or not iw or nmcli is installed.
func linuxIfaceIsWireless(iface string) bool {
	if iface == "" {
		return false
	}
	_, err := os.Stat("/sys/class/net/" + iface + "/wireless")
	return err == nil
}
