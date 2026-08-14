// Package diagnose is ctdev's field-tech health report. Unlike the rest of
// ctdev, nothing here assumes the machine is one we manage — or even one we've
// seen before. Every check is read-only, degrades to Skipped rather than
// failing when a tool or root is missing, and carries plain-English advice for
// whoever's laptop this is.
//
// The catalog follows the same shape as package cleanup: struct literals with
// closures, built as a function of platform.Info, run concurrently.
package diagnose

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

// Severity ranks a finding by how much it demands attention.
type Severity int

const (
	// OK means the check looked and found nothing wrong.
	OK Severity = iota
	// Info is context worth printing that needs no action.
	Info
	// Warn is degraded but working — worth fixing, not why they called.
	Warn
	// Fail is broken, and likely the thing they called about.
	Fail
	// Skipped means we couldn't look: no tool, no root, or not applicable.
	Skipped
)

// Rank orders severities for display — what needs attention first, then what's
// healthy, then what we couldn't check. This deliberately differs from the
// constant order so Skipped sorts last rather than worst.
func (s Severity) Rank() int {
	switch s {
	case Fail:
		return 0
	case Warn:
		return 1
	case Info:
		return 2
	case OK:
		return 3
	default:
		return 4
	}
}

func (s Severity) String() string {
	switch s {
	case OK:
		return "ok"
	case Info:
		return "info"
	case Warn:
		return "warn"
	case Fail:
		return "fail"
	default:
		return "skipped"
	}
}

// Glyph is the status marker, matching the vocabulary already used by
// `ctdev verify` and `ctdev status`.
func (s Severity) Glyph() string {
	switch s {
	case OK:
		return "✓"
	case Info:
		return "•"
	case Warn:
		return "⚠"
	case Fail:
		return "✗"
	default:
		return "○"
	}
}

// Check groups, in the order a technician actually works: what the machine is
// attached to, then whether that reaches the world, then the hardware under it.
const (
	GroupNetwork  = "Network"
	GroupInternet = "Internet"
	GroupHardware = "Hardware"
	GroupSystem   = "System"
	GroupSecurity = "Security"
)

// GroupOrder is the render order for check groups. Anything not listed sorts
// after these, alphabetically.
var GroupOrder = []string{GroupNetwork, GroupInternet, GroupHardware, GroupSystem, GroupSecurity}

// Check is one diagnostic probe. It never mutates the system.
type Check struct {
	ID    string
	Name  string
	Group string
	// Network marks a check that performs network I/O. `ctdev status`
	// excludes these to keep its "fast enough for every login" contract.
	Network bool
	// Deep marks a check that is slow, uses the user's bandwidth, or touches
	// third-party infrastructure. Only runs under --deep.
	Deep bool

	Run func(ctx context.Context, f Facts) Result
}

// Data keys the correlation pass reads. Checks and Diagnose have to agree on
// these, so they are named constants — and the pass keys off them rather than
// pattern-matching the prose in Detail, which is written for people and free
// to be reworded.
const (
	// DataCaptivePortal is "yes" when the probe was redirected, as opposed to
	// failing outright. Only a redirect means there's a sign-in page.
	DataCaptivePortal = "captive_portal"
	// DataRSSI is Wi-Fi signal strength in dBm, as a plain integer.
	DataRSSI = "rssi_dbm"
	// DataClockSkewSec is the signed clock offset in seconds.
	DataClockSkewSec = "clock_skew_seconds"
	// DataTempC is CPU temperature in whole degrees Celsius.
	DataTempC = "cpu_celsius"
)

// Result is what a Check found.
type Result struct {
	Severity Severity
	// Detail is the one-line finding, e.g.
	// "-78 dBm on ch 6 (2.4 GHz), 11 other APs on that channel".
	Detail string
	// Advice is what to do about it, in plain English. Rendered only for
	// Warn and Fail.
	Advice string
	// Data carries structured extras for the Markdown report. Never put a
	// credential here — the report is meant to be shared.
	Data map[string]string
}

// Result constructors, kept terse because check bodies are full of them.

func okf(format string, a ...any) Result {
	return Result{Severity: OK, Detail: fmt.Sprintf(format, a...)}
}

func infof(format string, a ...any) Result {
	return Result{Severity: Info, Detail: fmt.Sprintf(format, a...)}
}

func warnf(advice, format string, a ...any) Result {
	return Result{Severity: Warn, Detail: fmt.Sprintf(format, a...), Advice: advice}
}

func failf(advice, format string, a ...any) Result {
	return Result{Severity: Fail, Detail: fmt.Sprintf(format, a...), Advice: advice}
}

func skipf(format string, a ...any) Result {
	return Result{Severity: Skipped, Detail: fmt.Sprintf(format, a...)}
}

