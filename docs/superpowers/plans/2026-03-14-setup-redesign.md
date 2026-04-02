# Setup Redesign Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the multi-step setup wizard with a unified single-screen interactive settings view with info modals.

**Architecture:** A Settings Registry defines all 27 settings as data structs with detect/apply functions. A new Bubble Tea v2 TUI (`tui/setup/`) renders the main list and modal overlays in two modes (interactive and readonly). The existing `cmd/setup.go` is simplified to wire hardware detection, TUI launch, and apply logic. macOS retains its existing bash-based path (`macos_apply`).

**Tech Stack:** Go, Cobra, Bubble Tea v2 (`charm.land/bubbletea/v2`), Lipgloss v2 (`charm.land/lipgloss/v2`)

**Spec:** `docs/superpowers/specs/2026-03-14-setup-redesign-design.md`

---

## File Structure

### New Files
| File | Responsibility |
|------|---------------|
| `ctdev/setup/setting.go` | Setting type, ControlType enum, SliderRange, PickerChoice structs |
| `ctdev/setup/registry.go` | All 27 settings defined as data with detect/apply function references |
| `ctdev/setup/detect.go` | Detection functions — read current system values via exec/os |
| `ctdev/setup/detect_test.go` | Tests for detection functions that don't require system access |
| `ctdev/setup/apply.go` | Apply functions — write values to system via exec |
| `ctdev/setup/apply_test.go` | Tests for apply logic (dry-run mode, argument construction) |
| `ctdev/tui/setup/model.go` | Bubble Tea model: main list view, scrolling, mode, state |
| `ctdev/tui/setup/model_test.go` | Tests for navigation, toggling, mode switching |
| `ctdev/tui/setup/modal.go` | Info modal overlay: slider, picker, toggle, readonly variants |
| `ctdev/tui/setup/modal_test.go` | Tests for modal controls (slider ←/→, picker ↑/↓, default reset) |
| `ctdev/tui/setup/confirm.go` | Confirmation screen showing diff before apply |
| `ctdev/tui/setup/confirm_test.go` | Tests for confirmation rendering and navigation |

### Modified Files
| File | Changes |
|------|---------|
| `ctdev/cmd/setup.go` | Rewrite: remove wizard, dashboard, helpers. Wire new setup TUI + apply logic |

### Removed Files
| File | Reason |
|------|--------|
| `ctdev/tui/wizard/wizard.go` | Replaced by `tui/setup/` |
| `ctdev/tui/wizard/wizard_test.go` | Replaced by `tui/setup/` tests |

---

## Chunk 1: Settings Registry

### Task 1: Setting Type Definition

**Files:**
- Create: `ctdev/setup/setting.go`

- [ ] **Step 1: Create the setting.go file with all type definitions**

```go
package setup

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
}

type PickerChoice struct {
	Value       string
	Description string
}

type Setting struct {
	Name        string
	Category    string
	Description string       // shown in info modal body
	TechDetail  string       // underlying commands/paths, shown in modal footer
	Control     ControlType
	Default     string       // our recommended value as string
	Slider      *SliderRange
	Choices     []PickerChoice
	DetectFunc  func() string        // reads current system value
	ApplyFunc   func(value string) error // writes value to system
	HardwareFn  func() bool          // optional; setting hidden when returns false
	ApplyGroup  string               // settings sharing a group run one post-apply hook
}

// SettingState holds runtime state for a setting during TUI interaction.
type SettingState struct {
	Setting      *Setting
	CurrentValue string // detected system value
	DesiredValue string // what will be applied (initialized to Default)
	Enabled      bool   // whether included in apply set
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
var PostApplyHooks = map[string]func() error{}

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
func InitStates(settings []Setting) []SettingState {
	states := make([]SettingState, len(settings))
	for i := range settings {
		current := ""
		if settings[i].DetectFunc != nil {
			current = settings[i].DetectFunc()
		}
		states[i] = SettingState{
			Setting:      &settings[i],
			CurrentValue: current,
			DesiredValue: settings[i].Default,
			Enabled:      true,
		}
	}
	return states
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
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build ./setup/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add ctdev/setup/setting.go
git commit -m "feat: add setup settings type definitions"
```

### Task 2: Detection Functions

**Files:**
- Create: `ctdev/setup/detect.go`
- Create: `ctdev/setup/detect_test.go`

