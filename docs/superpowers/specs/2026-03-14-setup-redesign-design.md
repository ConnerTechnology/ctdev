# Setup Redesign: Unified Interactive Settings Screen

## Summary

Replace the multi-step setup wizard with a single-screen interactive settings view. All system settings are visible at once, grouped by category, with current values shown inline. Press `i` on any setting to open a detail modal with description, context, and configurable controls (slider, picker, or toggle). `ctdev setup --show` becomes a read-only version of the same screen.

## Scope

- **Linux only.** macOS setup (`buildMacOSSteps`/`macos_apply`) is out of scope for this redesign. The existing macOS path remains unchanged for now.
- **Component installation is separate.** The current wizard's "Install Components" step is removed. Component installation is handled via `ctdev install`.
- **Deprecated flags:** `--skip-gpu` and `--skip-configure` are removed. Hardware-based filtering replaces `--skip-gpu`. All settings are individually toggleable, replacing `--skip-configure`.

## Goals

- One screen shows all settings with current system values
- Info modal (`i`) provides description, explanation, and controls for each setting
- `--show` mode is read-only: navigate and inspect, but no changes
- Only settings relevant to detected hardware are shown
- Apply only settings that differ from current state (skip already-configured)
- `--force` flag re-applies everything regardless of current state

## Settings Registry

27 settings organized into 7 categories. Each setting has:

```go
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
    Description string       // shown in info modal
    TechDetail  string       // underlying commands/paths, shown in modal footer
    Control     ControlType
    Default     string       // our recommended value as string
    Slider      *SliderRange // non-nil for ControlSlider
    Choices     []PickerChoice // non-nil for ControlPicker
    DetectFunc  func() string   // reads current system value
    ApplyFunc   func(value string) error // writes value to system
    HardwareFn  func() bool  // optional; setting hidden when returns false
    ApplyGroup  string       // settings with same group share a post-apply hook
}
```

### Post-Apply Hooks

Settings with the same `ApplyGroup` share a post-apply hook that runs once after all settings in the group are applied:

| ApplyGroup | Hook |
|------------|------|
| `"grub"` | `update-grub` |

### Settings Inventory

#### GPU & Boot
| Setting | Control | Default | Range/Options |
|---------|---------|---------|---------------|
| NVIDIA driver signing | toggle | on | — |
| NVIDIA suspend services | toggle | on | — |
| GRUB menu style | picker | menu | menu / hidden / countdown |
| GRUB timeout | slider | 10s | 0–30s, step 1 |
| OS prober | toggle | on | — |

Hardware condition: NVIDIA settings shown only if `lsmod | grep nvidia`.
GRUB settings share `ApplyGroup: "grub"`.

#### Power & Display
| Setting | Control | Default | Range/Options |
|---------|---------|---------|---------------|
| Power profile | picker | performance | performance / balanced / power-saver |
| Display sleep | slider | 60 min | 5–120 min, step 5 |
| Inactive sleep | slider | 45 min | 5–120 min, step 5 |
| Lock on suspend | toggle | on | — |
| Screensaver lock | toggle | off | — |
| Idle delay | slider | 30 min | 5–120 min, step 5 |

#### Input Devices
| Setting | Control | Default | Range/Options |
|---------|---------|---------|---------------|
| Key repeat delay | slider | 200ms | 100–1000ms, step 25 |
| Key repeat rate | slider | 50 cps | 10–100 cps, step 5 |
| NumLock on boot | toggle | on | — |
| Mouse accel profile | picker | flat | flat / adaptive |
| Mouse speed | slider | 0.65 | 0.0–1.0, step 0.05 |
| Natural scroll | toggle | on | — |
| Mouse bindings (xbindkeys) | toggle | on | — |

#### Audio & Bluetooth
| Setting | Control | Default | Range/Options |
|---------|---------|---------|---------------|
| Bluetooth/audio packages | toggle | on | — |
| Bluetooth service | toggle | on | — |
| WirePlumber LDAC config | toggle | on | — |
| Event sounds | toggle | off | — |

#### Desktop
| Setting | Control | Default | Range/Options |
|---------|---------|---------|---------------|
| File manager view | picker | list | list / icon |

#### Network & System
| Setting | Control | Default | Range/Options |
|---------|---------|---------|---------------|
| WiFi suspend fix (MT7925E) | toggle | on | — |
| SSD TRIM timer | toggle | on | — |

Hardware condition: WiFi suspend fix shown only if MT7925E detected (`lspci -d 14c3:0717` or `lsmod | grep mt7925e`).

## TUI Design

### Main Screen (Interactive Mode)

