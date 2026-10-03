package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// Key repeat defaults for the macOS `keyboard` sliders, in milliseconds.
// macOS stores both in units of 15 ms (see keyRepeatUnitMs).
const (
	// macKeyRepeatIntervalDefaultMs is the recommended time between repeated
	// characters: 30 ms is KeyRepeat 2. Set "15" for KeyRepeat 1, the fastest.
	macKeyRepeatIntervalDefaultMs = "30"
	// macKeyRepeatDelayDefaultMs is the recommended hold before repeating
	// starts: the 15 ms step nearest the Linux default of 200 ms.
	macKeyRepeatDelayDefaultMs = "195"
)

// keyRepeatUnitMs is the unit of NSGlobalDomain KeyRepeat and InitialKeyRepeat.
const keyRepeatUnitMs = 15

// macKeyRepeatGroup is the ApplyGroup shared by the macOS key repeat sliders,
// so the log-out note prints once however many of them apply.
const macKeyRepeatGroup = "macos-keyrepeat"

// macRead runs a read-only command and returns its stdout. Used for
// `defaults read` only; tests substitute a fake so detect runs on any OS.
var macRead = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// macExec runs a command that changes the system (`defaults write`,
// `killall`). It is sysutil.Run, so dry-run prints instead of executing;
// tests substitute a fake for the non-dry-run path.
var macExec = sysutil.Run

// defaultsType is the value type of a `defaults write`.
type defaultsType int

const (
	defBool defaultsType = iota
	defInt
	defFloat
	defString
)

// flag returns the `defaults write` type flag.
func (t defaultsType) flag() string {
	switch t {
	case defBool:
		return "-bool"
	case defInt:
		return "-int"
	case defFloat:
		return "-float"
	default:
		return "-string"
	}
}

func (t defaultsType) String() string { return strings.TrimPrefix(t.flag(), "-") }

// macDefault is one `defaults` key ctdev sets. Value is what follows the type
// flag in `defaults write` ("true"/"false" for bools).
type macDefault struct {
	Section string // progress heading printed during apply
	Domain  string
	Key     string
	Type    defaultsType
	Value   string
}

// matches reports whether out, the trimmed output of `defaults read`, equals
// the entry's value. `defaults read` prints booleans as 1/0.
func (d macDefault) matches(out string) bool {
	switch d.Type {
	case defBool:
		want := "0"
		if d.Value == "true" {
			want = "1"
		}
		return out == want
	case defFloat:
		got, err1 := strconv.ParseFloat(out, 64)
		want, err2 := strconv.ParseFloat(d.Value, 64)
		return err1 == nil && err2 == nil && math.Abs(got-want) < 1e-9
	default:
		return out == d.Value
	}
}

// macOSDefaults is the table behind the `macos` category: ApplyMacOSDefaults
// writes every entry and detectMacOSDefaults reads every entry back.
var macOSDefaults = []macDefault{
	{"Dock", "com.apple.dock", "autohide", defBool, "true"},
	{"Dock", "com.apple.dock", "launchanim", defBool, "false"},
	{"Dock", "com.apple.dock", "show-recents", defBool, "false"},
	{"Dock", "com.apple.dock", "autohide-delay", defFloat, "0"},
	{"Dock", "com.apple.dock", "autohide-time-modifier", defFloat, "0.15"},
	{"Dock", "com.apple.dock", "mineffect", defString, "scale"},
	{"Dock", "com.apple.dock", "expose-animation-duration", defFloat, "0.1"},

	{"Windows", "NSGlobalDomain", "NSAutomaticWindowAnimationsEnabled", defBool, "false"},

	{"Sound", "NSGlobalDomain", "com.apple.sound.beep.feedback", defBool, "true"},

	{"Finder", "com.apple.finder", "ShowPathbar", defBool, "true"},
	{"Finder", "com.apple.finder", "ShowStatusBar", defBool, "true"},
	{"Finder", "com.apple.desktopservices", "DSDontWriteNetworkStores", defBool, "true"},
	{"Finder", "com.apple.desktopservices", "DSDontWriteUSBStores", defBool, "true"},
	{"Finder", "com.apple.finder", "FXDefaultSearchScope", defString, "SCcf"},
	{"Finder", "com.apple.finder", "FXPreferredViewStyle", defString, "Nlsv"},
	{"Finder", "com.apple.finder", "QuitMenuItem", defBool, "true"},

	// Key repeat speed lives in the `keyboard` category (macKeyRepeat*).
	{"Keyboard", "NSGlobalDomain", "NSAutomaticQuoteSubstitutionEnabled", defBool, "false"},
	{"Keyboard", "NSGlobalDomain", "NSAutomaticDashSubstitutionEnabled", defBool, "false"},
	{"Keyboard", "NSGlobalDomain", "NSAutomaticSpellingCorrectionEnabled", defBool, "false"},
	{"Keyboard", "NSGlobalDomain", "NSAutomaticCapitalizationEnabled", defBool, "false"},
	{"Keyboard", "NSGlobalDomain", "NSAutomaticPeriodSubstitutionEnabled", defBool, "false"},
	{"Keyboard", "NSGlobalDomain", "ApplePressAndHoldEnabled", defBool, "false"},

	{"Dialogs", "NSGlobalDomain", "NSNavPanelExpandedStateForSaveMode", defBool, "true"},
	{"Dialogs", "NSGlobalDomain", "NSNavPanelExpandedStateForSaveMode2", defBool, "true"},
	{"Dialogs", "NSGlobalDomain", "PMPrintingExpandedStateForPrint", defBool, "true"},
	{"Dialogs", "NSGlobalDomain", "PMPrintingExpandedStateForPrint2", defBool, "true"},

	{"Security", "com.apple.screensaver", "askForPassword", defInt, "1"},
	{"Security", "com.apple.screensaver", "askForPasswordDelay", defInt, "0"},
}

