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

	checks = append(checks,
		Check{ID: "hw.disk", Name: "Disk space", Group: GroupHardware, Run: checkDiskSpace},
		Check{ID: "hw.inodes", Name: "Inodes", Group: GroupHardware, Run: checkInodes},
		Check{ID: "hw.memory", Name: "Memory", Group: GroupHardware, Run: checkMemory},
		Check{ID: "hw.load", Name: "CPU load", Group: GroupHardware, Run: checkLoad},
		Check{ID: "hw.temp", Name: "CPU temperature", Group: GroupHardware, Run: checkTemperature},
		Check{ID: "hw.smart", Name: "Drive health", Group: GroupHardware, Run: checkSMART},
		Check{ID: "hw.battery", Name: "Battery", Group: GroupHardware, Run: checkBattery},
		Check{ID: "hw.raid", Name: "RAID array", Group: GroupHardware, Run: checkRAID},

		Check{ID: "sys.uptime", Name: "Uptime", Group: GroupSystem, Run: checkUptime},
		Check{ID: "sys.reboot", Name: "Restart pending", Group: GroupSystem, Run: checkRebootPending},
		Check{ID: "sys.units", Name: "Services", Group: GroupSystem, Run: checkFailedUnits},
		Check{ID: "sys.oom", Name: "Memory kills", Group: GroupSystem, Run: checkOOM},
		Check{ID: "sys.os", Name: "OS support", Group: GroupSystem, Run: checkOSSupport},
		Check{ID: "sys.updates", Name: "Updates", Group: GroupSystem, Run: checkPendingUpdates},
		Check{ID: "sys.print", Name: "Print queue", Group: GroupSystem, Run: checkPrintQueue},

		Check{ID: "sec.firewall", Name: "Firewall", Group: GroupSecurity, Run: checkFirewall},
		Check{ID: "sec.encryption", Name: "Disk encryption", Group: GroupSecurity, Run: checkDiskEncryption},
		Check{ID: "sec.ssh", Name: "SSH access", Group: GroupSecurity, Run: checkSSHExposure},
	)

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