```
System Setup
Space toggle · i info · Enter apply · q quit

GPU & Boot
  ◉ NVIDIA driver signing          signed
  ◉ NVIDIA suspend services         enabled
  ◉ GRUB menu style                 menu
  ◉ GRUB timeout                    10s
  ◉ OS prober                       enabled

Power & Display
  ◉ Power profile                   performance
  ◉ Display sleep                   60 min
  ...

Input Devices
  ◉ Key repeat delay                200ms      ←  (cursor)
  ◉ Key repeat rate                 50 cps
  ...
```

**Behavior:**
- ↑/↓ or j/k to navigate between settings
- Space toggles setting on/off — controls whether the setting is included in the apply set
  - Enabled (◉ green): setting will be applied with its desired value
  - Disabled (○ dim): setting will be skipped during apply, regardless of current vs desired
- `i` opens info modal for highlighted setting
- Enter shows confirmation screen listing changes, then applies
- q/Ctrl+C quits without applying
- Value color: green = matches our default, yellow = differs or not configured
- Scrolling: viewport follows cursor. If the list exceeds terminal height, the view scrolls to keep the cursor visible with 3 lines of context above/below.

### Main Screen (Read-Only Mode — `--show`)

```
System Configuration
i info · q quit

GPU & Boot
  NVIDIA driver signing          signed
  NVIDIA suspend services         enabled
  GRUB menu style                 menu
  GRUB timeout                    10s
  OS prober                       enabled

Input Devices
  Key repeat delay                500ms      ←  (cursor)
  Key repeat rate                 25 cps
  ...
```

- No toggle circles (◉/○)
- No Space toggle, no Enter apply
- Navigate with ↑/↓, press `i` to inspect
- Title: "System Configuration" instead of "System Setup"

### Confirmation Screen (before applying)

When Enter is pressed, a confirmation screen shows what will change:

```
Apply Changes?

  GRUB timeout                    5s → 10s
  Key repeat delay                500ms → 200ms
  Key repeat rate                 25 cps → 50 cps
  WiFi suspend fix                not installed → install
  Mouse bindings (xbindkeys)      not installed → install

5 settings will be applied. Enter to confirm · Esc to go back
```

Only settings where current != desired and enabled are listed.

### Info Modal — Slider Type (Interactive)

```
┌─────────────────────────────────────────────┐
│  Key Repeat Delay                           │
│  Current: 500ms                             │
│                                             │
│  How long to hold a key before it starts    │
│  repeating. Lower values make the keyboard  │
│  feel more responsive. System default is    │
│  500ms; power users prefer 150–250ms.       │
│                                             │
│  Value: 200ms                               │
│  100 ◂━━●━━━━━━━━━━━━━━━━━━━━▸ 1000ms      │
│  ← / → adjust · d default (200ms)          │
│                                             │
│  org.cinnamon.desktop.peripherals.          │
│  keyboard.delay                             │
│  xset r rate <delay> <rate>                 │
│                                             │
│  Esc close · ← → adjust · d default        │
└─────────────────────────────────────────────┘
```

Interactive modals show both "Current" (detected system value) and "Value" (what will be applied).

### Info Modal — Picker Type (Interactive)

```
┌─────────────────────────────────────────────┐
│  Power Profile                              │
│  Current: balanced                          │
│                                             │
│  Controls CPU/GPU performance scaling.      │
│  "Performance" keeps clocks high for dev    │
│  workloads. "Balanced" lets the system      │
│  throttle when idle.                        │
│                                             │
│  ◉ performance — max clocks, no throttling  │
│  ○ balanced — system-managed scaling        │
│  ○ power-saver — cap frequency              │
│                                             │
│  powerprofilesctl set <profile>             │
│                                             │
│  Esc close · ↑ ↓ select · d default        │
└─────────────────────────────────────────────┘
```

### Info Modal — Toggle Type (Interactive)

```
┌─────────────────────────────────────────────┐
│  WiFi Suspend Fix (MT7925E)                 │
│  Current: not installed                     │
│                                             │
│  Installs a systemd sleep hook that         │
│  unloads mt7925e before suspend and         │
│  reloads after resume. The MT7925E PCI      │
│  resume path is buggy (ETIMEDOUT / -110).   │
│                                             │
│  /usr/lib/systemd/system-sleep/wifi-mt7925  │
│  modprobe -r mt7925e (pre-suspend)          │
│  modprobe mt7925e (post-resume)             │
│                                             │
│  Esc close · Space toggle                   │
└─────────────────────────────────────────────┘
```

### Info Modal — Read-Only (--show)

