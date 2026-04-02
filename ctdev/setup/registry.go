package setup


func init() {
	PostApplyHooks["grub"] = applyUpdateGrub
}

// Registry is the single source of truth for all setup settings.
var Registry = []Setting{
	// ── GPU & Boot ──────────────────────────────────────────────────────

	{
		Name:        "NVIDIA driver signing",
		Category:    "GPU & Boot",
		Description: "Signs the NVIDIA kernel module with a Machine Owner Key so Secure Boot accepts it. Without this, the driver won't load on Secure-Boot-enabled systems.",
		TechDetail:  "Runs cmds/gpu.sh which enrolls a MOK and signs the nvidia module.",
		Control:     ControlToggle,
		Default:     "signed",
		DetectFunc:  detectModuleSigned,
		ApplyFunc:   func(_ string) error { return applyNvidiaSigning() },
		HardwareFn:  detectNvidiaLoaded,
	},
	{
		Name:        "NVIDIA suspend services",
		Category:    "GPU & Boot",
		Description: "Enables systemd services that properly suspend and resume the NVIDIA GPU. Prevents black screens or freezes after waking from sleep.",
		TechDetail:  "systemctl enable nvidia-suspend/resume/hibernate/persistenced",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  detectNvidiaSuspendServices,
		ApplyFunc:   func(_ string) error { return applyNvidiaSuspendServices() },
		HardwareFn:  detectNvidiaLoaded,
	},
	{
		Name:        "GRUB menu style",
		Category:    "GPU & Boot",
		Description: "Controls how the GRUB boot menu appears. 'menu' always shows it, 'hidden' skips it unless Shift is held, 'countdown' shows a timer.",
		TechDetail:  "GRUB_TIMEOUT_STYLE in /etc/default/grub",
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
		Category:    "GPU & Boot",
		Description: "How many seconds the GRUB menu waits before booting the default entry.",
		TechDetail:  "GRUB_TIMEOUT in /etc/default/grub",
		Control:     ControlSlider,
		Default:     "10",
		Slider:      &SliderRange{Min: 0, Max: 30, Step: 1, Unit: "s"},
		DetectFunc:  detectGrubTimeout,
		ApplyFunc:   func(v string) error { return applyGrubVar("GRUB_TIMEOUT", v) },
		ApplyGroup:  "grub",
	},
	{
		Name:        "OS prober",
		Category:    "GPU & Boot",
		Description: "Lets GRUB detect other operating systems (e.g. Windows) and add them to the boot menu. Useful for dual-boot setups.",
		TechDetail:  "GRUB_DISABLE_OS_PROBER in /etc/default/grub (\"false\" = enabled)",
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

	// ── Power & Display ────────────────────────────────────────────────

	{
		Name:        "Power profile",
		Category:    "Power & Display",
		Description: "Sets the system-wide power profile. 'performance' maximizes speed, 'balanced' is the middle ground, 'power-saver' extends battery life.",
		TechDetail:  "powerprofilesctl set <profile>",
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
		Category:    "Power & Display",
		Description: "How long the system waits on AC power before turning off the display.",
		TechDetail:  "dconf /org/cinnamon/settings-daemon/plugins/power/sleep-display-ac",
		Control:     ControlSlider,
		Default:     "3600",
		Slider:      &SliderRange{Min: 300, Max: 7200, Step: 300, Unit: "s"},
		DetectFunc:  func() string { return detectDconfInt("/org/cinnamon/settings-daemon/plugins/power/sleep-display-ac") },
		ApplyFunc:   func(v string) error { return applyDconfInt("/org/cinnamon/settings-daemon/plugins/power/sleep-display-ac", v) },
	},
	{
		Name:        "Inactive sleep",
		Category:    "Power & Display",
		Description: "How long the system waits on AC power with no user activity before suspending.",
		TechDetail:  "dconf /org/cinnamon/settings-daemon/plugins/power/sleep-inactive-ac-timeout",
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
		Category:    "Power & Display",
		Description: "Whether the screen locks automatically when the system suspends.",
		TechDetail:  "dconf /org/cinnamon/settings-daemon/plugins/power/lock-on-suspend",
		Control:     ControlToggle,
		Default:     "true",
		DetectFunc:  func() string { return detectDconfBool("/org/cinnamon/settings-daemon/plugins/power/lock-on-suspend") },
		ApplyFunc:   func(v string) error { return applyDconfBool("/org/cinnamon/settings-daemon/plugins/power/lock-on-suspend", v) },
	},
	{
		Name:        "Screensaver lock",
		Category:    "Power & Display",
		Description: "Whether the screensaver locks the screen when it activates.",
		TechDetail:  "dconf /org/cinnamon/desktop/screensaver/lock-enabled",
		Control:     ControlToggle,
		Default:     "false",
		DetectFunc:  func() string { return detectDconfBool("/org/cinnamon/desktop/screensaver/lock-enabled") },
		ApplyFunc:   func(v string) error { return applyDconfBool("/org/cinnamon/desktop/screensaver/lock-enabled", v) },
	},
	{
		Name:        "Idle delay",
		Category:    "Power & Display",
		Description: "How long the system waits with no input before activating the screensaver.",
		TechDetail:  "dconf /org/cinnamon/desktop/session/idle-delay",
		Control:     ControlSlider,
		Default:     "1800",
		Slider:      &SliderRange{Min: 300, Max: 7200, Step: 300, Unit: "s"},
		DetectFunc:  func() string { return detectDconfInt("/org/cinnamon/desktop/session/idle-delay") },
		ApplyFunc:   func(v string) error { return applyDconfInt("/org/cinnamon/desktop/session/idle-delay", v) },
	},

	// ── Input Devices ──────────────────────────────────────────────────

	{
		Name:        "Key repeat delay",
		Category:    "Input Devices",
		Description: "How long a key must be held before it starts repeating.",
		TechDetail:  "xset r rate <delay> <rate> + gsettings org.cinnamon.desktop.peripherals.keyboard delay",
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
		Category:    "Input Devices",
		Description: "How many characters per second are generated while a key is held down.",
		TechDetail:  "xset r rate <delay> <rate> + gsettings org.cinnamon.desktop.peripherals.keyboard repeat-interval",
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
		Category:    "Input Devices",
		Description: "Ensures NumLock is turned on at login. Installs numlockx if needed.",
		TechDetail:  "dpkg -s numlockx",
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
		Category:    "Input Devices",
		Description: "Controls pointer acceleration. 'flat' gives raw 1:1 input, 'adaptive' accelerates based on speed.",
		TechDetail:  "dconf /org/gnome/desktop/peripherals/mouse/accel-profile",
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
		Category:    "Input Devices",
		Description: "Overall pointer speed multiplier.",
		TechDetail:  "dconf /org/gnome/desktop/peripherals/mouse/speed",
		Control:     ControlSlider,
		Default:     "0.65",
		Slider:      &SliderRange{Min: 0.0, Max: 1.0, Step: 0.05, Unit: ""},
		DetectFunc:  detectMouseSpeed,
		ApplyFunc: func(v string) error {
			return applyDconfString("/org/gnome/desktop/peripherals/mouse/speed", v)
		},
	},
	{
		Name:        "Natural scroll",
		Category:    "Input Devices",
		Description: "Reverses scroll direction so content moves with your fingers, like a touchscreen.",
		TechDetail:  "dconf /org/gnome/desktop/peripherals/mouse/natural-scroll",
		Control:     ControlToggle,
		Default:     "true",
		DetectFunc:  detectNaturalScroll,
		ApplyFunc: func(v string) error {
			return applyDconfBool("/org/gnome/desktop/peripherals/mouse/natural-scroll", v)
		},
	},
	{
		Name:        "Mouse bindings (xbindkeys)",
		Category:    "Input Devices",
		Description: "Installs xbindkeys and xdotool for custom mouse button mappings, and symlinks the config from dotfiles.",
		TechDetail:  "xbindkeys + xdotool packages, ~/.xbindkeysrc symlink",
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

	// ── Audio & Bluetooth ──────────────────────────────────────────────

	{
		Name:        "Bluetooth/audio packages",
		Category:    "Audio & Bluetooth",
		Description: "Installs the core Bluetooth and audio stack: PipeWire, WirePlumber, Bluetooth codecs, and PulseAudio compatibility.",
		TechDetail:  "apt install pipewire-audio bluez blueman libspa-0.2-bluetooth pulseaudio-utils",
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
		Category:    "Audio & Bluetooth",
		Description: "Enables and starts the system Bluetooth daemon so devices can pair and connect.",
		TechDetail:  "systemctl enable/start bluetooth.service",
		Control:     ControlToggle,
		Default:     "active",
		DetectFunc:  func() string { return detectSystemdService("bluetooth.service") },
		ApplyFunc:   func(_ string) error { return applySystemdEnable("bluetooth.service") },
	},
	{
		Name:        "WirePlumber LDAC config",
		Category:    "Audio & Bluetooth",
		Description: "Copies a WirePlumber config that prioritizes LDAC codec for high-quality Bluetooth audio, then restarts the PipeWire stack.",
		TechDetail:  "/etc/wireplumber/wireplumber.conf.d/51-ldac-hq.conf",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  func() string { return detectFileExists("/etc/wireplumber/wireplumber.conf.d/51-ldac-hq.conf") },
		ApplyFunc:   func(_ string) error { return applyWireplumberLDAC() },
	},
	{
		Name:        "Event sounds",
		Category:    "Audio & Bluetooth",
		Description: "Controls whether desktop event sounds (alerts, notifications) play.",
		TechDetail:  "dconf /org/cinnamon/desktop/sound/event-sounds",
		Control:     ControlToggle,
		Default:     "false",
		DetectFunc:  func() string { return detectDconfBool("/org/cinnamon/desktop/sound/event-sounds") },
		ApplyFunc:   func(v string) error { return applyDconfBool("/org/cinnamon/desktop/sound/event-sounds", v) },
	},

	// ── Desktop ────────────────────────────────────────────────────────

	{
		Name:        "File manager view",
		Category:    "Desktop",
		Description: "Sets the default view mode in the Nemo file manager.",
		TechDetail:  "dconf /org/nemo/preferences/default-folder-viewer",
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

	// ── Network & System ───────────────────────────────────────────────

	{
		Name:        "WiFi suspend fix (MT7925E)",
		Category:    "Network & System",
		Description: "Installs a systemd sleep hook that unloads and reloads the MT7925E WiFi driver around suspend. Fixes WiFi not reconnecting after wake.",
		TechDetail:  "/usr/lib/systemd/system-sleep/wifi-mt7925 hook script",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  func() string { return detectFileExists("/usr/lib/systemd/system-sleep/wifi-mt7925") },
		ApplyFunc:   func(_ string) error { return applyWifiSuspendFix() },
		HardwareFn:  detectMT7925E,
	},
	{
		Name:        "SSD TRIM timer",
		Category:    "Network & System",
		Description: "Enables periodic TRIM for SSDs, which helps maintain write performance and longevity.",
		TechDetail:  "systemctl enable/start fstrim.timer",
		Control:     ControlToggle,
		Default:     "active",
		DetectFunc:  func() string { return detectSystemdService("fstrim.timer") },
		ApplyFunc:   func(_ string) error { return applySSDTrim() },
	},
}
