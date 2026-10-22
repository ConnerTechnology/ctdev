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

// macKeyRepeatDelayDefaultMs is the recommended hold before repeating starts,
// for the macOS `keyboard` slider: the 15 ms step nearest the Linux default of
// 200 ms. The rate is in keyboard.go, in characters per second.
const macKeyRepeatDelayDefaultMs = "195"

// keyRepeatUnitMs is the unit of NSGlobalDomain KeyRepeat and InitialKeyRepeat.
// Apple doesn't document it: 15 ms per unit is the community's long-standing
// measurement of these keys, not an Apple spec. Both keys take any positive
// integer.
const keyRepeatUnitMs = 15

// macKeyRepeatGroup is the ApplyGroup shared by the macOS key repeat settings,
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
	Domain string
	Key    string
	Type   defaultsType
	Value  string
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

// The macOS `desktop` settings. Each table is one on/off setting: on writes
// every entry, off deletes every key so macOS falls back to its own default.
var (
	macDesktopDock = []macDefault{
		{"com.apple.dock", "autohide", defBool, "true"},
		{"com.apple.dock", "autohide-delay", defFloat, "0"},
		{"com.apple.dock", "autohide-time-modifier", defFloat, "0.15"},
		{"com.apple.dock", "show-recents", defBool, "false"},
	}
	macDesktopAnimations = []macDefault{
		{"com.apple.dock", "launchanim", defBool, "false"},
		{"com.apple.dock", "mineffect", defString, "scale"},
		{"com.apple.dock", "expose-animation-duration", defFloat, "0.1"},
		{"NSGlobalDomain", "NSAutomaticWindowAnimationsEnabled", defBool, "false"},
	}
	macDesktopFinder = []macDefault{
		{"com.apple.finder", "ShowPathbar", defBool, "true"},
		{"com.apple.finder", "ShowStatusBar", defBool, "true"},
		{"com.apple.finder", "FXDefaultSearchScope", defString, "SCcf"},
		{"com.apple.finder", "FXPreferredViewStyle", defString, "Nlsv"},
		{"com.apple.finder", "QuitMenuItem", defBool, "true"},
	}
	macDesktopDSStore = []macDefault{
		{"com.apple.desktopservices", "DSDontWriteNetworkStores", defBool, "true"},
		{"com.apple.desktopservices", "DSDontWriteUSBStores", defBool, "true"},
	}
	macDesktopDialogs = []macDefault{
		{"NSGlobalDomain", "NSNavPanelExpandedStateForSaveMode", defBool, "true"},
		{"NSGlobalDomain", "NSNavPanelExpandedStateForSaveMode2", defBool, "true"},
		{"NSGlobalDomain", "PMPrintingExpandedStateForPrint", defBool, "true"},
		{"NSGlobalDomain", "PMPrintingExpandedStateForPrint2", defBool, "true"},
	}
)

// The other macOS on/off settings, one table each, in the category of their
// topic. Like the desktop ones, on writes the table and off deletes its keys.
// None of them restarts an app.
var (
	// macAudioVolumeClick is `audio`'s "Click when changing volume".
	macAudioVolumeClick = []macDefault{
		{"NSGlobalDomain", "com.apple.sound.beep.feedback", defBool, "true"},
	}
	// macPowerScreensaverPassword is `power`'s "Ask for the password right
	// after the screensaver".
	macPowerScreensaverPassword = []macDefault{
		{"com.apple.screensaver", "askForPassword", defInt, "1"},
		{"com.apple.screensaver", "askForPasswordDelay", defInt, "0"},
	}
	// macKeyboardTypeAsTyped is `keyboard`'s "Type exactly what I type": no
	// automatic text changes.
	macKeyboardTypeAsTyped = []macDefault{
		{"NSGlobalDomain", "NSAutomaticQuoteSubstitutionEnabled", defBool, "false"},
		{"NSGlobalDomain", "NSAutomaticDashSubstitutionEnabled", defBool, "false"},
		{"NSGlobalDomain", "NSAutomaticSpellingCorrectionEnabled", defBool, "false"},
		{"NSGlobalDomain", "NSAutomaticCapitalizationEnabled", defBool, "false"},
		{"NSGlobalDomain", "NSAutomaticPeriodSubstitutionEnabled", defBool, "false"},
	}
	// macKeyboardHoldToRepeat is `keyboard`'s "Holding a key repeats it": no
	// accent popup.
	macKeyboardHoldToRepeat = []macDefault{
		{"NSGlobalDomain", "ApplePressAndHoldEnabled", defBool, "false"},
	}
)