Same layout but no controls. Shows current value, default value, description, and technical details.

```
┌─────────────────────────────────────────────┐
│  Key Repeat Delay                           │
│                                             │
│  Current: 500ms     Default: 200ms          │
│                                             │
│  How long to hold a key before it starts    │
│  repeating. Lower values make the keyboard  │
│  feel more responsive. System default is    │
│  500ms; power users prefer 150–250ms.       │
│  Range: 100–1000ms.                         │
│                                             │
│  org.cinnamon.desktop.peripherals.          │
│  keyboard.delay                             │
│  xset r rate <delay> <rate>                 │
│                                             │
│  Esc close                                  │
└─────────────────────────────────────────────┘
```

## Application Logic

### Startup Flow

1. Detect hardware (NVIDIA GPU, MT7925E WiFi)
2. Filter settings registry by hardware conditions
3. For each visible setting, run DetectFunc to get current value
4. Initialize `desiredValue` to our default for each setting
5. Launch TUI with settings, current values, and mode (interactive or readonly)

Note: `desiredValue` starts at our opinionated default, not the current system value. This means on a fresh system, all settings that differ from our defaults will show as pending changes. This is intentional — setup is an "apply our preferred configuration" tool.

### Apply Flow (interactive mode, Enter pressed)

1. Show confirmation screen listing all settings where enabled AND current != desired
2. If user confirms (Enter):
   - Run `ensureSudo()` — if sudo is denied, abort and show error
   - For each setting in the apply set, call ApplyFunc with desired value
   - Run post-apply hooks for each ApplyGroup that had changes (e.g., `update-grub`)
   - Show summary of what was changed (success/failure per setting)
3. If user cancels (Esc): return to main screen

### CLI Flag Behavior

- **`--dry-run`**: Show what would be applied without executing any ApplyFunc. In the TUI, the confirmation screen shows changes with a "[dry-run]" banner and exits after display.
- **`--force`**: Include all enabled settings in the apply set, regardless of whether current == desired.
- **`--verbose`**: Show DetectFunc results during startup and full ApplyFunc command output during apply.
- **`--reset`**: Set all `desiredValue` fields to system defaults (not our defaults) and apply. Effectively undoes our configuration. Uses the existing `linux_mint_reset()` logic.

### Batch Mode

When stdin is not a TTY or `--batch` is passed: skip the TUI entirely, apply all settings using our defaults. Equivalent to pressing Enter immediately with everything enabled. Respects `--dry-run` and `--force`.

### Value Tracking

- Each setting stores: `enabled` (bool), `currentValue` (string), `desiredValue` (string)
- `enabled` controls whether the setting is included in the apply set — it does not affect the value
- Toggle settings: desired is the default value string (e.g., "true", "false", "installed")
- Slider settings: desired is the slider position value (e.g., "200")
- Picker settings: desired is the selected choice (e.g., "performance")
- On first load, `desiredValue` is initialized to the setting's Default field
- When the user changes a value in the modal, `desiredValue` updates

## File Structure

### New Files

```
ctdev/setup/
  setting.go       — Setting type definition (struct, ControlType, SliderRange, PickerChoice)
  registry.go      — All 27 settings defined as data
  detect.go        — Detection functions (read current system values)
  apply.go         — Apply functions (write values to system)

ctdev/tui/setup/
  model.go         — Bubble Tea model: main list view, scrolling, mode handling
  modal.go         — Info modal overlay (slider/picker/toggle/readonly)
  confirm.go       — Confirmation screen before applying
```

### Modified Files

```
ctdev/cmd/setup.go — Simplified: hardware detection, TUI launch, apply diff
```

### Removed Files

```
ctdev/tui/wizard/  — Entire package replaced by tui/setup
```

## Key Decisions

- **Smart diff by default**: Only apply settings that differ from current state. `--force` re-applies everything.
- **Hardware filtering**: Settings for missing hardware are hidden, not dimmed.
- **No inline editing on main screen**: All value changes happen in the info modal.
- **Unified view**: `--show` and `setup` share the same screen layout and rendering code, differing only in mode (readonly vs interactive).
- **Settings as data**: Each setting is a struct with detect/apply functions, not hardcoded logic scattered through the codebase.
- **Opinionated defaults**: `desiredValue` initializes to our preferred default, not the current system value. Setup is a "configure to our standard" tool.
- **Confirmation before apply**: Enter shows a diff of what will change before executing. No accidental system modifications.
- **macOS out of scope**: This redesign covers Linux only. macOS path is preserved as-is.
- **Component installation separate**: Handled by `ctdev install`, not part of setup.