- [ ] **Step 1: Write detect_test.go with tests for pure helper functions**

Test the helper functions that don't require system access (parsing, string extraction). Detection functions that call `exec.Command` are tested via integration in the TUI.

```go
package setup

import "testing"

func TestParseXsetRepeat(t *testing.T) {
	input := "    auto repeat delay:  200    repeat rate:  50"
	delay, rate := parseXsetRepeat(input)
	if delay != "200" {
		t.Errorf("expected delay 200, got %s", delay)
	}
	if rate != "50" {
		t.Errorf("expected rate 50, got %s", rate)
	}
}

func TestParseXsetRepeatEmpty(t *testing.T) {
	delay, rate := parseXsetRepeat("some other line")
	if delay != "" || rate != "" {
		t.Errorf("expected empty, got %s %s", delay, rate)
	}
}

func TestParseGrubVar(t *testing.T) {
	content := `GRUB_TIMEOUT_STYLE=menu
GRUB_TIMEOUT=10
# GRUB_DISABLE_OS_PROBER=true
GRUB_DISABLE_OS_PROBER=false`

	if v := parseGrubVar(content, "GRUB_TIMEOUT"); v != "10" {
		t.Errorf("expected 10, got %s", v)
	}
	if v := parseGrubVar(content, "GRUB_TIMEOUT_STYLE"); v != "menu" {
		t.Errorf("expected menu, got %s", v)
	}
	// Should get uncommented value, not commented one
	if v := parseGrubVar(content, "GRUB_DISABLE_OS_PROBER"); v != "false" {
		t.Errorf("expected false, got %s", v)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./setup/ -v -run TestParse`
Expected: FAIL — functions not defined

- [ ] **Step 3: Write detect.go with all detection functions**

Create `ctdev/setup/detect.go` with:

Many of these functions already exist in `cmd/setup.go` (e.g., `detectGRUBConfig`, `detectKeyRepeat`, `detectPackageInstalled`, `detectMT7925E`, `detectSecureBoot`, `detectBluetooth`). Move them to the `setup` package and adapt signatures to match the `func() string` pattern expected by `Setting.DetectFunc`. New functions are also needed for settings not previously detected.

- `parseXsetRepeat(line string) (delay, rate string)` — extracts delay/rate from xset output
- `parseGrubVar(content, varName string) string` — extracts GRUB variable from /etc/default/grub content
- `detectNvidiaLoaded() bool` — checks `lsmod | grep nvidia`
- `detectMT7925E() bool` — checks PCI device 14c3:0717 or mt7925e module
- `detectGrubTimeout() string` — reads GRUB_TIMEOUT from /etc/default/grub
- `detectGrubStyle() string` — reads GRUB_TIMEOUT_STYLE
- `detectGrubOSProber() string` — reads GRUB_DISABLE_OS_PROBER, returns "enabled"/"disabled"
- `detectPowerProfile() string` — runs `powerprofilesctl get`
- `detectDconfInt(path string) string` — reads dconf integer value
- `detectDconfBool(path string) string` — reads dconf boolean value
- `detectDconfString(path string) string` — reads dconf string value
- `detectGsettingsString(schema, key string) string` — reads gsettings value
- `detectKeyRepeatDelay() string` — uses xset q output
- `detectKeyRepeatRate() string` — uses xset q output
- `detectModuleSigned() string` — checks NVIDIA module signing status
- `detectNvidiaSuspendServices() string` — checks systemd service status
- `detectSystemdService(name string) string` — checks if systemd service is active
- `detectPackageInstalled(pkg string) bool` — checks dpkg -s
- `detectFileExists(path string) string` — returns "installed"/"not installed"
- `detectMouseSpeed() string` — reads dconf mouse speed
- `detectNaturalScroll() string` — reads dconf natural-scroll

Each detection function returns a string representation of the current value (e.g., "200", "menu", "enabled", "installed", "0.65").

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./setup/ -v -run TestParse`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add ctdev/setup/detect.go ctdev/setup/detect_test.go
git commit -m "feat: add setup detection functions"
```

### Task 3: Apply Functions

**Files:**
- Create: `ctdev/setup/apply.go`
- Create: `ctdev/setup/apply_test.go`

- [ ] **Step 1: Write apply_test.go**

Test the command-building helpers (not actual system execution):

