package diagnose

// sharedChecks is the cross-platform catalog: pure Go against the network
// stack, plus shell-outs whose per-OS differences are handled inside the probe.
//
// Checks are gated here rather than inside their own bodies, so a wired
// machine has no Wi-Fi rows at all instead of a column of "not applicable" —
// the same construction-time gating package cleanup uses.
func sharedChecks(f Facts) []Check {
	checks := []Check{
		{
			ID:    "link.ip",
			Name:  "IP address",
			Group: GroupNetwork,
			Run:   checkLocalAddress,
		},
		{
			ID:      "link.gateway",
			Name:    "Router",
			Group:   GroupNetwork,
			Network: true,
			Run:     checkGateway,
		},
		{
			ID:      "dns.resolvers",
			Name:    "DNS servers",
			Group:   GroupNetwork,
			Network: true,
			Run:     checkResolvers,
		},
		{
			ID:      "dns.system",
			Name:    "Name resolution",
			Group:   GroupNetwork,
			Network: true,
			Run:     checkSystemDNS,
		},
		{
			ID:      "dns.public",
			Name:    "Public DNS",
			Group:   GroupInternet,
			Network: true,
			Run:     checkPublicDNS,
		},
		{
			ID:      "dns.hijack",
			Name:    "DNS integrity",
			Group:   GroupInternet,
			Network: true,
			Run:     checkDNSHijack,
		},
		{
			ID:      "net.icmp",
			Name:    "Internet by address",
			Group:   GroupInternet,
			Network: true,
			Run:     checkInternetICMP,
		},
		{
			ID:      "net.captive",
			Name:    "Internet reachable",
			Group:   GroupInternet,
			Network: true,
			Run:     checkCaptivePortal,
		},
		{
			ID:      "net.https",
			Name:    "Secure connections",
			Group:   GroupInternet,
			Network: true,
			Run:     checkHTTPS,
		},
		{
			ID:      "net.public",
			Name:    "Public address",
			Group:   GroupInternet,
			Network: true,
			Run:     checkPublicIP,
		},
		{
			ID:      "net.ipv6",
			Name:    "IPv6",
			Group:   GroupInternet,
			Network: true,
			Run:     checkIPv6,
		},
		{
			ID:      "sys.clock",
			Name:    "Clock",
			Group:   GroupSystem,
			Network: true,
			Run:     checkClock,
		},
	}

	if f.IsWiFi {
		checks = append(checks,
			Check{
				ID:    "link.wifi",
				Name:  "Wi-Fi signal",
				Group: GroupNetwork,
				Run:   checkWiFi,
			},
			Check{
				ID:    "link.radio",
				Name:  "Wi-Fi radio",
				Group: GroupNetwork,
				Run:   checkRadioBlocked,
			},
		)
	} else if f.Iface != "" {
		checks = append(checks, Check{
			ID:    "link.speed",
			Name:  "Link speed",
			Group: GroupNetwork,
			Run:   checkLinkSpeed,
		})
	}

	return checks
}
