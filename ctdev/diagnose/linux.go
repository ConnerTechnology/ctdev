package diagnose

import (
	"context"
	"net/netip"
	"os"
	"slices"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

func linuxChecks(info platform.Info, f Facts) []Check {
	var checks []Check
	// Only worth asking on a machine that runs containers — the controller is
	// irrelevant otherwise, and a row saying so is noise.
	if commandExists("docker") {
		checks = append(checks, Check{
			ID:    "sys.memcgroup",
			Name:  "Memory cgroup",
			Group: GroupSystem,
			Run:   checkMemoryCgroup,
		})
	}
	return checks
}

// checkMemoryCgroup reports whether the kernel exposes the memory controller.
//
// Without it Docker reports every per-container CPU, memory *and* network
// figure as zero — the stats path gives up on the first bad memory read rather
// than degrading to partial data, and Beszel's agent aborts its whole
// container-stats block for the same reason (henrygd/beszel#144). Flat zeros
// read as "idle containers", not "broken instrumentation", which is how this
// hides: ctpi01 ran nine days of a 125 MB/day leak with every Docker panel a
// flat line at zero. `mem_limit` in a compose file is silently ignored too, so
// the backstop that should have caught the leak was never armed either.
//
// Raspberry Pi OS prepends cgroup_disable=memory to the device-tree bootargs,
// so a stock Pi always lands here until cmdline.txt overrides it.
func checkMemoryCgroup(_ context.Context, _ Facts) Result {
	b, err := os.ReadFile("/sys/fs/cgroup/cgroup.controllers")
	if err != nil {
		// cgroup v1, or a kernel without unified hierarchy. Not a fault, and
		// not something this check can speak to.
		return skipf("no cgroup v2 controller list at /sys/fs/cgroup/cgroup.controllers")
	}
	return memoryCgroupVerdict(strings.Fields(string(b)))
}

// memoryCgroupVerdict is the rule, split out so it is testable without a
// particular kernel underneath.
func memoryCgroupVerdict(controllers []string) Result {
	if slices.Contains(controllers, "memory") {
		return okf("memory controller enabled — per-container stats and mem_limit work")
	}
	return warnf(
		`Append "cgroup_enable=memory cgroup_memory=1" to the end of /boot/firmware/cmdline.txt, keeping it a single line, then reboot. The firmware prepends cgroup_disable=memory, so yours has to come last to win. Verify with: cat /sys/fs/cgroup/cgroup.controllers`,
		"memory controller missing (have: %s) — per-container CPU, memory and network all report zero, and mem_limit is ignored",
		strings.Join(controllers, " "))
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
