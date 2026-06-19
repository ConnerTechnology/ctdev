package setup

import (
	"context"
	"os/exec"
	"regexp"
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
	"quad9":      {"9.9.9.9", "149.112.112.112"},
	"google":     {"8.8.8.8", "8.8.4.4"},
}

var ipv4Re = regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)

// piholeInstalled reports whether the pihole CLI is present; used as a
// HardwareFn so the Pi-hole settings only show on a node that runs Pi-hole.
func piholeInstalled() bool {
	_, err := exec.LookPath("pihole")
	return err == nil
}

// piholeConfigRead returns the current value of a pihole-FTL config key,
// preferring a privileged read and falling back to an unprivileged one.
func piholeConfigRead(key string) string {
	if out, err := exec.Command("sudo", "-n", "pihole-FTL", "--config", key).Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	out, err := exec.Command("pihole-FTL", "--config", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// detectPiholeUpstreams returns the preset key matching the configured
// upstreams, or "custom" when the set doesn't match a known preset.
func detectPiholeUpstreams() string {
	got := ipv4Re.FindAllString(piholeConfigRead("dns.upstreams"), -1)
	if len(got) == 0 {
		return ""
	}
	for key, ips := range piholeUpstreamPresets {
		if sameIPSet(got, ips) {
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
	return sysutil.SudoRun(ctx, o, "pihole-FTL", "--config", "dns.upstreams",
		`["`+strings.Join(ips, `","`)+`"]`)
}

// detectPiholeListenMode returns Pi-hole's current listening mode (e.g. ALL,
// LOCAL).
func detectPiholeListenMode() string {
	return piholeConfigRead("dns.listeningMode")
}

// applyPiholeListenMode sets Pi-hole's listening mode. The pihole-FTL restart
// happens once via the "pihole-ftl" post-apply hook.
func applyPiholeListenMode(ctx context.Context, o sysutil.Opts, value string) error {
	return sysutil.SudoRun(ctx, o, "pihole-FTL", "--config", "dns.listeningMode", value)
}

// detectPiholeBlocking reports whether Pi-hole blocking is active.
func detectPiholeBlocking() string {
	if piholeConfigRead("dns.blocking.active") == "false" {
		return "disabled"
	}
	return "enabled"
}

// applyPiholeBlocking enables or disables Pi-hole blocking via the pihole CLI,
// which applies the change live (no restart needed).
func applyPiholeBlocking(ctx context.Context, o sysutil.Opts, value string) error {
	if value == "disabled" {
		return sysutil.SudoRun(ctx, o, "pihole", "disable")
	}
	return sysutil.SudoRun(ctx, o, "pihole", "enable")
}

// applyPiholeRestart restarts pihole-FTL so config changes (upstreams, listen
// mode) take effect. Registered as the post-apply hook for the "pihole-ftl"
// group so it runs at most once per configure run.
func applyPiholeRestart(ctx context.Context, o sysutil.Opts) error {
	return sysutil.SudoRun(ctx, o, "systemctl", "restart", "pihole-FTL")
}