```go
package setup

import "testing"

func TestGrubVarCommand(t *testing.T) {
	args := grubVarArgs("GRUB_TIMEOUT", "10")
	// Should produce sed command args for /etc/default/grub
	if len(args) == 0 {
		t.Error("expected non-empty args")
	}
}

func TestDconfWriteArgs(t *testing.T) {
	args := dconfWriteArgs("/org/cinnamon/desktop/sound/event-sounds", "false")
	if args[0] != "dconf" {
		t.Errorf("expected dconf, got %s", args[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./setup/ -v -run TestGrub`
Expected: FAIL

- [ ] **Step 3: Write apply.go with all apply functions**

Create `ctdev/setup/apply.go` with:

- `applyGrubVar(varName, value string) error` — uses sed to set GRUB variable
- `applyDconfInt(path, value string) error` — runs `dconf write`
- `applyDconfBool(path, value string) error` — runs `dconf write`
- `applyDconfString(path, value string) error` — runs `dconf write`
- `applyGsettings(schema, key, value string) error` — runs `gsettings set`
- `applyPowerProfile(value string) error` — runs `powerprofilesctl set`
- `applyKeyRepeat(delay, rate string) error` — runs `xset r rate` + gsettings
- `applySystemdEnable(service string) error` — runs `systemctl enable && start`
- `applyPackages(packages []string) error` — runs `apt-get install`
- `applyNvidiaSigning() error` — runs GPU setup script
- `applyNvidiaSuspendServices() error` — enables NVIDIA systemd services
- `applyWifiSuspendFix() error` — writes systemd sleep hook file
- `applyXbindkeys(dotfilesRoot string) error` — installs xbindkeys + config
- `applyWireplumberLDAC(dotfilesRoot string) error` — copies wireplumber config
- `applySSDTrim() error` — enables fstrim.timer
- `applyUpdateGrub() error` — post-apply hook for grub group
- Helper functions: `grubVarArgs()`, `dconfWriteArgs()`, `sudoRun()`, `run()`

All apply functions use `exec.Command`. Functions requiring root use a `sudoRun(args ...string) error` helper that prepends `"sudo"` to the command. The `ensureSudo()` in `cmd/root.go` caches credentials before the TUI starts, so `sudo` calls here won't block. A `run(name string, args ...string) error` helper handles non-sudo commands. Functions that need the dotfiles root read `setup.DotfilesRoot`.

- [ ] **Step 4: Run tests**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./setup/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add ctdev/setup/apply.go ctdev/setup/apply_test.go
git commit -m "feat: add setup apply functions"
```

### Task 4: Settings Registry

**Files:**
- Create: `ctdev/setup/registry.go`

- [ ] **Step 1: Create registry.go with all 27 settings**

Define `var Registry = []Setting{...}` with all settings from the spec. Each setting references detect/apply functions from the previous tasks. Group by category in order: GPU & Boot, Power & Display, Input Devices, Audio & Bluetooth, Desktop, Network & System.

Key details:
- NVIDIA settings: `HardwareFn: detectNvidiaLoaded`
- WiFi suspend fix: `HardwareFn: detectMT7925E`
- GRUB settings: `ApplyGroup: "grub"`
- Each slider has `Slider: &SliderRange{...}` with Min/Max/Step/Unit from spec
- Each picker has `Choices: []PickerChoice{...}` with Value/Description
- `PostApplyHooks["grub"] = applyUpdateGrub` initialized in `init()`

Note: Some apply functions need the dotfiles root path. Pass this via a package-level variable set at startup: `var DotfilesRoot string`

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build ./setup/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add ctdev/setup/registry.go
git commit -m "feat: add setup settings registry with 27 settings"
```

---

## Chunk 2: TUI — Main List View

### Task 5: Stub Modal and Confirm Types

**Files:**
- Create: `ctdev/tui/setup/modal.go`
- Create: `ctdev/tui/setup/confirm.go`

model.go references ModalModel and ConfirmModel. Create stub files first so the package compiles incrementally.

- [ ] **Step 1: Create modal.go stub**

```go
package setup

import (
	s "github.com/ConnerTechnology/dotfiles/ctdev/setup"
)

// ModalModel handles the info modal overlay.
type ModalModel struct {
	state  *s.SettingState
	mode   Mode
	closed bool
}

func NewModal(state *s.SettingState, mode Mode) ModalModel {
	return ModalModel{state: state, mode: mode}
}

func (inst *ModalModel) Closed() bool { return inst.closed }
```

