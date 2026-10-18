package setup

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// neverSuspendName is the power setting that masks the sleep targets.
const neverSuspendName = "Never suspend, even when asked"

// sleepTargets are the systemd targets masked to keep an always-on box awake.
var sleepTargets = []string{"sleep.target", "suspend.target", "hibernate.target", "hybrid-sleep.target"}

// minutesTimer is the range of power's timers: 5 to 120 minutes, or never.
// A function so each setting gets its own copy.
func minutesTimer() *SliderRange {
	return &SliderRange{Min: 5, Max: 120, Step: 5, Unit: "min", Never: true}
}

// systemctlIsEnabled returns `systemctl is-enabled <unit>`'s stdout. It prints
// "masked" with a non-zero exit, so stdout is read rather than the error.
// A var so tests can fake it.
var systemctlIsEnabled = func(ctx context.Context, unit string) string {
	out, _ := exec.CommandContext(ctx, "systemctl", "is-enabled", unit).Output()
	return strings.TrimSpace(string(out))
}

// SuspendMasked reports whether every sleep target is masked, i.e. "Never
// suspend, even when asked" is on and this machine cannot suspend. Other
// categories call it to decide whether a suspend-related setting is needed.
func SuspendMasked(ctx context.Context) bool {
	for _, t := range sleepTargets {
		if systemctlIsEnabled(ctx, t) != "masked" {
			return false
		}
	}
	return true
}

// detectSuspendMasked reports "enabled" when all sleep targets are masked.
func detectSuspendMasked(ctx context.Context) string {
	if SuspendMasked(ctx) {
		return "enabled"
	}
	return "disabled"
}

// applyNeverSuspend masks the sleep targets for "enabled" and unmasks them
// for "disabled", which removes exactly what enabling added.
func applyNeverSuspend(ctx context.Context, o sysutil.Opts, value string) error {
	var verb string
	switch value {
	case "enabled":
		verb = "mask"
	case "disabled":
		verb = "unmask"
	default:
		return fmt.Errorf("never suspend: unknown value %q", value)
	}
	return sysutil.SudoRun(ctx, o, "systemctl", append([]string{verb}, sleepTargets...)...)
}

// gvariantTypes are the type prefixes `dconf read` prints when a value's type
// isn't the default for its literal, e.g. "uint32 0" for idle-delay.
var gvariantTypes = map[string]bool{
	"byte": true, "int16": true, "uint16": true, "int32": true,
	"uint32": true, "int64": true, "uint64": true, "double": true,
}

// stripGVariantType drops a leading GVariant type annotation ("uint32 0" → "0").
func stripGVariantType(v string) string {
	typ, rest, ok := strings.Cut(strings.TrimSpace(v), " ")
	if ok && gvariantTypes[typ] {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(v)
}

// dconfSecondsToMinutes renders a dconf timer in seconds as minutes for a
// SliderRange with Never: 0 is SliderNever. Unset keys stay empty and values
// that aren't numbers are shown as read.
func dconfSecondsToMinutes(raw string) string {
	v := stripGVariantType(raw)
	if v == "" {
		return ""
	}
	sec, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return v
	}
	if sec == 0 {
		return SliderNever
	}
	return strconv.FormatFloat(sec/60, 'f', -1, 64)
}

// minutesToDconf converts a minutes slider value (or SliderNever) to the
// seconds dconf stores, prefixed with gvType when the key isn't an int32.
func minutesToDconf(value, gvType string) (string, error) {
	sec := 0
	if value != SliderNever {
		mins, err := strconv.ParseFloat(value, 64)
		if err != nil || mins < 0 {
			return "", fmt.Errorf("invalid minutes %q", value)
		}
		sec = int(math.Round(mins * 60))
	}
	if gvType != "" {
		return gvType + " " + strconv.Itoa(sec), nil
	}
	return strconv.Itoa(sec), nil
}

// detectDconfMinutes reads a dconf timer in seconds and reports it in minutes.
func detectDconfMinutes(ctx context.Context, path string) string {
	return dconfSecondsToMinutes(detectDconfInt(ctx, path))
}

// applyDconfMinutes writes a minutes slider value to a dconf timer in seconds.
func applyDconfMinutes(ctx context.Context, o sysutil.Opts, path, value, gvType string) error {
	v, err := minutesToDconf(value, gvType)
	if err != nil {
		return err
	}
	return applyDconf(ctx, o, path, v)
}
