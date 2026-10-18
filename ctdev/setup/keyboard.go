package setup

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// macOS key repeat rate, shown in characters per second like Cinnamon's.
// NSGlobalDomain KeyRepeat stores the time between repeats in units of
// keyRepeatUnitMs, so only the speeds in macKeyRepeatRateChoices exist.

// macKeyRepeatMaxUnits is the slowest KeyRepeat ctdev offers (120 ms, 8 cps),
// the bottom of the old ms slider.
const macKeyRepeatMaxUnits = 8

// macKeyRepeatRateDefaultCps is the recommended rate: the speed nearest the
// Linux default of 50 cps. 50 cps is a 20 ms interval; the nearest 15 ms step
// is 15 ms (KeyRepeat 1), which is 67 cps. (In cps, 67 and 33 are equally far.)
const macKeyRepeatRateDefaultCps = "67"

// keyRepeatUnitsToCps converts a KeyRepeat value to characters per second,
// rounded to a whole number. ok is false for a value below 1.
func keyRepeatUnitsToCps(units int) (string, bool) {
	if units < 1 {
		return "", false
	}
	cps := 1000 / float64(units*keyRepeatUnitMs)
	return strconv.Itoa(int(math.Round(cps))), true
}

// keyRepeatCpsToUnits converts a rate in characters per second to the nearest
// KeyRepeat value (by interval), with a floor of 1.
func keyRepeatCpsToUnits(cps string) (int, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(cps), 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("key repeat rate %q: not a positive number of characters per second", cps)
	}
	units := int(math.Round(1000 / v / keyRepeatUnitMs))
	if units < 1 {
		units = 1
	}
	return units, nil
}

// macKeyRepeatRateChoices lists every rate KeyRepeat 1..macKeyRepeatMaxUnits
// gives, fastest first.
func macKeyRepeatRateChoices() []PickerChoice {
	var choices []PickerChoice
	for u := 1; u <= macKeyRepeatMaxUnits; u++ {
		cps, _ := keyRepeatUnitsToCps(u)
		desc := fmt.Sprintf("%s characters per second (a repeat every %d ms)", cps, u*keyRepeatUnitMs)
		if u == 1 {
			desc += ", the fastest"
		}
		choices = append(choices, PickerChoice{Value: cps, Description: desc})
	}
	return choices
}

// detectMacKeyRepeatRate reads NSGlobalDomain KeyRepeat and returns it in
// characters per second, or "" when the key is unset or unreadable.
func detectMacKeyRepeatRate(ctx context.Context) string {
	out, ok := defaultsRead(ctx, "NSGlobalDomain", "KeyRepeat")
	if !ok {
		return ""
	}
	units, err := strconv.Atoi(out)
	if err != nil {
		return ""
	}
	cps, _ := keyRepeatUnitsToCps(units)
	return cps
}

// applyMacKeyRepeatRate writes NSGlobalDomain KeyRepeat from a rate in cps.
func applyMacKeyRepeatRate(ctx context.Context, o sysutil.Opts, cps string) error {
	units, err := keyRepeatCpsToUnits(cps)
	if err != nil {
		return err
	}
	if err := macExec(ctx, o, "defaults", "write", "NSGlobalDomain", "KeyRepeat", "-int", strconv.Itoa(units)); err != nil {
		return fmt.Errorf("defaults write NSGlobalDomain KeyRepeat: %w", err)
	}
	return nil
}

// macBoolKeys is an on/off setting made of NSGlobalDomain bool keys that ctdev
// sets to false. On writes every key; off deletes them, which puts back the
// macOS default.
type macBoolKeys []string

// detect returns "enabled" when every key reads back false.
func (k macBoolKeys) detect(ctx context.Context) string {
	for _, key := range k {
		out, ok := defaultsRead(ctx, "NSGlobalDomain", key)
		if !ok || out != "0" {
			return "disabled"
		}
	}
	return "enabled"
}

// apply writes every key false for "enabled" and deletes every key that is
// set for "disabled". It tries every key and joins the failures.
func (k macBoolKeys) apply(ctx context.Context, o sysutil.Opts, v string) error {
	var errs []error
	for _, key := range k {
		switch v {
		case "enabled":
			defaultsWrite(ctx, o, &errs, "NSGlobalDomain", key, "-bool", "false")
		case "disabled":
			// `defaults delete` fails on a missing key; only delete what is there.
			if _, ok := defaultsRead(ctx, "NSGlobalDomain", key); !ok {
				continue
			}
			if err := macExec(ctx, o, "defaults", "delete", "NSGlobalDomain", key); err != nil {
				errs = append(errs, fmt.Errorf("defaults delete NSGlobalDomain %s: %w", key, err))
			}
		default:
			return fmt.Errorf("unknown value %q: want enabled or disabled", v)
		}
	}
	return errors.Join(errs...)
}

// macTypeAsTyped turns off macOS's automatic text changes.
var macTypeAsTyped = macBoolKeys{
	"NSAutomaticQuoteSubstitutionEnabled",
	"NSAutomaticDashSubstitutionEnabled",
	"NSAutomaticSpellingCorrectionEnabled",
	"NSAutomaticCapitalizationEnabled",
	"NSAutomaticPeriodSubstitutionEnabled",
}

// macHoldToRepeat turns off the accent popup, so a held key repeats.
var macHoldToRepeat = macBoolKeys{"ApplePressAndHoldEnabled"}

// detectNumlockx reports whether the numlockx package is installed.
func detectNumlockx(ctx context.Context) string {
	if detectPackageInstalled(ctx, "numlockx") {
		return "installed"
	}
	return "not installed"
}

// applyNumlockx installs numlockx for "installed" and removes it otherwise.
func applyNumlockx(ctx context.Context, o sysutil.Opts, v string) error {
	if v == "installed" {
		return applyPackages(ctx, o, []string{"numlockx"})
	}
	return removePackages(ctx, o, "numlockx")
}

// removePackages is sysutil.RemovePackage; tests substitute a fake.
var removePackages = sysutil.RemovePackage
