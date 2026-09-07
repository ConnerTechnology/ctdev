package setup

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// Pi-hole settings (the `configure pihole` category) read and write Pi-hole's
// configuration through `pihole-FTL --config`. Reads are attempted with sudo
// first — some keys (e.g. dns.upstreams) return empty to an unprivileged
// caller — and fall back to an unprivileged read. The configure command primes
// sudo before detecting, so reads succeed without a prompt.

// piholeUpstreamPresets maps a friendly key to the resolver IPs it stands for.
// The order within each list is the order written to Pi-hole.
var piholeUpstreamPresets = map[string][]string{
	"cloudflare": {"1.1.1.1", "1.0.0.1"},
	// Cloudflare for Families: same resolver, but answers for malware and
	// adult-content domains are withheld — network-wide filtering for a
	// household in one picker choice.
	"cloudflare-family": {"1.1.1.3", "1.0.0.3"},
	"quad9":             {"9.9.9.9", "149.112.112.112"},
	"google":            {"8.8.8.8", "8.8.4.4"},
	"unbound":           {"127.0.0.1#5335"},
}

// piholeInstalled reports whether Pi-hole is present (container or host); used
// as a HardwareFn so the Pi-hole settings only show on a node that runs Pi-hole.
func piholeInstalled() bool {
	return sysutil.PiholeAvailable()
}

