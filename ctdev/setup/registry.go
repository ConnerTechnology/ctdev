package setup

import (
	"context"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func init() {
	PostApplyHooks["grub"] = applyUpdateGrub
	PostApplyHooks["pihole-ftl"] = applyPiholeRestart
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyNvidiaSigning(ctx, o)
		},
		HardwareFn: detectNvidiaLoaded,
	},
	{
		Name:        "NVIDIA suspend services",
		Slug:        "gpu",
		Category:    "GPU",
		Description: "Enables systemd services that properly suspend and resume the NVIDIA GPU. Prevents black screens or freezes after waking from sleep.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  detectNvidiaSuspendServices,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyNvidiaSuspendServices(ctx, o)
		},
		HardwareFn: detectNvidiaLoaded,
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyGrubVar(ctx, o, "GRUB_TIMEOUT_STYLE", v)
		},
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyGrubVar(ctx, o, "GRUB_TIMEOUT", v)
		},
		ApplyGroup: "grub",
	},
	{
		Name:        "OS prober",
		Slug:        "boot",
		Category:    "Boot",
		Description: "Lets GRUB detect other operating systems (e.g. Windows) and add them to the boot menu. Useful for dual-boot setups.",
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconf(ctx, o, "/org/cinnamon/settings-daemon/plugins/power/sleep-display-ac", v)
		},
	},
	{
		Name:        "Inactive sleep",
		Slug:        "power",
		Category:    "Power",
		Description: "How long the system waits on AC power with no user activity before suspending.",
		Control:     ControlSlider,
		Default:     "2700",
		Slider:      &SliderRange{Min: 300, Max: 7200, Step: 300, Unit: "s"},
		DetectFunc: func() string {
			return detectDconfInt("/org/cinnamon/settings-daemon/plugins/power/sleep-inactive-ac-timeout")
		},
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconf(ctx, o, "/org/cinnamon/settings-daemon/plugins/power/sleep-inactive-ac-timeout", v)
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconf(ctx, o, "/org/cinnamon/settings-daemon/plugins/power/lock-on-suspend", v)
		},
	},
	{
		Name:        "Screensaver lock",
		Slug:        "power",
		Category:    "Power",
		Description: "Whether the screensaver locks the screen when it activates.",
		Control:     ControlToggle,
		Default:     "false",
		DetectFunc:  func() string { return detectDconfBool("/org/cinnamon/desktop/screensaver/lock-enabled") },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconf(ctx, o, "/org/cinnamon/desktop/screensaver/lock-enabled", v)
		},
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconf(ctx, o, "/org/cinnamon/desktop/session/idle-delay", v)
		},
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			// applyKeyRepeat needs both delay and rate; detect current rate to preserve it.
			rate := detectKeyRepeatRate()
			if rate == "" {
				rate = "50"
			}
			return applyKeyRepeat(ctx, o, v, rate)
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			// applyKeyRepeat needs both delay and rate; detect current delay to preserve it.
			delay := detectKeyRepeatDelay()
			if delay == "" {
				delay = "200"
			}
			return applyKeyRepeat(ctx, o, delay, v)
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyPackages(ctx, o, []string{"numlockx"})
		},
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconfString(ctx, o, "/org/gnome/desktop/peripherals/mouse/accel-profile", v)
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconf(ctx, o, "/org/gnome/desktop/peripherals/mouse/speed", v)
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconf(ctx, o, "/org/gnome/desktop/peripherals/mouse/natural-scroll", v)
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyXbindkeys(ctx, o)
		},
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyPackages(ctx, o, []string{
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applySystemdEnable(ctx, o, "bluetooth.service")
		},
	},
	{
		Name:        "WirePlumber LDAC config",
		Slug:        "audio",
		Category:    "Audio",
		Description: "Copies a WirePlumber config that prioritizes LDAC codec for high-quality Bluetooth audio, then restarts the PipeWire stack.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  func() string { return detectFileExists("/etc/wireplumber/wireplumber.conf.d/51-ldac-hq.conf") },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyWireplumberLDAC(ctx, o)
		},
	},
	{
		Name:        "Event sounds",
		Slug:        "audio",
		Category:    "Audio",
		Description: "Controls whether desktop event sounds (alerts, notifications) play.",
		Control:     ControlToggle,
		Default:     "false",
		DetectFunc:  func() string { return detectDconfBool("/org/cinnamon/desktop/sound/event-sounds") },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconf(ctx, o, "/org/cinnamon/desktop/sound/event-sounds", v)
		},
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, v string) error {
			return applyDconfString(ctx, o, "/org/nemo/preferences/default-folder-viewer", v)
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyHideDrives(ctx, o)
		},
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyLogitechKVMFix(ctx, o)
		},
		HardwareFn: detectLogitechBolt,
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
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyWifiSuspendFix(ctx, o)
		},
		HardwareFn: detectMT7925E,
	},
	{
		Name:        "WiFi power save off",
		Slug:        "network",
		Category:    "Network",
		Description: "Drops a NetworkManager config that disables WiFi power saving so the adapter doesn't drop off the network while idle. Takes effect after the next NetworkManager restart or reboot.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  func() string { return detectFileExists("/etc/NetworkManager/conf.d/wifi-powersave-off.conf") },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyWifiPowersaveOff(ctx, o)
		},
	},
	{
		Name:        "SSD TRIM timer",
		Slug:        "system",
		Category:    "System",
		Description: "Enables periodic TRIM for SSDs, which helps maintain write performance and longevity.",
		Control:     ControlToggle,
		Default:     "active",
		DetectFunc:  func() string { return detectSystemdService("fstrim.timer") },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applySSDTrim(ctx, o)
		},
	},

	// ── SSH ──────────────────────────────────────────────────────────────

	{
		Name:        "SSH server",
		Slug:        "ssh",
		Category:    "SSH",
		Description: "Enables and starts the OpenSSH server so you can connect to this machine over SSH.",
		Control:     ControlToggle,
		Default:     "active",
		DetectFunc:  func() string { return detectSystemdService("ssh.service") },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applySSHServer(ctx, o)
		},
	},
	{
		Name:        "SSH key-based auth",
		Slug:        "ssh",
		Category:    "SSH",
		Description: "Hardens sshd for key-based login (pubkey on, keyboard-interactive off, keepalives). Password auth is disabled only once an authorized key exists, so you can't lock yourself out.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  func() string { return detectFileExists("/etc/ssh/sshd_config.d/99-ctdev.conf") },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applySSHKeyAuth(ctx, o)
		},
	},

	// ── Firewall ─────────────────────────────────────────────────────────

	{
		Name:        "Firewall (UFW)",
		Slug:        "ufw",
		Category:    "Firewall",
		Description: "Allows SSH (22/tcp) and Mosh (60000:61000/udp) from private LAN ranges and enables UFW. VLAN/subnet enforcement is left to your gateway firewall. Heads up: on a DNS or web host (Pi-hole, reverse proxy) UFW's default-deny will block those services unless you open their ports first.",
		Control:     ControlToggle,
		Default:     "active",
		DetectFunc:  func() string { return detectSystemdService("ufw.service") },
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyUFWRemote(ctx, o)
		},
	},

	// ── Locale ───────────────────────────────────────────────────────────

	{
		Name:        "UTF-8 locale (Mosh)",
		Slug:        "locale",
		Category:    "Locale",
		Description: "Generates the en_US.UTF-8 locale. Mosh refuses to start without a UTF-8 locale.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  detectUTF8Locale,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyUTF8Locale(ctx, o)
		},
	},

	// ── Sleep ────────────────────────────────────────────────────────────

	{
		Name:        "Never suspend",
		Slug:        "sleep",
		Category:    "Sleep",
		Description: "Masks the sleep, suspend, hibernate and hybrid-sleep systemd targets so an always-on machine stays reachable.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  detectSuspendMasked,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applySuspendMask(ctx, o)
		},
	},

	// ── Service lingering ────────────────────────────────────────────────

	{
		Name:        "User service lingering",
		Slug:        "linger",
		Category:    "Service Lingering",
		Description: "Enables systemd lingering for your user so user services (VS Code tunnel, tmux) keep running without an active login session.",
		Control:     ControlToggle,
		Default:     "enabled",
		DetectFunc:  detectLinger,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyLinger(ctx, o)
		},
	},

	// ── VS Code tunnel ───────────────────────────────────────────────────

	{
		Name:        "VS Code tunnel service",
		Slug:        "tunnel",
		Category:    "VS Code Tunnel",
		Description: "Installs the VS Code tunnel as a background service so you can open this machine from vscode.dev in a browser (e.g. iPad Safari). Requires VS Code; run 'code tunnel user login' once to authenticate.",
		Control:     ControlToggle,
		Default:     "installed",
		DetectFunc:  detectCodeTunnelService,
		ApplyFunc: func(ctx context.Context, o sysutil.Opts, _ string) error {
			return applyCodeTunnelService(ctx, o)
		},
	},

	// ── Pi-hole DNS ──────────────────────────────────────────────────────
	// Shown only on nodes where Pi-hole is installed (HardwareFn).

	{
		Name:        "Pi-hole upstream DNS",
		Slug:        "pihole",
		Category:    "Pi-hole DNS",
		Description: "Upstream resolvers Pi-hole forwards cache misses to.",
		Control:     ControlPicker,
		Default:     "cloudflare",
		Choices: []PickerChoice{
			{Value: "cloudflare", Description: "Cloudflare (1.1.1.1, 1.0.0.1)"},
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
}
