package setup

import (
	"context"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

type ControlType int

const (
	ControlToggle ControlType = iota
	ControlSlider
	ControlPicker
)

type SliderRange struct {
	Min  float64
	Max  float64
	Step float64
	Unit string // "ms", "s", "min", "cps", ""
	// Never adds a "never" value (SliderNever) one step below Min, for
	// timers where 0 means "don't do it at all".
	Never bool
}

type PickerChoice struct {
	Value       string
	Description string
}

// ApplyFunc applies a setting's desired value. Accepts ctx so Ctrl-C can
// terminate any shell-out, and sysutil.Opts so dry-run + output routing go
// through the same path as install/uninstall.
type ApplyFunc func(ctx context.Context, o sysutil.Opts, value string) error

// PostApplyHook runs once per ApplyGroup after all settings in that group apply.
type PostApplyHook func(ctx context.Context, o sysutil.Opts) error

type Setting struct {
	Name        string
	Slug        string // configure subcommand category: "gpu", "boot", "power", etc.
	Category    string // human-readable category for display grouping
	Description string // shown in wizard prompt
	Control     ControlType
	Default     string // our recommended value as string
	Slider      *SliderRange
	Choices     []PickerChoice
	DetectFunc  func(ctx context.Context) string // reads current system value
	ApplyFunc   ApplyFunc                        // writes value to system
	HardwareFn  func() bool                      // optional; setting hidden when returns false
	ApplyGroup  string                           // settings sharing a group run one post-apply hook
	// OneWay marks install/enable-style settings whose ApplyFunc has no off
	// path (it ignores the value). The wizard offers these as a single
	// "apply?" question instead of a fake enable/disable choice.
	OneWay bool
	// NoOffSwitch, on a OneWay setting, says why it has no off switch. The
	// wizard prints it.
	NoOffSwitch string
	// NotNeededFn reports why this machine doesn't need the setting, or ""
	// when it does. A setting that isn't needed is shown as such and is left
	// out of what --batch applies.
	NotNeededFn func(ctx context.Context) string
}

// SettingState holds runtime state for a setting during TUI interaction.
type SettingState struct {
	Setting      *Setting
	CurrentValue string // detected system value
	DesiredValue string // what will be applied (initialized to Default)
	Enabled      bool   // whether included in apply set
	NotNeeded    string // why this machine doesn't need the setting; "" when it does
}

// NeedsApply returns true if this setting should be applied.
func (inst *SettingState) NeedsApply(force bool) bool {
	if !inst.Enabled {
		return false
	}
	if force {
		return true
	}
	return inst.CurrentValue != inst.DesiredValue
}

// PostApplyHooks maps ApplyGroup names to functions run after all settings in the group are applied.
var PostApplyHooks = map[string]PostApplyHook{}

// FilterByHardware returns only settings whose HardwareFn is nil or returns true.
func FilterByHardware(settings []Setting) []Setting {
	var result []Setting
	for i := range settings {
		if settings[i].HardwareFn == nil || settings[i].HardwareFn() {
			result = append(result, settings[i])
		}
	}
	return result
}

// InitStates creates SettingState for each setting, detecting current values.
func InitStates(ctx context.Context, settings []Setting) []SettingState {
	states := make([]SettingState, len(settings))
	for i := range settings {
		current := ""
		if settings[i].DetectFunc != nil {
			current = settings[i].DetectFunc(ctx)
		}
		notNeeded := ""
		if settings[i].NotNeededFn != nil {
			notNeeded = settings[i].NotNeededFn(ctx)
		}
		states[i] = SettingState{
			Setting:      &settings[i],
			CurrentValue: current,
			DesiredValue: settings[i].Default,
			// A setting this machine doesn't need isn't applied by default.
			Enabled:   notNeeded == "",
			NotNeeded: notNeeded,
		}
	}
	return states
}

// FilterBySlug returns only settings matching the given slug.
func FilterBySlug(settings []Setting, slug string) []Setting {
	var result []Setting
	for i := range settings {
		if settings[i].Slug == slug {
			result = append(result, settings[i])
		}
	}
	return result
}

// Slugs returns the ordered list of unique slugs from settings.
func Slugs(settings []Setting) []string {
	seen := make(map[string]bool)
	var slugs []string
	for _, s := range settings {
		if s.Slug != "" && !seen[s.Slug] {
			seen[s.Slug] = true
			slugs = append(slugs, s.Slug)
		}
	}
	return slugs
}

// Categories returns the ordered list of unique categories from settings.
func Categories(settings []Setting) []string {
	seen := make(map[string]bool)
	var cats []string
	for _, s := range settings {
		if !seen[s.Category] {
			seen[s.Category] = true
			cats = append(cats, s.Category)
		}
	}
	return cats
}