// Facts are the prerequisites shared by many checks, resolved once up front so
// checks don't each re-derive them and don't need a dependency graph between
// them. Gathering is serial and local; the checks themselves then run
// concurrently.
type Facts struct {
	Platform platform.Info
	Hostname string

	// Iface is the interface carrying the default route — the one that
	// matters when someone says "the internet doesn't work".
	Iface  string
	IsWiFi bool
	// LocalIP is that interface's address. An invalid Addr means we found no
	// usable address at all.
	LocalIP netip.Addr
	// Gateway is the default route's next hop.
	Gateway netip.Addr
	// DNS lists the configured resolvers, in the order the system will try.
	DNS []netip.Addr

	// Root records whether root-only sources (SMART, some kernel logs) are
	// readable, so checks can degrade to Skipped with a useful reason.
	Root bool

	// IntegrationsConfigured records that vendor credentials were supplied for
	// this run, so checks can stop offering set-up instructions for access
	// that already exists. Deliberately a bool and not the credentials
	// themselves: Facts travels into the report, and secrets must not.
	IntegrationsConfigured bool
}

// Finding is a verdict the correlation engine reached by combining several
// checks — the answer, as opposed to a single check's observation.
type Finding struct {
	Severity Severity
	// Title is the verdict in one sentence, phrased to be read aloud to the
	// person whose machine this is.
	Title  string
	Detail string
	Action string
	// Because lists the check IDs that produced this verdict.
	Because []string
}

// Report is a completed diagnostic run.
type Report struct {
	Facts    Facts
	Version  string
	Started  time.Time
	Elapsed  time.Duration
	Deep     bool
	Checks   []Check
	Results  map[string]Result
	Findings []Finding
}

// Catalog returns the checks that make sense on this machine. Checks are gated
// at construction time — an absent tool or an inapplicable link type produces
// no entry rather than a row that says "n/a". That's why it takes Facts: what
// this machine is attached to decides which questions are even worth asking.
func Catalog(info platform.Info, f Facts) []Check {
	checks := sharedChecks(f)
	switch info.OS {
	case platform.Linux:
		checks = append(checks, linuxChecks(info, f)...)
	case platform.MacOS:
		checks = append(checks, macChecks(info, f)...)
	case platform.Windows:
		checks = append(checks, windowsChecks(info, f)...)
	}
	return checks
}

// Select filters a catalog for a run. deep includes the slow and third-party
// checks; network includes the ones that touch the network at all.
func Select(checks []Check, deep, network bool) []Check {
	out := make([]Check, 0, len(checks))
	for _, c := range checks {
		if c.Deep && !deep {
			continue
		}
		if c.Network && !network {
			continue
		}
		out = append(out, c)
	}
	return out
}

// checkTimeout bounds a single check. A wedged controller or a black-holed
// resolver must not be able to stall the whole report. It's a var so tests
// don't have to wait it out.
var checkTimeout = 15 * time.Second

// maxConcurrent bounds in-flight probes. Most checks are shell-outs or network
// round-trips, so this is about not stampeding a small machine, not CPU.
const maxConcurrent = 8

// RunAll runs every check concurrently and returns results keyed by check ID.
//
// A check that panics or overruns its budget becomes a Skipped result rather
// than taking down the run: on someone else's machine, one surprising line of
// `ip` output should cost you that row, not the whole report.
func RunAll(ctx context.Context, checks []Check, f Facts) map[string]Result {
	results := make(map[string]Result, len(checks))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrent)

	for _, c := range checks {
		if c.Run == nil {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res := runOne(ctx, c, f)
			mu.Lock()
			results[c.ID] = res
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results
}

func runOne(ctx context.Context, c Check, f Facts) Result {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	// Buffered so the check's goroutine can always finish and exit, even after
	// we've given up waiting for it and returned the timeout below.
	done := make(chan Result, 1)
	go func() {
		// The recover has to live in here, with the call: a panic only
		// unwinds its own goroutine, so recovering in runOne would let a
		// panicking check take down the process.
		defer func() {
			if r := recover(); r != nil {
				done <- skipf("check panicked: %v", r)
			}
		}()
		done <- c.Run(ctx, f)
	}()

	select {
	case r := <-done:
		return r
	case <-ctx.Done():
		return skipf("timed out after %s", checkTimeout)
	}
}

// LocalChecks runs the checks that touch nothing but this machine — no network
// I/O, nothing slow. It exists so `ctdev status` can share this catalog while
// keeping its own contract of being cheap enough to run on every login.
func LocalChecks(ctx context.Context, info platform.Info, f Facts) (map[string]Result, []Check) {
	checks := Select(Catalog(info, f), false, false)
	return RunAll(ctx, checks, f), checks
}

// NeedsAttention filters results down to the ones worth interrupting someone
// for. A clean pass and a check that couldn't run are both silence.
func NeedsAttention(results map[string]Result) map[string]Result {
	out := make(map[string]Result, len(results))
	for id, res := range results {
		if res.Severity == Warn || res.Severity == Fail {
			out[id] = res
		}
	}
	return out
}