// piholeConfigRead returns the current value of a pihole-FTL config key, read
// from the container or host install via the shared Pi-hole runtime helper.
func piholeConfigRead(ctx context.Context, key string) string {
	out, err := sysutil.PiholeCapture(ctx, "pihole-FTL", "--config", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// parseUpstreams turns pihole-FTL's "[ a, b ]" upstreams rendering into a slice
// of entries (each may carry a #port, e.g. 127.0.0.1#5335).
func parseUpstreams(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// detectPiholeUpstreams returns the preset key matching the configured
// upstreams, or "custom" when the set doesn't match a known preset.
func detectPiholeUpstreams(ctx context.Context) string {
	got := parseUpstreams(piholeConfigRead(ctx, "dns.upstreams"))
	if len(got) == 0 {
		return ""
	}
	for key, want := range piholeUpstreamPresets {
		if sameIPSet(got, want) {
			return key
		}
	}
	return "custom"
}

func sameIPSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	as := append([]string(nil), a...)
	bs := append([]string(nil), b...)
	sort.Strings(as)
	sort.Strings(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

// applyPiholeUpstreams writes the chosen preset's resolvers to Pi-hole. The
// pihole-FTL restart happens once via the "pihole-ftl" post-apply hook.
func applyPiholeUpstreams(ctx context.Context, o sysutil.Opts, value string) error {
	ips, ok := piholeUpstreamPresets[value]
	if !ok {
		return nil // "custom"/unknown: leave the user's resolvers untouched
	}
	return sysutil.PiholeRun(ctx, o, "pihole-FTL", "--config", "dns.upstreams",
		`["`+strings.Join(ips, `","`)+`"]`)
}

// detectPiholeListenMode returns Pi-hole's current listening mode (e.g. ALL,
// LOCAL).
func detectPiholeListenMode(ctx context.Context) string {
	return piholeConfigRead(ctx, "dns.listeningMode")
}

// applyPiholeListenMode sets Pi-hole's listening mode. The pihole-FTL restart
// happens once via the "pihole-ftl" post-apply hook.
func applyPiholeListenMode(ctx context.Context, o sysutil.Opts, value string) error {
	return sysutil.PiholeRun(ctx, o, "pihole-FTL", "--config", "dns.listeningMode", value)
}

// detectPiholeBlocking reports whether Pi-hole blocking is active.
func detectPiholeBlocking(ctx context.Context) string {
	if piholeConfigRead(ctx, "dns.blocking.active") == "false" {
		return "disabled"
	}
	return "enabled"
}

// applyPiholeBlocking enables or disables Pi-hole blocking via the pihole CLI,
// which applies the change live (no restart needed).
func applyPiholeBlocking(ctx context.Context, o sysutil.Opts, value string) error {
	if value == "disabled" {
		return sysutil.PiholeRun(ctx, o, "pihole", "disable")
	}
	return sysutil.PiholeRun(ctx, o, "pihole", "enable")
}

// applyPiholeRestart restarts pihole-FTL so config changes (upstreams, listen
// mode) take effect. Registered as the post-apply hook for the "pihole-ftl"
// group so it runs at most once per configure run.
func applyPiholeRestart(ctx context.Context, o sysutil.Opts) error {
	return sysutil.PiholeReload(ctx, o)
}

// ── Host resolver ──────────────────────────────────────────────────────────
//
// A Pi-hole node that accepts Tailscale DNS resolves through MagicDNS, whose
// global nameserver is this very Pi-hole. The host's own DNS then lives and
// dies with the container: with FTL down the node cannot pull the image to
// fix it, reach its backup repository, or run the brain. The host resolver
// setting breaks that dependency — loopback first, so the host's own lookups
// stay filtered and logged, and a public validating resolver second, which
// glibc reaches instantly because a stopped FTL refuses the connection
// rather than timing out.
//
// Tailnet names still have to resolve on this host (the brain dials the mail
// server by its MagicDNS name), so Pi-hole forwards the tailnet suffix to
// tailscaled's resolver at 100.100.100.100, which keeps answering MagicDNS
// queries even when it is no longer allowed to write resolv.conf. That
// forward reaches every LAN client too.

const (
	// hostResolverFallback answers when FTL is down. Quad9 validates DNSSEC
	// and is not the ISP; the router would usually be the ISP.
	hostResolverFallback = "9.9.9.9"
	hostResolverMarker   = "# ctdev: Pi-hole host resolver — managed by `ctdev configure pihole`"
	hostResolvConfPath   = "/etc/resolv.conf"
	// hostResolverNMConf tells NetworkManager to leave resolv.conf alone.
	hostResolverNMConf = "/etc/NetworkManager/conf.d/ctdev-pihole-resolver.conf"
	// hostResolverDnsmasq is the Pi-hole drop-in forwarding tailnet names.
	hostResolverDnsmasq = "03-tailnet.conf"
	magicDNSResolver    = "100.100.100.100"
)

func hostResolvConf() string {
	return hostResolverMarker + "\n" +
		"# Loopback is Pi-hole; when it is down the fallback answers immediately.\n" +
		"nameserver 127.0.0.1\n" +
		"nameserver " + hostResolverFallback + "\n" +
		"options timeout:2 attempts:1\n"
}

// detectHostResolverContent tells a ctdev-managed resolv.conf from anything
// else — Tailscale's, NetworkManager's, or a hand edit — by its marker.
func detectHostResolverContent(content string) string {
	if strings.Contains(content, hostResolverMarker) {
		return "applied"
	}
	return "not applied"
}

func detectHostResolver(_ context.Context) string {
	b, err := os.ReadFile(hostResolvConfPath)
	if err != nil {
		return "not applied"
	}
	return detectHostResolverContent(string(b))
}

// magicDNSForwardRecord forwards one tailnet suffix to tailscaled's resolver.
func magicDNSForwardRecord(suffix string) string {
	if suffix == "" {
		return ""
	}
	return "# Tailnet names resolve via tailscaled's MagicDNS resolver (ctdev-managed).\n" +
		"server=/" + suffix + "/" + magicDNSResolver + "\n"
}

// gatePiholeHostResolver shows the setting only where it can work: a Pi-hole
// node whose resolv.conf NetworkManager owns. Under systemd-resolved the
// stub at 127.0.0.53 owns the file and a static one would be overwritten.
func gatePiholeHostResolver() bool {
	if !piholeInstalled() || !gateNetworkManager() {
		return false
	}
	target, err := os.Readlink(hostResolvConfPath)
	return err != nil || !strings.Contains(target, "systemd")
}

func applyPiholeHostResolver(ctx context.Context, o sysutil.Opts) error {
	if sysutil.CommandExists("tailscale") {
		if err := sysutil.SudoRun(ctx, o, "tailscale", "set", "--accept-dns=false"); err != nil {
			return fmt.Errorf("stop Tailscale managing resolv.conf: %w", err)
		}
	}
	if err := sysutil.SudoWriteFileMode(ctx, o, "[main]\ndns=none\n", hostResolverNMConf, "0644"); err != nil {
		return err
	}
	if sysutil.CommandExists("nmcli") {
		_ = sysutil.SudoRun(ctx, o, "nmcli", "general", "reload")
	}
	// NetworkManager may have left a symlink; install would write through it.
	// World-readable on purpose: every process on the host reads this file.
	_ = sysutil.SudoRun(ctx, o, "rm", "-f", hostResolvConfPath)
	if err := sysutil.SudoWriteFileMode(ctx, o, hostResolvConf(), hostResolvConfPath, "0644"); err != nil {
		return err
	}

	suffix := sysutil.TailscaleDNS(ctx).MagicDNSSuffix
	if suffix == "" {
		fmt.Fprintln(o.Stdout, "  Tailscale not up — tailnet names will not resolve on this host until 'ctdev configure pihole' is re-run")
	}
	if err := sysutil.PiholeWriteDnsmasq(ctx, o, hostResolverDnsmasq, magicDNSForwardRecord(suffix)); err != nil {
		return err
	}
	return sysutil.PiholeReload(ctx, o)
}

// resetPiholeHostResolver hands resolv.conf back to NetworkManager and
// Tailscale. Best-effort, like the rest of reset.
func resetPiholeHostResolver(ctx context.Context, o sysutil.Opts) {
	if _, err := os.Stat(hostResolverNMConf); err != nil {
		return
	}
	_ = sysutil.SudoRun(ctx, o, "rm", "-f", hostResolverNMConf)
	if sysutil.CommandExists("nmcli") {
		_ = sysutil.SudoRun(ctx, o, "nmcli", "general", "reload")
	}
	if sysutil.CommandExists("tailscale") {
		_ = sysutil.SudoRun(ctx, o, "tailscale", "set", "--accept-dns=true")
	}
	if sysutil.PiholeAvailable() {
		_ = sysutil.PiholeWriteDnsmasq(ctx, o, hostResolverDnsmasq, "")
		_ = sysutil.PiholeReload(ctx, o)
	}
}
