package setup


func init() {
	PostApplyHooks["grub"] = applyUpdateGrub
}

// Registry is the single source of truth for all setup settings.
var Registry = []Setting{
	// ── GPU & Boot ──────────────────────────────────────────────────────

	{
		Name:        "NVIDIA driver signing",
		Slug:        "gpu",
		Category:    "GPU",
		Description: "Signs the NVIDIA kernel module with a Machine Owner Key so Secure Boot accepts it. Without this, the driver won't load on Secure-Boot-enabled systems.",
		Control:     ControlToggle,
		Default:     "signed",
		DetectFunc:  detectModuleSigned,
		ApplyFunc:   func(_ string) error { return applyNvidiaSigning() },
		HardwareFn:  detectNvidiaLoaded,
	},
	{
		Name:        "NVIDIA suspend services",
		Slug:        "gpu",
		Category:    "GPU",
		Description: "Enables systemd services that properly suspend and resume the NVIDIA GPU. Prevents black screens or freezes after waking from sleep.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  detectNvidiaSuspendServices,
		ApplyFunc:   func(_ string) error { return applyNvidiaSuspendServices() },
		HardwareFn:  detectNvidiaLoaded,
	},
	{
		Name:        "GRUB menu style",
		Slug:        "boot",
		Category:    "Boot",
		Description: "Controls how the GRUB boot menu appears. 'menu' always shows it, 'hidden' skips it unless Shift is held, 'countdown' shows a timer.",
		Control:     ControlPicker,
		Default:     "menu",
		Choices: []PickerChoice{
			{Value: "menu", Description: "Always show the boot menu"},
			{Value: "hidden", Description: "Hide menu unless Shift is held"},
			{Value: "countdown", Description: "Show a countdown timer"},
		},
		DetectFunc: detectGrubStyle,
		ApplyFunc:  func(v string) error { return applyGrubVar("GRUB_TIMEOUT_STYLE", v) },
		ApplyGroup: "grub",
	},
	{
		Name:        "GRUB timeout",
		Slug:        "boot",
		Category:    "Boot",
		Description: "How many seconds the GRUB menu waits before booting the default entry.",
		Control:     ControlSlider,
		Default:     "10",
		Slider:      &SliderRange{Min: 0, Max: 30, Step: 1, Unit: "s"},
		DetectFunc:  detectGrubTimeout,
		ApplyFunc:   func(v string) error { return applyGrubVar("GRUB_TIMEOUT", v) },
		ApplyGroup:  "grub",
	},
	{
		Name:        "OS prober",
		Slug:        "boot",
		Category:    "Boot",
		Description: "Lets GRUB detect other operating systems (e.g. Windows) and add them to the boot menu. Useful for dual-boot setups.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  detectGrubOSProber,
		ApplyFunc: func(v string) error {
			grubVal := "true"
			if v == "enabled" {
				grubVal = "false"
			}
			return applyGrubVar("GRUB_DISABLE_OS_PROBER", grubVal)
		},
		ApplyGroup: "grub",
	},

	// ── Power ──────────────────────────────────────────────────────────

	{
		Name:        "Power profile",
		Slug:        "power",
		Category:    "Power",
		Description: "Sets the system-wide power profile. 'performance' maximizes speed, 'balanced' is the middle ground, 'power-saver' extends battery life.",
		Control:     ControlPicker,
		Default:     "performance",
		Choices: []PickerChoice{
			{Value: "performance", Description: "Maximum performance"},
			{Value: "balanced", Description: "Balance between performance and power"},
			{Value: "power-saver", Description: "Reduce power consumption"},
		},
		DetectFunc: detectPowerProfile,
		ApplyFunc:  applyPowerProfile,
	},
	{
		Name:        "Display sleep",
		Slug:        "power",
		Category:    "Power",
		Description: "How long the system waits on AC power before turning off the display.",
		Control:     ControlSlider,
		Default:     "3600",
		Slider:      &SliderRange{Min: 300, Max: 7200, Step: 300, Unit: "s"},
		DetectFunc:  func() string { return detectDconfInt("/org/cinnamon/settings-daemon/plugins/power/sleep-display-ac") },
		ApplyFunc:   func(v string) error { return applyDconfInt("/org/cinnamon/settings-daemon/plugins/power/sleep-display-ac", v) },
	},
	{
		Name:        "Inactive sleep",
		Slug:        "power",
		Category:    "Power",
		Description: "How long the system waits on AC power with no user activity before suspending.",
		Control:     ControlSlider,
		Default:     "2700",
		Slider:      &SliderRange{Min: 300, Max: 7200, Step: 300, Unit: "s"},
		DetectFunc:  func() string { return detectDconfInt("/org/cinnamon/settings-daemon/plugins/power/sleep-inactive-ac-timeout") },
		ApplyFunc: func(v string) error {
			return applyDconfInt("/org/cinnamon/settings-daemon/plugins/power/sleep-inactive-ac-timeout", v)
		},
	},
	{
		Name:        "Lock on suspend",
		Slug:        "power",
		Category:    "Power",
		Description: "Whether the screen locks automatically when the system suspends.",
		Control:     ControlToggle,
		Default:     "true",
		DetectFunc:  func() string { return detectDconfBool("/org/cinnamon/settings-daemon/plugins/power/lock-on-suspend") },
		ApplyFunc:   func(v string) error { return applyDconfBool("/org/cinnamon/settings-daemon/plugins/power/lock-on-suspend", v) },
	},
	{
		Name:        "Screensaver lock",
		Slug:        "power",
		Category:    "Power",
		Description: "Whether the screensaver locks the screen when it activates.",
		Control:     ControlToggle,
		Default:     "false",
		DetectFunc:  func() string { return detectDconfBool("/org/cinnamon/desktop/screensaver/lock-enabled") },
		ApplyFunc:   func(v string) error { return applyDconfBool("/org/cinnamon/desktop/screensaver/lock-enabled", v) },
	},
	{
		Name:        "Idle delay",
		Slug:        "power",
		Category:    "Power",
		Description: "How long the system waits with no input before activating the screensaver.",
		Control:     ControlSlider,
		Default:     "1800",
		Slider:      &SliderRange{Min: 300, Max: 7200, Step: 300, Unit: "s"},
		DetectFunc:  func() string { return detectDconfInt("/org/cinnamon/desktop/session/idle-delay") },
		ApplyFunc:   func(v string) error { return applyDconfInt("/org/cinnamon/desktop/session/idle-delay", v) },
	},

	// ── Keyboard ───────────────────────────────────────────────────────

	{
		Name:        "Key repeat delay",
		Slug:        "keyboard",
		Category:    "Keyboard",
		Description: "How long a key must be held before it starts repeating.",
		Control:     ControlSlider,
		Default:     "200",
		Slider:      &SliderRange{Min: 100, Max: 1000, Step: 25, Unit: "ms"},
		DetectFunc:  detectKeyRepeatDelay,
		ApplyFunc: func(v string) error {
			// applyKeyRepeat needs both delay and rate; detect current rate to preserve it.
			rate := detectKeyRepeatRate()
			if rate == "" {
				rate = "50"
			}
			return applyKeyRepeat(v, rate)
		},
	},
	{
		Name:        "Key repeat rate",
		Slug:        "keyboard",
		Category:    "Keyboard",
		Description: "How many characters per second are generated while a key is held down.",
		Control:     ControlSlider,
		Default:     "50",
		Slider:      &SliderRange{Min: 10, Max: 100, Step: 5, Unit: "cps"},
		DetectFunc:  detectKeyRepeatRate,
		ApplyFunc: func(v string) error {
			// applyKeyRepeat needs both delay and rate; detect current delay to preserve it.
			delay := detectKeyRepeatDelay()
			if delay == "" {
				delay = "200"
			}
			return applyKeyRepeat(delay, v)
		},
	},
	{
		Name:        "NumLock on boot",
		Slug:        "keyboard",
		Category:    "Keyboard",
		Description: "Ensures NumLock is turned on at login. Installs numlockx if needed.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc: func() string {
			if detectPackageInstalled("numlockx") {
				return "installed"
			}
			return "not installed"
		},
		ApplyFunc: func(_ string) error { return applyPackages([]string{"numlockx"}) },
	},
	{
		Name:        "Mouse accel profile",
		Slug:        "mouse",
		Category:    "Mouse",
		Description: "Controls pointer acceleration. 'flat' gives raw 1:1 input, 'adaptive' accelerates based on speed.",
		Control:     ControlPicker,
		Default:     "flat",
		Choices: []PickerChoice{
			{Value: "flat", Description: "No acceleration (1:1 input)"},
			{Value: "adaptive", Description: "Speed-dependent acceleration"},
		},
		DetectFunc: func() string {
			return detectDconfString("/org/gnome/desktop/peripherals/mouse/accel-profile")
		},
		ApplyFunc: func(v string) error {
			return applyDconfString("/org/gnome/desktop/peripherals/mouse/accel-profile", v)
		},
	},
	{
		Name:        "Mouse speed",
		Slug:        "mouse",
		Category:    "Mouse",
		Description: "Overall pointer speed multiplier.",
		Control:     ControlSlider,
		Default:     "0.65",
		Slider:      &SliderRange{Min: 0.0, Max: 1.0, Step: 0.05, Unit: ""},
		DetectFunc:  detectMouseSpeed,
		ApplyFunc: func(v string) error {
			return applyDconfDouble("/org/gnome/desktop/peripherals/mouse/speed", v)
		},
	},
	{
		Name:        "Natural scroll",
		Slug:        "mouse",
		Category:    "Mouse",
		Description: "Reverses scroll direction so content moves with your fingers, like a touchscreen.",
		Control:     ControlToggle,
		Default:     "true",
		DetectFunc:  detectNaturalScroll,
		ApplyFunc: func(v string) error {
			return applyDconfBool("/org/gnome/desktop/peripherals/mouse/natural-scroll", v)
		},
	},
	{
		Name:        "Mouse bindings (xbindkeys)",
		Slug:        "mouse",
		Category:    "Mouse",
		Description: "Installs xbindkeys and xdotool for custom mouse button mappings.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc: func() string {
			if detectPackageInstalled("xbindkeys") && detectPackageInstalled("xdotool") {
				return "installed"
			}
			return "not installed"
		},
		ApplyFunc: func(_ string) error { return applyXbindkeys() },
	},

	// ── Audio ──────────────────────────────────────────────────────────

	{
		Name:        "Bluetooth/audio packages",
		Slug:        "audio",
		Category:    "Audio",
		Description: "Installs the core Bluetooth and audio stack: PipeWire, WirePlumber, Bluetooth codecs, and PulseAudio compatibility.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc: func() string {
			pkgs := []string{"pipewire-audio", "bluez", "blueman", "libspa-0.2-bluetooth", "pulseaudio-utils"}
			for _, p := range pkgs {
				if !detectPackageInstalled(p) {
					return "not installed"
				}
			}
			return "installed"
		},
		ApplyFunc: func(_ string) error {
			return applyPackages([]string{
				"pipewire-audio", "bluez", "blueman",
				"libspa-0.2-bluetooth", "pulseaudio-utils",
			})
		},
	},
	{
		Name:        "Bluetooth service",
		Slug:        "bluetooth",
		Category:    "Bluetooth",
		Description: "Enables and starts the system Bluetooth daemon so devices can pair and connect.",
		Control:     ControlToggle,
		Default:     "active",
		DetectFunc:  func() string { return detectSystemdService("bluetooth.service") },
		ApplyFunc:   func(_ string) error { return applySystemdEnable("bluetooth.service") },
	},
	{
		Name:        "WirePlumber LDAC config",
		Slug:        "audio",
		Category:    "Audio",
		Description: "Copies a WirePlumber config that prioritizes LDAC codec for high-quality Bluetooth audio, then restarts the PipeWire stack.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  func() string { return detectFileExists("/etc/wireplumber/wireplumber.conf.d/51-ldac-hq.conf") },
		ApplyFunc:   func(_ string) error { return applyWireplumberLDAC() },
	},
	{
		Name:        "Event sounds",
		Slug:        "audio",
		Category:    "Audio",
		Description: "Controls whether desktop event sounds (alerts, notifications) play.",
		Control:     ControlToggle,
		Default:     "false",
		DetectFunc:  func() string { return detectDconfBool("/org/cinnamon/desktop/sound/event-sounds") },
		ApplyFunc:   func(v string) error { return applyDconfBool("/org/cinnamon/desktop/sound/event-sounds", v) },
	},

	// ── Desktop ────────────────────────────────────────────────────────

	{
		Name:        "File manager view",
		Slug:        "desktop",
		Category:    "Desktop",
		Description: "Sets the default view mode in the Nemo file manager.",
		Control:     ControlPicker,
		Default:     "list-view",
		Choices: []PickerChoice{
			{Value: "list-view", Description: "Show files in a detailed list"},
			{Value: "icon-view", Description: "Show files as icons"},
		},
		DetectFunc: func() string {
			return detectDconfString("/org/nemo/preferences/default-folder-viewer")
		},
		ApplyFunc: func(v string) error {
			return applyDconfString("/org/nemo/preferences/default-folder-viewer", v)
		},
	},

	{
		Name:        "Hide drives",
		Slug:        "desktop",
		Category:    "Desktop",
		Description: "Hides Windows/secondary NVMe partitions from the file manager so they don't clutter the sidebar.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  func() string { return detectFileExists("/etc/udev/rules.d/99-hide-drives.rules") },
		ApplyFunc:   func(_ string) error { return applyHideDrives() },
	},

	// ── System ─────────────────────────────────────────────────────────

	{
		Name:        "Logitech KVM mouse fix",
		Slug:        "system",
		Category:    "System",
		Description: "Installs a udev rule and systemd user service that restarts Solaar when the Logi Bolt receiver reconnects after a KVM switch. Fixes middle-click not working.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  func() string { return detectFileExists("/etc/udev/rules.d/99-logitech-kvm-fix.rules") },
		ApplyFunc:   func(_ string) error { return applyLogitechKVMFix() },
		HardwareFn:  detectLogitechBolt,
	},

	// ── Network ────────────────────────────────────────────────────────

	{
		Name:        "WiFi suspend fix (MT7925E)",
		Slug:        "network",
		Category:    "Network",
		Description: "Installs a systemd sleep hook that performs a PCIe-level reset of the MT7925E WiFi adapter around suspend. Fixes WiFi not reconnecting after wake.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  func() string { return detectFileExists("/usr/lib/systemd/system-sleep/wifi-mt7925") },
		ApplyFunc:   func(_ string) error { return applyWifiSuspendFix() },
		HardwareFn:  detectMT7925E,
	},
	{
		Name:        "SSD TRIM timer",
		Slug:        "system",
		Category:    "System",
		Description: "Enables periodic TRIM for SSDs, which helps maintain write performance and longevity.",
		Control:     ControlToggle,
		Default:     "active",
		DetectFunc:  func() string { return detectSystemdService("fstrim.timer") },
		ApplyFunc:   func(_ string) error { return applySSDTrim() },
	},
}