// defaultsRead returns the trimmed output of `defaults read <domain> <key>`.
// ok is false when the read fails: macOS exits non-zero for a missing key, and
// the error text changed in macOS 27, so only the exit status is trusted.
func defaultsRead(ctx context.Context, domain, key string) (string, bool) {
	out, err := macRead(ctx, "defaults", "read", domain, key)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// defaultsWrite runs `defaults write <domain> <key> <args...>` and appends any
// failure to errs. A failure is logged to o.Stdout but doesn't abort —
// subsequent writes still run so unrelated keys apply even if one is
// SIP-restricted.
func defaultsWrite(ctx context.Context, o sysutil.Opts, errs *[]error, domain, key string, args ...string) {
	full := append([]string{"write", domain, key}, args...)
	if err := macExec(ctx, o, "defaults", full...); err != nil {
		fmt.Fprintf(o.Stdout, "warning: defaults write %s %s failed: %v\n", domain, key, err)
		*errs = append(*errs, fmt.Errorf("defaults write %s %s: %w", domain, key, err))
	}
}

// macOSDefaultsPlan returns one dry-run line per section of macOSDefaults,
// naming every key and the value it would get.
func macOSDefaultsPlan() []string {
	var lines []string
	var b strings.Builder
	section, domain := "", ""
	flush := func() {
		if b.Len() > 0 {
			lines = append(lines, b.String())
			b.Reset()
		}
	}
	for _, d := range macOSDefaults {
		if d.Section != section {
			flush()
			section, domain = d.Section, ""
			fmt.Fprintf(&b, "[dry-run] Would configure %s:", section)
		} else {
			b.WriteString(",")
		}
		if d.Domain != domain {
			domain = d.Domain
			fmt.Fprintf(&b, " %s", domain)
		}
		fmt.Fprintf(&b, " %s=%s", d.Key, d.Value)
	}
	flush()
	return lines
}

// detectMacOSDefaults reports "applied" only when every entry in
// macOSDefaults reads back at its value; a missing key is "not applied".
func detectMacOSDefaults(ctx context.Context) string {
	for _, d := range macOSDefaults {
		out, ok := defaultsRead(ctx, d.Domain, d.Key)
		if !ok || !d.matches(out) {
			return "not applied"
		}
	}
	return "applied"
}

// ApplyMacOSDefaults writes every entry in macOSDefaults (Dock, animations,
// Finder, typing, dialogs, screensaver security), then restarts Dock and
// Finder. Exposed as the `macos` configure category.
func ApplyMacOSDefaults(ctx context.Context, o sysutil.Opts) error {
	w := o.Stdout
	if o.DryRun {
		for _, line := range macOSDefaultsPlan() {
			fmt.Fprintln(w, line)
		}
		fmt.Fprintln(w, "[dry-run] Would restart Dock and Finder")
		return nil
	}

	var errs []error
	section := ""
	for _, d := range macOSDefaults {
		if d.Section != section {
			section = d.Section
			fmt.Fprintf(w, "Configuring %s...\n", section)
		}
		defaultsWrite(ctx, o, &errs, d.Domain, d.Key, d.Type.flag(), d.Value)
	}

	fmt.Fprintln(w, "Applying changes...")
	// Best-effort restarts; killall's "No matching processes" is noise.
	quiet := sysutil.Opts{Stdout: io.Discard}
	_ = macExec(ctx, quiet, "killall", "Dock")
	_ = macExec(ctx, quiet, "killall", "Finder")

	if len(errs) > 0 {
		fmt.Fprintf(w, "macOS defaults applied with %d warning(s)\n", len(errs))
		return errors.Join(errs...)
	}
	fmt.Fprintln(w, "macOS defaults configured")
	return nil
}

// keyRepeatMsToUnits converts a slider value in ms to macOS's 15 ms units,
// rounding to the nearest unit with a floor of 1.
func keyRepeatMsToUnits(ms string) (int, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(ms), 64)
	if err != nil {
		return 0, fmt.Errorf("key repeat %q: not a number of ms", ms)
	}
	units := int(math.Round(v / keyRepeatUnitMs))
	if units < 1 {
		units = 1
	}
	return units, nil
}

// detectMacKeyRepeat reads NSGlobalDomain <key> (KeyRepeat or
// InitialKeyRepeat) and returns it in ms, or "" when the key is unset.
func detectMacKeyRepeat(ctx context.Context, key string) string {
	out, ok := defaultsRead(ctx, "NSGlobalDomain", key)
	if !ok {
		return ""
	}
	units, err := strconv.Atoi(out)
	if err != nil {
		return ""
	}
	return strconv.Itoa(units * keyRepeatUnitMs)
}

// applyMacKeyRepeat writes NSGlobalDomain <key> from a value in ms.
func applyMacKeyRepeat(ctx context.Context, o sysutil.Opts, key, ms string) error {
	units, err := keyRepeatMsToUnits(ms)
	if err != nil {
		return err
	}
	if err := macExec(ctx, o, "defaults", "write", "NSGlobalDomain", key, "-int", strconv.Itoa(units)); err != nil {
		return fmt.Errorf("defaults write NSGlobalDomain %s: %w", key, err)
	}
	return nil
}

// macKeyRepeatNote is the post-apply hook for macKeyRepeatGroup.
func macKeyRepeatNote(_ context.Context, o sysutil.Opts) error {
	fmt.Fprintln(o.Stdout, "  Note: key repeat changes take effect after you log out and back in.")
	return nil
}