- [ ] **Step 2: Create confirm.go stub**

```go
package setup

import (
	s "github.com/ConnerTechnology/dotfiles/ctdev/setup"
)

// ConfirmModel handles the confirmation screen before applying changes.
type ConfirmModel struct {
	changes   []changeEntry
	confirmed bool
	cancelled bool
	dryRun    bool
}

type changeEntry struct {
	name string
	from string
	to   string
}

func NewConfirm(states []s.SettingState, dryRun bool) ConfirmModel {
	var changes []changeEntry
	for _, st := range states {
		if st.NeedsApply(false) {
			changes = append(changes, changeEntry{name: st.Setting.Name, from: st.CurrentValue, to: st.DesiredValue})
		}
	}
	return ConfirmModel{changes: changes, dryRun: dryRun}
}

func (inst *ConfirmModel) Confirmed() bool  { return inst.confirmed }
func (inst *ConfirmModel) Cancelled() bool   { return inst.cancelled }
```

- [ ] **Step 3: Verify stubs compile**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build ./tui/setup/`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add ctdev/tui/setup/modal.go ctdev/tui/setup/confirm.go
git commit -m "feat: add stub types for setup modal and confirm"
```

### Task 6: Main List Model

**Files:**
- Create: `ctdev/tui/setup/model.go`
- Create: `ctdev/tui/setup/model_test.go`

- [ ] **Step 1: Write model_test.go**

```go
package setup

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	s "github.com/ConnerTechnology/dotfiles/ctdev/setup"
)

func testStates() []s.SettingState {
	return []s.SettingState{
		{Setting: &s.Setting{Name: "Test Toggle", Category: "Cat A", Control: s.ControlToggle, Default: "on"}, CurrentValue: "off", DesiredValue: "on", Enabled: true},
		{Setting: &s.Setting{Name: "Test Slider", Category: "Cat A", Control: s.ControlSlider, Default: "200", Slider: &s.SliderRange{Min: 100, Max: 1000, Step: 25, Unit: "ms"}}, CurrentValue: "500", DesiredValue: "200", Enabled: true},
		{Setting: &s.Setting{Name: "Other Setting", Category: "Cat B", Control: s.ControlToggle, Default: "on"}, CurrentValue: "on", DesiredValue: "on", Enabled: true},
	}
}

func TestNavigateDown(t *testing.T) {
	m := New(testStates(), ModeInteractive)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model := updated.(*Model)
	if model.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", model.cursor)
	}
}

func TestNavigateUpAtTop(t *testing.T) {
	m := New(testStates(), ModeInteractive)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	model := updated.(*Model)
	if model.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", model.cursor)
	}
}

func TestToggleSpace(t *testing.T) {
	m := New(testStates(), ModeInteractive)
	updated, _ := m.Update(tea.KeyPressMsg{Code: -1, Text: " "})
	model := updated.(*Model)
	if model.states[0].Enabled {
		t.Error("expected first item disabled after space")
	}
}

func TestToggleDisabledInReadonly(t *testing.T) {
	m := New(testStates(), ModeReadonly)
	updated, _ := m.Update(tea.KeyPressMsg{Code: -1, Text: " "})
	model := updated.(*Model)
	if !model.states[0].Enabled {
		t.Error("space should not toggle in readonly mode")
	}
}

func TestQuitOnQ(t *testing.T) {
	m := New(testStates(), ModeInteractive)
	_, cmd := m.Update(tea.KeyPressMsg{Code: -1, Text: "q"})
	if cmd == nil {
		t.Error("expected quit command")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./tui/setup/ -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Write model.go**

Create `ctdev/tui/setup/model.go` implementing:

```go
package setup

type Mode int
const (
	ModeInteractive Mode = iota
	ModeReadonly
)

type viewState int
const (
	viewList viewState = iota
	viewModal
	viewConfirm
)

