package diagnose

import (
	"context"
	"net"
	"net/netip"
	"os"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// GatherFacts resolves the prerequisites shared by many checks. It runs before
// the catalog and is deliberately serial: it's all local, it's fast, and having
// it settled means no check has to depend on another.
func GatherFacts(ctx context.Context, info platform.Info) Facts {
	f := Facts{Platform: info}
	f.Hostname, _ = os.Hostname()
	f.Root = sysutil.IsRoot() || sysutil.CanElevateQuietly(ctx)

	f.LocalIP = egressIP()
	f.Iface = ifaceForIP(f.LocalIP)
	if f.Iface != "" {
		f.IsWiFi = ifaceIsWireless(ctx, info, f.Iface)
	}
	f.Gateway = defaultGateway(ctx, info)
	f.DNS = resolvers(ctx, info, f.Iface)
	return f
}

// egressIP reports the source address the kernel would use to reach the
// internet. Opening a UDP socket sends no packets — it just asks the routing
// table — which makes this the one way to identify the egress interface
// without parsing a route table per OS.
func egressIP() netip.Addr {
	conn, err := net.Dial("udp", "1.1.1.1:80")
	if err != nil {
		return netip.Addr{}
	}
	defer conn.Close()
	ua, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return netip.Addr{}
	}
	addr, ok := netip.AddrFromSlice(ua.IP)
	if !ok {
		return netip.Addr{}
	}
	return addr.Unmap()
}

// ifaceForIP maps an address back to the interface that holds it.
func ifaceForIP(ip netip.Addr) string {
	if !ip.IsValid() {
		return ""
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if got, ok := netip.AddrFromSlice(ipnet.IP); ok && got.Unmap() == ip {
				return iface.Name
			}
		}
	}
	return ""
}

// LinkLocalIPv4 reports whether ip is an APIPA address (169.254.0.0/16), which
// means DHCP failed — the machine gave up and picked its own address, and it
// can talk to nothing beyond the local segment.
func LinkLocalIPv4(ip netip.Addr) bool {
	return ip.IsValid() && ip.Is4() && ip.IsLinkLocalUnicast()
}

// cgnatPrefix is RFC6598 carrier-grade NAT space. netip.Addr.IsPrivate covers
// RFC1918 and RFC4193 but not this, and it's exactly what an ISP-supplied
// router hands out when the customer has no public address of their own.
var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// IsCGNAT reports whether ip is a carrier-grade NAT lease.
func IsCGNAT(ip netip.Addr) bool {
	return ip.IsValid() && ip.Is4() && cgnatPrefix.Contains(ip)
}

// IsPrivate reports whether ip is on a network we're plausibly attached to
// rather than out on the internet: RFC1918, CGNAT, or link-local. Used to
// decide when skipping TLS verification is defensible.
func IsPrivate(ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return true
	}
	return IsCGNAT(ip)
}