// detectMacDefaults reports "enabled" when every entry in table reads back at
// its value, else "disabled".
func detectMacDefaults(ctx context.Context, table []macDefault) string {
	for _, d := range table {
		out, ok := defaultsRead(ctx, d.Domain, d.Key)
		if !ok || !d.matches(out) {
			return "disabled"
		}
	}
	return "enabled"
}

// applyMacDefaults writes table for "enabled" or deletes its keys for
// anything else. A delete of a key that isn't set fails harmlessly, so its
// error is ignored. The app that reads the keys restarts once per apply, in
// its ApplyGroup's post-apply hook (macRestartGroups).
func applyMacDefaults(ctx context.Context, o sysutil.Opts, table []macDefault, value string) error {
	var errs []error
	for _, d := range table {
		if value == "enabled" {
			defaultsWrite(ctx, o, &errs, d.Domain, d.Key, d.Type.flag(), d.Value)
		} else {
			_ = macExec(ctx, o, "defaults", "delete", d.Domain, d.Key)
		}
	}
	return errors.Join(errs...)
}

// ApplyGroups of the macOS desktop settings, keyed by the app each restarts.
const (
	macDockGroup   = "macos-dock"
	macFinderGroup = "macos-finder"
)

// macRestartGroups maps each group to the app its post-apply hook restarts.
var macRestartGroups = map[string]string{macDockGroup: "Dock", macFinderGroup: "Finder"}

// macRestartHook returns a post-apply hook that restarts app (best effort;
// killall's "No matching processes" is noise).
func macRestartHook(app string) PostApplyHook {
	return func(ctx context.Context, o sysutil.Opts) error {
		fmt.Fprintf(o.Stdout, "  Restarting %s...\n", app)
		_ = macExec(ctx, sysutil.Opts{Stdout: io.Discard, DryRun: o.DryRun}, "killall", app)
		return nil
	}
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

// readMacKeyRepeatUnits reads NSGlobalDomain <key> (KeyRepeat or
// InitialKeyRepeat) in its raw 15 ms units. ok is false when the key is unset
// or not an integer.
func readMacKeyRepeatUnits(ctx context.Context, key string) (int, bool) {
	out, ok := defaultsRead(ctx, "NSGlobalDomain", key)
	if !ok {
		return 0, false
	}
	units, err := strconv.Atoi(out)
	if err != nil {
		return 0, false
	}
	return units, true
}

// writeMacKeyRepeatUnits writes NSGlobalDomain <key> in its raw 15 ms units.
func writeMacKeyRepeatUnits(ctx context.Context, o sysutil.Opts, key string, units int) error {
	if err := macExec(ctx, o, "defaults", "write", "NSGlobalDomain", key, "-int", strconv.Itoa(units)); err != nil {
		return fmt.Errorf("defaults write NSGlobalDomain %s: %w", key, err)
	}
	return nil
}

// detectMacKeyRepeat reads NSGlobalDomain <key> (KeyRepeat or
// InitialKeyRepeat) and returns it in ms, or "" when the key is unset.
func detectMacKeyRepeat(ctx context.Context, key string) string {
	units, ok := readMacKeyRepeatUnits(ctx, key)
	if !ok {
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
	return writeMacKeyRepeatUnits(ctx, o, key, units)
}

// macKeyRepeatNote is the post-apply hook for macKeyRepeatGroup.
func macKeyRepeatNote(_ context.Context, o sysutil.Opts) error {
	fmt.Fprintln(o.Stdout, "  Note: key repeat changes take effect after you log out and back in.")
	return nil
}
