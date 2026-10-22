package setup

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// macOS key repeat rate, shown in characters per second like Cinnamon's.
// NSGlobalDomain KeyRepeat stores the time between repeats as a positive
// integer in units of keyRepeatUnitMs. ctdev offers the eight fastest steps
// (15–120 ms) in macKeyRepeatRateChoices; macOS itself takes slower values.

// macKeyRepeatMaxUnits is the slowest KeyRepeat ctdev offers (120 ms, 8 cps),
// the bottom of the old ms slider. It is ctdev's choice, not a macOS limit.
const macKeyRepeatMaxUnits = 8

// macKeyRepeatRateDefaultCps is the recommended rate: the speed nearest the
// Linux default of 50 cps. 50 cps is a 20 ms interval; the nearest 15 ms step
// is 15 ms (KeyRepeat 1), which is 67 cps. (In cps, 67 and 33 are equally far.)
// 67 cps is faster than System Settings' keyboard slider goes. Like the 15 ms
// unit, that is from community reports, not Apple: the slider's fastest
// setting writes KeyRepeat 2.
const macKeyRepeatRateDefaultCps = "67"

// keyRepeatUnitsToCps converts a KeyRepeat value to characters per second,
// rounded to a whole number, or to one decimal below 1 cps, never below
// "0.1", so a very slow rate never reads as 0. ok is false for a value below 1.
func keyRepeatUnitsToCps(units int) (string, bool) {
	if units < 1 {
		return "", false
	}
	cps := 1000 / float64(units*keyRepeatUnitMs)
	if cps < 1 {
		return strconv.FormatFloat(math.Max(math.Round(cps*10)/10, 0.1), 'f', 1, 64), true
	}
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

// macKeyRepeatRateChoices lists the rate each KeyRepeat 1..macKeyRepeatMaxUnits
// gives, fastest first.
func macKeyRepeatRateChoices() []PickerChoice {
	var choices []PickerChoice
	for u := 1; u <= macKeyRepeatMaxUnits; u++ {
		cps, _ := keyRepeatUnitsToCps(u)
		desc := fmt.Sprintf("%s characters per second (a repeat every %d ms)", cps, u*keyRepeatUnitMs)
		if u == 1 {
			// Community reports, not Apple: System Settings' fastest is KeyRepeat 2.
			desc += ", faster than System Settings allows"
		}
		choices = append(choices, PickerChoice{Value: cps, Description: desc})
	}
	return choices
}

// detectMacKeyRepeatRate reads NSGlobalDomain KeyRepeat and returns it in
// characters per second, or "" when the key is unset or unreadable. A value
// slower than ctdev's choices is still returned, so it shows as the current
// value without matching any choice.
func detectMacKeyRepeatRate(ctx context.Context) string {
	units, ok := readMacKeyRepeatUnits(ctx, "KeyRepeat")
	if !ok {
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
	return writeMacKeyRepeatUnits(ctx, o, "KeyRepeat", units)
}

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
