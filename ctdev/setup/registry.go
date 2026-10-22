package setup

import (
	"context"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func init() {
	PostApplyHooks["grub"] = applyUpdateGrub
	PostApplyHooks["pihole-ftl"] = applyPiholeRestart
	PostApplyHooks[macKeyRepeatGroup] = macKeyRepeatNote
	for group, app := range macRestartGroups {
		PostApplyHooks[group] = macRestartHook(app)
	}
}

// Registry is the single source of truth for all setup settings.
var Registry = []Setting{
	// ── GPU & Boot ──────────────────────────────────────────────────────

	{
		Name:        "NVIDIA driver works with Secure Boot",
		Slug:        "gpu",
		Category:    "GPU",
		Description: "Secure Boot only loads signed drivers, so this signs the NVIDIA driver with this machine's own key.",
		Control:     ControlToggle,
		OneWay:      true,
		NoOffSwitch: "Undoing it would only stop the driver loading under Secure Boot again.",
		Default:     "signed",
		DetectFunc:  detectModuleSigned,
		NotNeededFn: NvidiaSigningNotNeeded,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyNvidiaSigning(ctx, o)
		},
		HardwareFn: detectNvidiaLoaded,
	},
	{
		Name:        "NVIDIA GPU wakes from sleep cleanly",
		Slug:        "gpu",
		Category:    "GPU",
		Description: "Lets the NVIDIA driver suspend and resume with the machine, so you don't wake to a black screen or a frozen desktop.",
		Control:     ControlToggle,
		OneWay:      true,
		NoOffSwitch: "Undoing it would only bring the black screens and freezes after sleep back.",
		Default:     "enabled",
		DetectFunc:  detectNvidiaSuspendServices,
		NotNeededFn: suspendNotNeeded,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyNvidiaSuspendServices(ctx, o)
		},
		HardwareFn: detectNvidiaLoaded,
	},
	{
		Name:        "Boot menu at startup",
		Slug:        "boot",
		Category:    "Boot",
		Description: "Seeing the menu at startup lets you pick another system or a recovery entry when this one won't boot.",
		Control:     ControlPicker,
		Default:     "menu",
		Choices: []PickerChoice{
			{Value: "menu", Description: "Always show the boot menu"},
			{Value: "hidden", Description: "Hide menu unless Shift is held"},
			{Value: "countdown", Description: "Show a countdown timer"},
		},
		DetectFunc: detectGrubStyle,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyGrubVar(ctx, o, "GRUB_TIMEOUT_STYLE", v)
		},
		ApplyGroup: "grub",
		HardwareFn: gateGrub,
	},
	{
		Name:        "Boot menu wait time",
		Slug:        "boot",
		Category:    "Boot",
		Description: "Gives you time to pick an entry before the default one starts on its own; 0 starts it right away.",
		Control:     ControlSlider,
		Default:     "10",
		Slider:      &SliderRange{Min: 0, Max: 30, Step: 1, Unit: "s"},
		DetectFunc:  detectGrubTimeout,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyGrubVar(ctx, o, "GRUB_TIMEOUT", v)
		},
		ApplyGroup: "grub",
		HardwareFn: gateGrub,
	},
	{
		Name:        "Other operating systems in the boot menu",
		Slug:        "boot",
		Category:    "Boot",
		Description: "Lets you start Windows or another installed system from the boot menu; turn it off on a host whose VMs use LVM volumes or raw disks, where scanning them can damage those systems.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  detectGrubOSProber,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			grubVal := "true"
			if v == "enabled" {
				grubVal = "false"
			}
			return applyGrubVar(ctx, o, "GRUB_DISABLE_OS_PROBER", grubVal)
		},
		ApplyGroup: "grub",
		HardwareFn: gateGrub,
	},

	// ── Power ──────────────────────────────────────────────────────────

	{
		Name:        "Power profile",
		Slug:        "power",
		Category:    "Power",
		Description: "Balanced is the middle ground; performance trades power for speed, power-saver the reverse.",
		Control:     ControlPicker,
		Default:     "balanced",
		Choices: []PickerChoice{
			{Value: "performance", Description: "Maximum performance"},
			{Value: "balanced", Description: "Balance between performance and power"},
			{Value: "power-saver", Description: "Reduce power consumption"},
		},
		DetectFunc: detectPowerProfile,
		ApplyFunc:  applyPowerProfile,
		HardwareFn: gatePowerProfiles,
	},
	{
		Name:        "Turn off the display after",
		Slug:        "power",
		Category:    "Power",
		Description: "Saves power and the panel when nobody is looking, on AC power.",
		Control:     ControlSlider,
		Default:     "60",
		Slider:      minutesTimer(),
		DetectFunc: func(ctx context.Context) string {
			return detectDconfMinutes(ctx, "/org/cinnamon/settings-daemon/plugins/power/sleep-display-ac")
		},
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconfMinutes(ctx, o, "/org/cinnamon/settings-daemon/plugins/power/sleep-display-ac", v, "")
		},
		HardwareFn: gateCinnamon,
	},
	{
		Name:        "Suspend when idle after",
		Slug:        "power",
		Category:    "Power",
		Description: "Saves power on AC when nobody uses the machine; never keeps it awake for remote use.",
		Control:     ControlSlider,
		Default:     SliderNever,
		Slider:      minutesTimer(),
		DetectFunc: func(ctx context.Context) string {
			return detectDconfMinutes(ctx, "/org/cinnamon/settings-daemon/plugins/power/sleep-inactive-ac-timeout")
		},
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconfMinutes(ctx, o, "/org/cinnamon/settings-daemon/plugins/power/sleep-inactive-ac-timeout", v, "")
		},
		HardwareFn:  gateCinnamon,
		NotNeededFn: suspendNotNeeded,
	},
	{
		Name:        neverSuspendName,
		Slug:        "power",
		Category:    "Power",
		Description: "Keeps an always-on machine reachable: masks systemd's sleep targets so nothing, not even the menu, can suspend it.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  detectSuspendMasked,
		ApplyFunc:   applyNeverSuspend,
		HardwareFn:  gateLinux,
	},
	{
		Name:        "Lock the screen on suspend",
		Slug:        "power",
		Category:    "Power",
		Description: "Asks for your password when the machine wakes from suspend.",
		Control:     ControlToggle,
		Default:     "true",
		DetectFunc: func(ctx context.Context) string {
			return detectDconfBool(ctx, "/org/cinnamon/settings-daemon/plugins/power/lock-on-suspend")
		},
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconf(ctx, o, "/org/cinnamon/settings-daemon/plugins/power/lock-on-suspend", v)
		},
		HardwareFn:  gateCinnamon,
		NotNeededFn: suspendNotNeeded,
	},
	{
		Name:        "Lock the screen with the screensaver",
		Slug:        "power",
		Category:    "Power",
		Description: "Asks for your password when the screensaver ends.",
		Control:     ControlToggle,
		Default:     "false",
		DetectFunc: func(ctx context.Context) string {
			return detectDconfBool(ctx, "/org/cinnamon/desktop/screensaver/lock-enabled")
		},
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconf(ctx, o, "/org/cinnamon/desktop/screensaver/lock-enabled", v)
		},
		HardwareFn: gateCinnamon,
	},
	{
		Name:        "Start the screensaver after",
		Slug:        "power",
		Category:    "Power",
		Description: "Blanks the screen when nobody uses it; never leaves it on.",
		Control:     ControlSlider,
		Default:     SliderNever,
		Slider:      minutesTimer(),
		DetectFunc: func(ctx context.Context) string {
			return detectDconfMinutes(ctx, "/org/cinnamon/desktop/session/idle-delay")
		},
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			// idle-delay is a uint32; an untyped write would store an int32
			// the schema ignores.
			return applyDconfMinutes(ctx, o, "/org/cinnamon/desktop/session/idle-delay", v, "uint32")
		},
		HardwareFn: gateCinnamon,
	},

	// ── Keyboard ───────────────────────────────────────────────────────

	{
		Name:        "Key repeat delay",
		Slug:        "keyboard",
		Category:    "Keyboard",
		Description: "How long you hold a key before it repeats: short enough to hold a key on purpose, long enough not to repeat by accident.",
		Control:     ControlSlider,
		Default:     "200",
		Slider:      &SliderRange{Min: 100, Max: 1000, Step: 25, Unit: "ms"},
		DetectFunc:  detectKeyRepeatDelay,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			// applyKeyRepeat needs both delay and rate; detect current rate to preserve it.
			rate := detectKeyRepeatRate(ctx)
			if rate == "" {
				rate = "50"
			}
			return applyKeyRepeat(ctx, o, v, rate)
		},
		// applyKeyRepeat writes org.cinnamon gsettings, so Cinnamon only.
		HardwareFn: gateCinnamon,
	},
	{
		Name:        "Key repeat rate",
		Slug:        "keyboard",
		Category:    "Keyboard",
		Description: "How many characters per second a held key types, so moving the cursor or deleting with a held key is quick.",
		Control:     ControlSlider,
		Default:     "50",
		Slider:      &SliderRange{Min: 10, Max: 100, Step: 5, Unit: "cps"},
		DetectFunc:  detectKeyRepeatRate,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			// applyKeyRepeat needs both delay and rate; detect current delay to preserve it.
			delay := detectKeyRepeatDelay(ctx)
			if delay == "" {
				delay = "200"
			}
			return applyKeyRepeat(ctx, o, delay, v)
		},
		// applyKeyRepeat writes org.cinnamon gsettings, so Cinnamon only.
		HardwareFn: gateCinnamon,
	},
	{
		Name:     "Key repeat delay",
		Slug:     "keyboard",
		Category: "Keyboard",
		Description: "How long you hold a key before it repeats: short enough to hold a key on purpose, long enough not to repeat by accident. " +
			"macOS moves in 15 ms steps; takes effect after logging out and back in.",
		Control:    ControlSlider,
		Default:    macKeyRepeatDelayDefaultMs,
		Slider:     &SliderRange{Min: 120, Max: 1800, Step: keyRepeatUnitMs, Unit: "ms"},
		DetectFunc: func(ctx context.Context) string { return detectMacKeyRepeat(ctx, "InitialKeyRepeat") },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyMacKeyRepeat(ctx, o, "InitialKeyRepeat", v)
		},
		ApplyGroup: macKeyRepeatGroup,
		HardwareFn: gateMacOS,
	},
	{
		Name:     "Key repeat rate",
		Slug:     "keyboard",
		Category: "Keyboard",
		// "Faster than System Settings allows" rests on community reports that
		// the slider's fastest setting is KeyRepeat 2; Apple doesn't document it.
		Description: "How many characters per second a held key types, so moving the cursor or deleting with a held key is quick. " +
			"ctdev offers the eight fastest 15 ms steps (15–120 ms); 67 cps is faster than System Settings allows. " +
			"Takes effect after logging out and back in.",
		Control:    ControlPicker,
		Default:    macKeyRepeatRateDefaultCps,
		Choices:    macKeyRepeatRateChoices(),
		DetectFunc: detectMacKeyRepeatRate,
		ApplyFunc:  applyMacKeyRepeatRate,
		ApplyGroup: macKeyRepeatGroup,
		HardwareFn: gateMacOS,
	},
	{
		Name:        "Type exactly what I type",
		Slug:        "keyboard",
		Category:    "Keyboard",
		Description: "Stops macOS swapping in curly quotes and long dashes, fixing spelling, capitalizing and adding periods as you type, which breaks code and commands. Off puts back the macOS defaults.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  macTypeAsTyped.detect,
		ApplyFunc:   macTypeAsTyped.apply,
		HardwareFn:  gateMacOS,
	},
	{
		Name:        "Holding a key repeats it",
		Slug:        "keyboard",
		Category:    "Keyboard",
		Description: "Holding a key repeats it instead of opening the accent popup, so a held arrow or Delete key keeps going. Off brings back the accent popup.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  macHoldToRepeat.detect,
		ApplyFunc:   macHoldToRepeat.apply,
		HardwareFn:  gateMacOS,
	},
	{
		Name:        "NumLock on boot",
		Slug:        "keyboard",
		Category:    "Keyboard",
		Description: "Turns NumLock on at login, so the number pad types numbers straight away. Installs numlockx; off removes it.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  detectNumlockx,
		ApplyFunc:   applyNumlockx,
		HardwareFn:  gateDesktop,
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
		DetectFunc: func(ctx context.Context) string {
			return detectDconfString(ctx, "/org/gnome/desktop/peripherals/mouse/accel-profile")
		},
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconfString(ctx, o, "/org/gnome/desktop/peripherals/mouse/accel-profile", v)
		},
		HardwareFn: gateDconfDesktop,
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconf(ctx, o, "/org/gnome/desktop/peripherals/mouse/speed", v)
		},
		HardwareFn: gateDconfDesktop,
	},
	{
		Name:        "Natural scroll",
		Slug:        "mouse",
		Category:    "Mouse",
		Description: "Reverses scroll direction so content moves with your fingers, like a touchscreen.",
		Control:     ControlToggle,
		Default:     "true",
		DetectFunc:  detectNaturalScroll,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconf(ctx, o, "/org/gnome/desktop/peripherals/mouse/natural-scroll", v)
		},
		HardwareFn: gateDconfDesktop,
	},
	{
		Name:        "Mouse bindings (xbindkeys)",
		Slug:        "mouse",
		Category:    "Mouse",
		Description: "Installs xbindkeys and xdotool for custom mouse button mappings.",
		Control:     ControlToggle,
		OneWay:      true,
		Default:     "installed",
		DetectFunc: func(ctx context.Context) string {
			if detectPackageInstalled(ctx, "xbindkeys") && detectPackageInstalled(ctx, "xdotool") {
				return "installed"
			}
			return "not installed"
		},
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyXbindkeys(ctx, o)
		},
		HardwareFn: gateDesktop,
	},

	// ── Audio ──────────────────────────────────────────────────────────

	{
		Name:        "Sound system (PipeWire)",
		Slug:        "audio",
		Category:    "Audio",
		Description: "Installs PipeWire, WirePlumber and the PulseAudio tools, so apps play sound and you can control it from the desktop and the command line.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  func(ctx context.Context) string { return detectPackages(ctx, soundSystemPackages) },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyOwnedPackages(ctx, o, "sound-system", soundSystemPackages, v)
		},
		HardwareFn: allOf(gateDesktop, gateSoundCard),
	},
	{
		Name:        "Bluetooth headphones",
		Slug:        "audio",
		Category:    "Audio",
		Description: "Installs PipeWire's Bluetooth codecs, so Bluetooth headphones and speakers play sound instead of only pairing.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  func(ctx context.Context) string { return detectPackages(ctx, bluetoothAudioPackages) },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyOwnedPackages(ctx, o, "bluetooth-audio", bluetoothAudioPackages, v)
		},
		HardwareFn: allOf(gateDesktop, gateBluetoothAdapter),
	},
	{
		Name:        "Hi-fi Bluetooth audio",
		Slug:        "audio",
		Category:    "Audio",
		Description: "Prefers the LDAC codec and the high-fidelity profile, so Bluetooth headphones sound their best instead of phone-call quality.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  func(_ context.Context) string { return detectFileExists(ldacConfPath) },
		ApplyFunc:   applyLDAC,
		HardwareFn:  allOf(gateDesktop, gateBluetoothAdapter),
	},
	{
		Name:        "Alert sounds",
		Slug:        "audio",
		Category:    "Audio",
		Description: "Plays a sound for desktop alerts and notifications. Off keeps the desktop quiet.",
		Control:     ControlToggle,
		Default:     "false",
		DetectFunc: func(ctx context.Context) string {
			return detectDconfBool(ctx, "/org/cinnamon/desktop/sound/event-sounds")
		},
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconf(ctx, o, "/org/cinnamon/desktop/sound/event-sounds", v)
		},
		HardwareFn: gateCinnamon,
	},

	// ── Bluetooth ──────────────────────────────────────────────────────

	{
		Name:        "Bluetooth support",
		Slug:        "bluetooth",
		Category:    "Bluetooth",
		Description: "Installs bluez and Blueman, so you can pair and manage Bluetooth devices from the desktop.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  func(ctx context.Context) string { return detectPackages(ctx, bluetoothPackages) },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyOwnedPackages(ctx, o, "bluetooth", bluetoothPackages, v)
		},
		HardwareFn: allOf(gateDesktop, gateBluetoothAdapter),
	},
	{
		Name:        "Bluetooth on at startup",
		Slug:        "bluetooth",
		Category:    "Bluetooth",
		Description: "Runs the Bluetooth service now and at every boot, so paired devices connect without a trip to the settings.",
		Control:     ControlToggle,
		Default:     "active",
		DetectFunc:  detectBluetoothService,
		ApplyFunc:   applyBluetoothService,
		HardwareFn:  allOf(gateDesktop, gateBluetoothAdapter),
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
		DetectFunc: func(ctx context.Context) string {
			return detectDconfString(ctx, "/org/nemo/preferences/default-folder-viewer")
		},
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconfString(ctx, o, "/org/nemo/preferences/default-folder-viewer", v)
		},
		// Nemo is Cinnamon's file manager.
		HardwareFn: gateCinnamon,
	},

	{
		Name:        "Hide drives",
		Slug:        "desktop",
		Category:    "Desktop",
		Description: "Hides the partitions you check (a Windows install, a recovery partition) from the file manager, so they can't be opened or ejected by mistake.",
		Control:     ControlMultiPick,
		// No Default: which partitions to hide is a per-machine choice.
		ChoicesFunc: hideDrivesChoices,
		DetectFunc:  detectHideDrives,
		ApplyFunc:   applyHideDrives,
		HardwareFn:  gateCinnamon,
	},

	{
		Name:        "Auto-hiding Dock",
		Slug:        "desktop",
		Category:    "Desktop",
		Description: "Hides the Dock until you point at it, with no delay and no recent-apps section, so windows get the whole screen. Restarts the Dock.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  func(ctx context.Context) string { return detectMacDefaults(ctx, macDesktopDock) },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyMacDefaults(ctx, o, macDesktopDock, v)
		},
		ApplyGroup: macDockGroup,
		HardwareFn: gateMacOS,
	},

	{
		Name:        "Faster animations",
		Slug:        "desktop",
		Category:    "Desktop",
		Description: "Drops the app-launch bounce and window open/close animations and speeds up minimize and Mission Control, so the Mac feels quicker. Restarts the Dock.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  func(ctx context.Context) string { return detectMacDefaults(ctx, macDesktopAnimations) },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyMacDefaults(ctx, o, macDesktopAnimations, v)
		},
		ApplyGroup: macDockGroup,
		HardwareFn: gateMacOS,
	},

	{
		Name:        "Finder list view with path and status bars",
		Slug:        "desktop",
		Category:    "Desktop",
		Description: "Opens folders as lists, shows where you are and how much is there, searches the current folder, and adds Quit to the Finder menu. Restarts Finder.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  func(ctx context.Context) string { return detectMacDefaults(ctx, macDesktopFinder) },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyMacDefaults(ctx, o, macDesktopFinder, v)
		},
		ApplyGroup: macFinderGroup,
		HardwareFn: gateMacOS,
	},

	{
		Name:        "No .DS_Store files on network and USB drives",
		Slug:        "desktop",
		Category:    "Desktop",
		Description: "Stops Finder leaving its hidden folder-settings files on shared and removable drives, where other computers see them as clutter. Restarts Finder.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  func(ctx context.Context) string { return detectMacDefaults(ctx, macDesktopDSStore) },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyMacDefaults(ctx, o, macDesktopDSStore, v)
		},
		ApplyGroup: macFinderGroup,
		HardwareFn: gateMacOS,
	},

	// ── System ─────────────────────────────────────────────────────────

	{
		Name:        "Logitech KVM mouse fix",
		Slug:        "system",
		Category:    "System",
		Description: "Installs a udev rule and systemd user service that restarts Solaar when the Logi Bolt receiver reconnects after a KVM switch. Fixes middle-click not working.",
		Control:     ControlToggle,
		OneWay:      true,
		Default:     "installed",
		DetectFunc: func(_ context.Context) string {
			return detectFileExists("/etc/udev/rules.d/99-logitech-kvm-fix.rules")
		},
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyLogitechKVMFix(ctx, o)
		},
		HardwareFn: detectLogitechBolt,
	},

	// ── Network ────────────────────────────────────────────────────────

	{
		Name:        "Keep WiFi connected when idle",
		Slug:        "network",
		Category:    "Network",
		Description: "Stops WiFi power saving, so the machine stays reachable over WiFi while idle instead of dropping off the network. Takes effect after the next reboot or NetworkManager restart.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc: func(_ context.Context) string {
			return detectFileExists(wifiPowersaveConfPath)
		},
		ApplyFunc:  applyKeepWiFiConnected,
		HardwareFn: gateWiFi,
	},

	// ── Storage ────────────────────────────────────────────────────────

	{
		Name:        "Keep SSDs fast (weekly TRIM)",
		Slug:        "storage",
		Category:    "Storage",
		Description: "Tells your SSDs once a week which blocks are free, so writes stay fast as the disk fills up.",
		Control:     ControlToggle,
		Default:     "active",
		DetectFunc:  func(ctx context.Context) string { return detectSystemdService(ctx, "fstrim.timer") },
		ApplyFunc:   applySSDTrim,
		HardwareFn:  gateSSD,
	},

	// ── SSH & remote access ──────────────────────────────────────────────
	// Everything that lets you reach this machine from somewhere else. Each
	// setting has an off path that removes what ctdev added (setup/remote.go).

	{
		Name:        "SSH server",
		Slug:        "ssh",
		Category:    "SSH & Remote Access",
		Description: "Lets you connect to this machine over SSH. \"On demand\" starts the server only when a connection arrives (ssh.socket), so it costs nothing while idle.",
		Control:     ControlPicker,
		Default:     sshOnDemand,
		Choices: []PickerChoice{
			{Value: sshOnDemand, Description: "Started when someone connects, via ssh.socket (recommended)"},
			{Value: sshAlwaysOn, Description: "Always running, via ssh.service"},
			{Value: sshOff, Description: "No SSH server; new connections are refused"},
		},
		DetectFunc: detectSSHServer,
		ApplyFunc:  applySSHServer,
		HardwareFn: gateLinux,
	},
	{
		Name:        "Key-only login",
		Slug:        "ssh",
		Category:    "SSH & Remote Access",
		Description: "Turns password login off once ~/.ssh/authorized_keys has your key, so a guessed password can't get in. Until a key is there, passwords keep working so you can't lock yourself out.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  detectSSHKeyOnly,
		ApplyFunc:   applySSHKeyOnly,
		// Gated on Linux, not on sshd: the SSH server setting before it
		// installs openssh-server in the same run.
		HardwareFn: gateLinux,
	},
	{
		Name:        "Keep my programs running after I disconnect",
		Slug:        "ssh",
		Category:    "SSH & Remote Access",
		Description: "Lets tmux sessions and background services (like the VS Code tunnel) keep running after you log out or your connection drops.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  detectLinger,
		ApplyFunc:   applyLinger,
		HardwareFn:  gateLinux,
	},
	{
		Name:        "Mosh support",
		Slug:        "ssh",
		Category:    "SSH & Remote Access",
		Description: "Generates the en_US.UTF-8 locale. Mosh, the SSH that survives sleep and roaming, refuses to start without it.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  detectMoshLocale,
		ApplyFunc:   applyMoshLocale,
		HardwareFn:  gateMosh,
	},
	{
		Name:        "Open this machine from vscode.dev",
		Slug:        "ssh",
		Category:    "SSH & Remote Access",
		Description: "Runs the VS Code tunnel in the background so you can open this machine from vscode.dev in any browser (an iPad, say). Also turns on \"Keep my programs running\", which it needs. Run 'code tunnel user login' once to sign in.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  detectCodeTunnel,
		ApplyFunc:   applyCodeTunnel,
		// The tunnel is a systemd user service installed by VS Code's CLI.
		HardwareFn: gateVSCode,
	},
	{
		Name:        "Only allow SSH and Mosh from my local network (firewall)",
		Slug:        "ssh",
		Category:    "SSH & Remote Access",
		Description: "Turns on the UFW firewall and allows SSH and Mosh only from private LAN ranges. Heads up: it blocks every other incoming service (Pi-hole, a web server) unless you open that service's ports first.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  detectUFW,
		ApplyFunc:   applyUFW,
		HardwareFn:  gateLinux,
	},

	// ── Pi-hole DNS ──────────────────────────────────────────────────────
	// Shown only on nodes where Pi-hole is installed (HardwareFn).

	{
		Name:        "Pi-hole upstream DNS",
		Slug:        "pihole",
		Category:    "Pi-hole DNS",
		Description: "Upstream resolvers Pi-hole forwards cache misses to.",
		Control:     ControlPicker,
		// `configure pihole --batch` applies every default, so batch-applying the
		// category moves a node on Unbound back to Cloudflare.
		Default: "cloudflare",
		Choices: []PickerChoice{
			{Value: "cloudflare", Description: "Cloudflare (1.1.1.1, 1.0.0.1)"},
			{Value: "cloudflare-family", Description: "Cloudflare for Families (1.1.1.3 — blocks malware + adult content network-wide)"},
			{Value: "quad9", Description: "Quad9 (9.9.9.9, 149.112.112.112)"},
			{Value: "google", Description: "Google (8.8.8.8, 8.8.4.4)"},
			{Value: "unbound", Description: "Local recursive (Unbound, 127.0.0.1#5335)"},
		},
		DetectFunc: detectPiholeUpstreams,
		ApplyFunc:  applyPiholeUpstreams,
		ApplyGroup: "pihole-ftl",
		HardwareFn: piholeInstalled,
	},
	{
		Name:        "Pi-hole listening mode",
		Slug:        "pihole",
		Category:    "Pi-hole DNS",
		Description: "Which clients Pi-hole answers. LOCAL responds only to devices on the same subnet; ALL answers on every interface (needed for clients on other subnets/VLANs or over Tailscale).",
		Control:     ControlPicker,
		Default:     "ALL",
		Choices: []PickerChoice{
			{Value: "ALL", Description: "Respond on all interfaces"},
			{Value: "LOCAL", Description: "Respond to the local subnet only"},
		},
		DetectFunc: detectPiholeListenMode,
		ApplyFunc:  applyPiholeListenMode,
		ApplyGroup: "pihole-ftl",
		HardwareFn: piholeInstalled,
	},
	{
		Name:        "Pi-hole blocking",
		Slug:        "pihole",
		Category:    "Pi-hole DNS",
		Description: "Whether Pi-hole is actively blocking ad and tracker domains.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  detectPiholeBlocking,
		ApplyFunc:   applyPiholeBlocking,
		HardwareFn:  piholeInstalled,
	},
	{
		Name:     "Pi-hole host resolver",
		Slug:     "pihole",
		Category: "Pi-hole DNS",
		Description: "Makes this node's own DNS survive a stopped Pi-hole: stops Tailscale and " +
			"NetworkManager rewriting resolv.conf, points it at Pi-hole on loopback with Quad9 " +
			"as fallback, and forwards tailnet (MagicDNS) names through Pi-hole so they keep " +
			"resolving here and on the LAN. Without it the host resolves through its own " +
			"container and cannot pull images or reach backups while Pi-hole is down.",
		Control:    ControlToggle,
		OneWay:     true,
		Default:    "applied",
		DetectFunc: detectHostResolver,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyPiholeHostResolver(ctx, o)
		},
		HardwareFn: gatePiholeHostResolver,
	},

	// ── macOS ────────────────────────────────────────────────────────────

	{
		Name:     "macOS defaults",
		Slug:     "macos",
		Category: "macOS",
		Description: "Applies opinionated macOS defaults: volume-change feedback sound, " +
			"expanded save/print dialogs, and password required immediately after screensaver. " +
			"Typing is in the keyboard category; Dock, Finder and animations are in desktop.",
		Control:    ControlToggle,
		OneWay:     true,
		Default:    "applied",
		DetectFunc: detectMacOSDefaults,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return ApplyMacOSDefaults(ctx, o)
		},
		HardwareFn: gateMacOS,
	},
}