type Model struct {
	states    []setup.SettingState
	mode      Mode
	cursor    int
	view      viewState
	modal     *ModalModel    // non-nil when modal is open
	confirm   *ConfirmModel  // non-nil when confirm screen is shown
	offset    int            // scroll offset
	width     int
	height    int
	quitting  bool
	applied   bool
}
```

**Init()**: return nil

**Update(msg)**:
- `tea.WindowSizeMsg`: update width/height
- `tea.KeyPressMsg` (when `view == viewList`):
  - `"q"`, `"ctrl+c"`: quit
  - `"up"`, `"k"`: move cursor up (skip category headers), clamp at 0
  - `"down"`, `"j"`: move cursor down (skip category headers), clamp at len-1
  - `"space"`: toggle enabled (interactive mode only)
  - `"i"`: open info modal for current setting
  - `"enter"`: show confirmation screen (interactive mode only)
- When `view == viewModal`: delegate to modal.Update(), on close set view back to viewList
- When `view == viewConfirm`: delegate to confirm.Update(), on confirm set applied=true and quit, on cancel set view back to viewList

**View()**:
- If `view == viewModal`: render main list dimmed + modal overlay centered
- If `view == viewConfirm`: render confirmation screen
- Otherwise render main list:
  - Title: "System Setup" (interactive) or "System Configuration" (readonly)
  - Help line with available keys
  - Settings grouped by category with orange category headers
  - Each setting: toggle indicator (interactive only) + name + value (colored green/yellow)
  - Cursor highlight on current row
  - Viewport scrolling: keep cursor visible with 3 lines context

**GetResult()**: returns whether applied or quit

- [ ] **Step 4: Run tests**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./tui/setup/ -v -run TestNavigate`
Expected: PASS (at least navigation tests; modal/confirm tests may need stubs)

- [ ] **Step 5: Commit**

```bash
git add ctdev/tui/setup/model.go ctdev/tui/setup/model_test.go
git commit -m "feat: add setup TUI main list model"
```

---

## Chunk 3: TUI — Modal and Confirmation

### Task 7: Info Modal (Full Implementation)

**Files:**
- Modify: `ctdev/tui/setup/modal.go` (replace stub)
- Create: `ctdev/tui/setup/modal_test.go`

- [ ] **Step 1: Write modal_test.go**

```go
package setup

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	s "github.com/ConnerTechnology/dotfiles/ctdev/setup"
)

func TestSliderLeft(t *testing.T) {
	state := &s.SettingState{
		Setting:      &s.Setting{Name: "Delay", Control: s.ControlSlider, Default: "200", Slider: &s.SliderRange{Min: 100, Max: 1000, Step: 25, Unit: "ms"}},
		DesiredValue: "200",
	}
	m := NewModal(state, ModeInteractive)
	updated := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if updated.state.DesiredValue != "175" {
		t.Errorf("expected 175, got %s", updated.state.DesiredValue)
	}
}

func TestSliderClampMin(t *testing.T) {
	state := &s.SettingState{
		Setting:      &s.Setting{Name: "Delay", Control: s.ControlSlider, Default: "200", Slider: &s.SliderRange{Min: 100, Max: 1000, Step: 25, Unit: "ms"}},
		DesiredValue: "100",
	}
	m := NewModal(state, ModeInteractive)
	updated := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if updated.state.DesiredValue != "100" {
		t.Errorf("expected 100 (clamped), got %s", updated.state.DesiredValue)
	}
}

func TestPickerDown(t *testing.T) {
	state := &s.SettingState{
		Setting: &s.Setting{
			Name: "Profile", Control: s.ControlPicker, Default: "performance",
			Choices: []s.PickerChoice{{Value: "performance"}, {Value: "balanced"}, {Value: "power-saver"}},
		},
		DesiredValue: "performance",
	}
	m := NewModal(state, ModeInteractive)
	updated := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if updated.state.DesiredValue != "balanced" {
		t.Errorf("expected balanced, got %s", updated.state.DesiredValue)
	}
}

func TestDefaultReset(t *testing.T) {
	state := &s.SettingState{
		Setting:      &s.Setting{Name: "Delay", Control: s.ControlSlider, Default: "200", Slider: &s.SliderRange{Min: 100, Max: 1000, Step: 25, Unit: "ms"}},
		DesiredValue: "500",
	}
	m := NewModal(state, ModeInteractive)
	updated := m.Update(tea.KeyPressMsg{Code: -1, Text: "d"})
	if updated.state.DesiredValue != "200" {
		t.Errorf("expected 200 (default), got %s", updated.state.DesiredValue)
	}
}

func TestReadonlyNoAdjust(t *testing.T) {
	state := &s.SettingState{
		Setting:      &s.Setting{Name: "Delay", Control: s.ControlSlider, Default: "200", Slider: &s.SliderRange{Min: 100, Max: 1000, Step: 25, Unit: "ms"}},
		DesiredValue: "200",
	}
	m := NewModal(state, ModeReadonly)
	updated := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if updated.state.DesiredValue != "200" {
		t.Errorf("readonly should not change value, got %s", updated.state.DesiredValue)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./tui/setup/ -v -run TestSlider`
