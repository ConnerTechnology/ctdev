package diagnose

import (
	"context"
	"net/netip"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

// The three Facts helpers below dispatch on the detected OS rather than build
// tags: every implementation is an exec of a system tool, so all of them
// compile everywhere and `go vet` stays honest cross-platform. This mirrors how
// package cleanup splits linux.go/macos.go.

func defaultGateway(ctx context.Context, info platform.Info) netip.Addr {
	switch info.OS {
	case platform.Linux:
		return linuxGateway(ctx)
	case platform.MacOS:
		return macGateway(ctx)
	case platform.Windows:
		return windowsGateway(ctx)
	}
	return netip.Addr{}
}

// resolvers reports the DNS servers queries from iface will actually reach.
// The interface matters: a VPN or a container bridge can carry its own
// resolver that the rest of the system never uses.
func resolvers(ctx context.Context, info platform.Info, iface string) []netip.Addr {
	switch info.OS {
	case platform.Linux:
		return linuxResolvers(ctx, iface)
	case platform.MacOS:
		return macResolvers(ctx)
	case platform.Windows:
		return windowsResolvers(ctx, iface)
	}
	return nil
}

func ifaceIsWireless(ctx context.Context, info platform.Info, iface string) bool {
	switch info.OS {
	case platform.Linux:
		return linuxIfaceIsWireless(iface)
	case platform.MacOS:
		return macIfaceIsWireless(ctx, iface)
	case platform.Windows:
		return windowsIfaceIsWireless(ctx, iface)
	}
	return false
}

// parseAddrList turns whitespace- or comma-separated address text into addrs,
// dropping anything unparseable. Shared by the resolver probes, which each get
// their list out of a different tool.
func parseAddrList(fields []string) []netip.Addr {
	var out []netip.Addr
	seen := make(map[netip.Addr]bool)
	for _, fld := range fields {
		// resolvectl and scutil both sometimes carry a scope or port suffix.
		if i := len(fld); i > 0 && fld[i-1] == ',' {
			fld = fld[:i-1]
		}
		addr, err := netip.ParseAddr(fld)
		if err != nil {
			continue
		}
		addr = addr.Unmap().WithZone("")
		if seen[addr] {
			continue
		}
		seen[addr] = true
		out = append(out, addr)
	}
	return out
}
