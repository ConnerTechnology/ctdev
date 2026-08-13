package diagnose

import (
	"context"
	"net/netip"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

func windowsChecks(info platform.Info, f Facts) []Check {
	return nil
}

// Every Windows probe goes through PowerShell's CIM cmdlets, which — unlike
// the storage and Defender ones — all run unelevated. That matters: we're
// diagnosing a machine we were handed, not one we administer.

func windowsGateway(ctx context.Context) netip.Addr {
	out := powershell(ctx,
		`Get-NetRoute -DestinationPrefix '0.0.0.0/0' -ErrorAction SilentlyContinue |`+
			` Sort-Object RouteMetric |`+
			` Select-Object -First 1 -ExpandProperty NextHop`)
	addr, err := netip.ParseAddr(firstLine(out))
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func windowsResolvers(ctx context.Context, iface string) []netip.Addr {
	// Scope to the egress adapter where we know it: a machine with Wi-Fi,
	// Ethernet, and a VPN adapter all present will otherwise report resolvers
	// nothing is actually querying.
	scope := ""
	if iface != "" {
		scope = ` -InterfaceAlias '` + escapePS(iface) + `'`
	}
	out := powershell(ctx,
		`Get-DnsClientServerAddress -AddressFamily IPv4`+scope+` -ErrorAction SilentlyContinue |`+
			` Select-Object -ExpandProperty ServerAddresses`)
	return parseAddrList(lines(out))
}

// windowsIfaceIsWireless reads the adapter's physical media type. Go reports
// the adapter's friendly name in net.Interface.Name on Windows, which is the
// same name Get-NetAdapter takes.
func windowsIfaceIsWireless(ctx context.Context, iface string) bool {
	if iface == "" {
		return false
	}
	out := powershell(ctx,
		`Get-NetAdapter -Name '`+escapePS(iface)+`' -ErrorAction SilentlyContinue |`+
			` Select-Object -ExpandProperty PhysicalMediaType`)
	return strings.Contains(firstLine(out), "802.11")
}

// escapePS escapes a value for a PowerShell single-quoted string, where the
// only special character is the quote itself, doubled.
func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