Expected: FAIL

- [ ] **Step 3: Write modal.go**

Create `ctdev/tui/setup/modal.go`:

```go
type ModalModel struct {
	state      *setup.SettingState
	mode       Mode
	closed     bool
	pickerIdx  int // for picker navigation
}
```

**NewModal(state, mode)**: Initialize modal. For picker, find current pickerIdx from DesiredValue.

ModalModel is a plain struct, NOT a tea.Model. It has a custom `Update(msg tea.KeyPressMsg) *ModalModel` method that returns itself (pointer receiver mutates in place). The parent Model delegates key events to it when the modal is open.

**Update(msg tea.KeyPressMsg) *ModalModel**:
- `"esc"`: set closed=true
- Interactive mode only:
  - Slider: `"left"`/`"right"` adjust DesiredValue by Step, clamp to Min/Max
  - Picker: `"up"`/`"down"` move pickerIdx, update DesiredValue
  - Toggle: `"space"` toggles Enabled
  - `"d"`: reset DesiredValue to Default

**View()** returns the modal box string:
- Bordered box (lipgloss rounded border) centered on screen
- Title (blue bold): setting name
- Current value line: "Current: {currentValue}"  (interactive) or "Current: {current}     Default: {default}" (readonly)
- Description text (dimmed)
- Control section (interactive only):
  - Slider: value display + ASCII slider bar + range labels
  - Picker: radio list with ◉/○ indicators
  - Toggle: just Space hint
- Tech detail footer (dimmed, smaller): underlying commands/paths
- Help line: available keys

Helper: `renderSliderBar(value, min, max float64, width int) string` — renders `◂━━●━━━━━━━━━━━▸`

- [ ] **Step 4: Run tests**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./tui/setup/ -v -run "TestSlider|TestPicker|TestDefault|TestReadonly"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add ctdev/tui/setup/modal.go ctdev/tui/setup/modal_test.go
git commit -m "feat: add setup TUI info modal with slider/picker/toggle"
```

### Task 8: Confirmation Screen (Full Implementation)

**Files:**
- Modify: `ctdev/tui/setup/confirm.go` (replace stub)
- Create: `ctdev/tui/setup/confirm_test.go`

- [ ] **Step 1: Write confirm_test.go**

```go
package setup

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	s "github.com/ConnerTechnology/dotfiles/ctdev/setup"
)

func TestConfirmShowsDiff(t *testing.T) {
	states := []s.SettingState{
		{Setting: &s.Setting{Name: "GRUB timeout"}, CurrentValue: "5", DesiredValue: "10", Enabled: true},
		{Setting: &s.Setting{Name: "Key repeat"}, CurrentValue: "500", DesiredValue: "200", Enabled: true},
		{Setting: &s.Setting{Name: "Already good"}, CurrentValue: "on", DesiredValue: "on", Enabled: true},
	}
	m := NewConfirm(states, false)
	if len(m.changes) != 2 {
		t.Errorf("expected 2 changes, got %d", len(m.changes))
	}
}

func TestConfirmEnter(t *testing.T) {
	states := []s.SettingState{
		{Setting: &s.Setting{Name: "Test"}, CurrentValue: "off", DesiredValue: "on", Enabled: true},
	}
	m := NewConfirm(states, false)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.confirmed {
		t.Error("expected confirmed on enter")
	}
}

func TestConfirmEsc(t *testing.T) {
	states := []s.SettingState{
		{Setting: &s.Setting{Name: "Test"}, CurrentValue: "off", DesiredValue: "on", Enabled: true},
	}
	m := NewConfirm(states, false)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.cancelled {
		t.Error("expected cancelled on esc")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./tui/setup/ -v -run TestConfirm`
Expected: FAIL

- [ ] **Step 3: Write confirm.go**

Create `ctdev/tui/setup/confirm.go`:

```go
type ConfirmModel struct {
	changes   []changeEntry
	confirmed bool
	cancelled bool
	dryRun    bool
}

type changeEntry struct {
	name    string
	from    string
	to      string
}
```

**NewConfirm(states, dryRun)**: Filter states to only those where `NeedsApply(false)` is true. Build changeEntry list.

ConfirmModel is a plain struct with pointer receiver `Update(msg tea.KeyPressMsg)`. Not a tea.Model.

**Update(msg tea.KeyPressMsg)**:
- `"enter"`: set confirmed=true
- `"esc"`: set cancelled=true
- `"q"`, `"ctrl+c"`: set cancelled=true

**View()**: Render:
```
Apply Changes?

  GRUB timeout                    5s → 10s
  Key repeat delay                500ms → 200ms

2 settings will be applied. Enter to confirm · Esc to go back
```

If dryRun, show `[dry-run] Would apply:` header and no Enter prompt.

- [ ] **Step 4: Run tests**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./tui/setup/ -v -run TestConfirm`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add ctdev/tui/setup/confirm.go ctdev/tui/setup/confirm_test.go
git commit -m "feat: add setup confirmation screen"
```

---

## Chunk 4: Wire It Together

### Task 9: Rewrite cmd/setup.go

**Files:**
- Modify: `ctdev/cmd/setup.go`

- [ ] **Step 1: Rewrite setup.go**

Replace the entire file. The new version:

```go
package cmd

import (
	"fmt"
	"os"
	"os/exec"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/setup"
	tuisetup "github.com/ConnerTechnology/dotfiles/ctdev/tui/setup"
	"github.com/spf13/cobra"
)

var (
	flagSetupShow  bool
	flagSetupReset bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure system settings",
	Long:  "Interactive system configuration. Linux Mint and macOS supported.",
	RunE:  runSetup,
}

func init() {
	setupCmd.Flags().BoolVar(&flagSetupShow, "show", false, "show current system configuration (read-only)")
	setupCmd.Flags().BoolVar(&flagSetupReset, "reset", false, "reset configuration to system defaults")
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	info := platform.Detect()

	// macOS uses the existing bash-based setup path
	if info.OS == platform.MacOS {
		return runMacOSSetup()
	}

	setup.DotfilesRoot = dotfilesRoot()

	// Filter settings by detected hardware
	settings := setup.FilterByHardware(setup.Registry)
	states := setup.InitStates(settings)

	if flagSetupReset {
		return runSetupReset()
	}

	mode := tuisetup.ModeInteractive
	if flagSetupShow {
		mode = tuisetup.ModeReadonly
	}

	if isBatchMode() && mode == tuisetup.ModeInteractive {
		return runBatchSetup(states)
	}

	m := tuisetup.New(states, mode)
	p := tea.NewProgram(&m)
	result, err := p.Run()
	resetTerminal()
	if err != nil {
		return err
	}

	model := result.(*tuisetup.Model)
	if !model.Applied() {
		return nil
	}

	return applySettings(model.States(), flagForce, flagDryRun, flagVerbose)
}

func applySettings(states []setup.SettingState, force, dryRun, verbose bool) error {
	if !dryRun {
		ensureSudo()
	}

	appliedGroups := make(map[string]bool)
	var applied, failed int

	for i := range states {
		if !states[i].NeedsApply(force) {
			continue
		}
		s := states[i].Setting

		if dryRun {
			fmt.Printf("  [dry-run] %s: %s → %s\n", s.Name, states[i].CurrentValue, states[i].DesiredValue)
			applied++
			continue
		}

		if verbose {
			fmt.Printf("  Applying: %s (%s → %s)\n", s.Name, states[i].CurrentValue, states[i].DesiredValue)
		} else {
			fmt.Printf("  Applying: %s\n", s.Name)
		}

		if s.ApplyFunc != nil {
			if err := s.ApplyFunc(states[i].DesiredValue); err != nil {
				fmt.Printf("  ✗ %s: %v\n", s.Name, err)
				failed++
				continue
			}
		}
		applied++

		if s.ApplyGroup != "" {
			appliedGroups[s.ApplyGroup] = true
		}
	}

	// Run post-apply hooks
	if !dryRun {
		for group := range appliedGroups {
			if hook, ok := setup.PostApplyHooks[group]; ok {
				if err := hook(); err != nil {
					fmt.Printf("  ✗ post-apply hook %s: %v\n", group, err)
				}
			}
		}
	}

	fmt.Printf("\n  %d applied", applied)
	if failed > 0 {
		fmt.Printf(" · %d failed", failed)
	}
	fmt.Println()
	return nil
}

func runBatchSetup(states []setup.SettingState) error {
	if flagDryRun {
		fmt.Println("[dry-run] Would apply default settings:")
	} else {
		fmt.Println("Applying default settings...")
	}
	return applySettings(states, flagForce, flagDryRun, flagVerbose)
}

func runSetupReset() error {
	root := dotfilesRoot()
	script := fmt.Sprintf(
		"export DOTFILES_ROOT=%q && source %q/lib/utils.sh && source %q/cmds/setup.sh && linux_mint_reset",
		root, root, root,
	)
	c := exec.Command("bash", "-c", script)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}

// runMacOSSetup preserves the existing macOS bash-based setup path.
func runMacOSSetup() error {
	root := dotfilesRoot()
	script := fmt.Sprintf(
		"export DOTFILES_ROOT=%q && source %q/lib/utils.sh && source %q/cmds/setup.sh && macos_apply",
		root, root, root,
	)
	c := exec.Command("bash", "-c", script)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}
```

Notes:
- The `--reset` path delegates to the existing `linux_mint_reset()` bash function. Can be migrated to Go later.
- macOS delegates to `macos_apply()` bash function, preserving existing behavior.
- `applySettings` accepts `dryRun` and `verbose` flags explicitly.
- `ensureSudo()` and `resetTerminal()` are defined in `cmd/root.go`.
- `isBatchMode()` is defined in `cmd/root.go`.
- `dotfilesRoot()` is defined in `cmd/info.go`.

- [ ] **Step 2: Remove unused imports and verify compilation**

The file needs `"os/exec"` for `runSetupReset`. Verify no unused imports remain from the old code.

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build -ldflags "-X main.dotfilesRoot=/home/thomas/Repos/github.com/ConnerTechnology/dotfiles" -o ctdev .`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add ctdev/cmd/setup.go
git commit -m "feat: rewrite setup command with unified settings TUI"
```

### Task 10: Remove Wizard Package

**Files:**
- Remove: `ctdev/tui/wizard/wizard.go`
- Remove: `ctdev/tui/wizard/wizard_test.go`

- [ ] **Step 1: Verify wizard is not imported anywhere else**

Run: `grep -r "tui/wizard" /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev/`
Expected: Only references in cmd/setup.go (already rewritten) and the wizard package itself

- [ ] **Step 2: Remove wizard package**

```bash
rm -rf ctdev/tui/wizard/
```

- [ ] **Step 3: Verify everything still compiles**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go build -ldflags "-X main.dotfilesRoot=/home/thomas/Repos/github.com/ConnerTechnology/dotfiles" -o ctdev .`
Expected: No errors

- [ ] **Step 4: Run all tests**

Run: `cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev && go test ./... -v`
Expected: All pass (wizard tests gone, setup tests pass)

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: remove wizard package, replaced by setup TUI"
```

### Task 11: Build and Install

- [ ] **Step 1: Build with ldflags**

Run:
```bash
cd /home/thomas/Repos/github.com/ConnerTechnology/dotfiles/ctdev
go build -ldflags "-X main.dotfilesRoot=/home/thomas/Repos/github.com/ConnerTechnology/dotfiles" -o ctdev .
```
Expected: Binary produced

- [ ] **Step 2: Install to user bin**

```bash
command cp -f ./ctdev /home/thomas/.local/bin/ctdev
```

- [ ] **Step 3: Smoke test interactive mode**

Run: `ctdev setup`
Expected: Single-screen TUI with all settings, grouped by category, current values shown. Navigation with ↑/↓, toggle with Space, `i` opens modal, Enter shows confirmation.

- [ ] **Step 4: Smoke test --show mode**

Run: `ctdev setup --show`
Expected: Same layout but no toggles, title says "System Configuration", `i` shows read-only modal with current + default values.

- [ ] **Step 5: Smoke test --dry-run**

Run: `ctdev setup --dry-run`
Expected: TUI works normally, confirmation screen shows "[dry-run]" banner, no changes applied.
