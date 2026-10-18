package setup

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// The two mouse settings that install files: the middle-click binding
// (xbindkeys) and the Logitech KVM fix. Each has an off path that removes
// exactly what its on path added.

// Seams for tests: the shell-outs and system paths the mouse settings touch.
var (
	mouseRun          = sysutil.Run
	mouseSudoRun      = sysutil.SudoRun
	mouseSudoWrite    = sysutil.SudoWriteFile
	mousePkgInstalled = detectPackageInstalled
	mouseStartXbind   = func() error { return startDetached("xbindkeys") }

	logitechUdevRule = "/etc/udev/rules.d/99-logitech-kvm-fix.rules"
)

const (
	xbindkeysrcSrc   = "configs/xbindkeys/.xbindkeysrc"
	xbindkeysAutoSrc = "configs/xbindkeys/xbindkeys.desktop"
	logitechRuleSrc  = "configs/udev/99-logitech-kvm-fix.rules"
	solaarServiceSrc = "configs/udev/solaar-restart.service"
	solaarService    = "solaar-restart.service"
)

// xbindkeysPaths returns where ctdev puts the binding config and its autostart entry.
func xbindkeysPaths() (rc, autostart string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".xbindkeysrc"),
		filepath.Join(home, ".config", "autostart", "xbindkeys.desktop"), nil
}

// solaarServicePath returns where ctdev puts the Solaar restart user service.
func solaarServicePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user", solaarService), nil
}

// isCtdevFile reports whether path holds exactly the embedded file src.
func isCtdevFile(path, src string) bool {
	want, err := Configs.ReadFile(src)
	if err != nil {
		return false
	}
	got, err := os.ReadFile(path)
	return err == nil && bytes.Equal(got, want)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// removeUserFile deletes a file in the user's home, honoring dry-run.
func removeUserFile(o sysutil.Opts, path string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] remove %s\n", path)
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// mouseSpeedStep is the mouse speed slider's step.
const mouseSpeedStep = 0.01

// roundMouseSpeed rounds a stored speed to the slider's step. The desktop's
// own speed slider stores values like 0.46218487394957974, which would never
// match a recommended 0.46. Values that aren't numbers pass through.
func roundMouseSpeed(raw string) string {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return raw
	}
	return FormatSliderVal(math.Round(v/mouseSpeedStep)*mouseSpeedStep, mouseSpeedStep)
}

// ── Middle-click opens the window overview ──────────────────────────────

// detectXbindkeys reports "installed" when xbindkeys and xdotool are present
// and ctdev's binding config and autostart entry are both in place.
func detectXbindkeys(ctx context.Context) string {
	rc, autostart, err := xbindkeysPaths()
	if err != nil {
		return "not installed"
	}
	if mousePkgInstalled(ctx, "xbindkeys") && mousePkgInstalled(ctx, "xdotool") &&
		isCtdevFile(rc, xbindkeysrcSrc) && isCtdevFile(autostart, xbindkeysAutoSrc) {
		return "installed"
	}
	return "not installed"
}

// setXbindkeys turns the middle-click binding on ("installed") or off.
func setXbindkeys(ctx context.Context, o sysutil.Opts, v string) error {
	if v == "not installed" {
		return removeXbindkeys(ctx, o)
	}
	return applyXbindkeys(ctx, o)
}

// applyXbindkeys installs xbindkeys and xdotool, deploys the config, and sets up autostart.
func applyXbindkeys(ctx context.Context, o sysutil.Opts) error {
	if err := mouseSudoRun(ctx, o, "apt-get", "install", "-y", "-qq", "xbindkeys", "xdotool"); err != nil {
		return fmt.Errorf("install xbindkeys/xdotool: %w", err)
	}

	rc, autostart, err := xbindkeysPaths()
	if err != nil {
		return err
	}

	// Use DeployFileFromFS so any pre-existing user-customized file gets
	// backed up on diff instead of silently overwritten.
	if err := sysutil.DeployFileFromFS(Configs, xbindkeysrcSrc, rc); err != nil {
		return fmt.Errorf("deploy .xbindkeysrc: %w", err)
	}
	if err := sysutil.DeployFileFromFS(Configs, xbindkeysAutoSrc, autostart); err != nil {
		return fmt.Errorf("deploy xbindkeys.desktop: %w", err)
	}

	// Restart xbindkeys to pick up new config; best-effort.
	_ = mouseRun(ctx, o, "killall", "xbindkeys")
	// Fire-and-forget: don't block on xbindkeys.
	_ = mouseStartXbind()

	return nil
}

// removeXbindkeys stops xbindkeys and removes ctdev's binding config and
// autostart entry. A file that no longer matches ctdev's copy was edited by
// the user, so it stays. The xbindkeys and xdotool packages stay too: other
// bindings may use them, and ctdev can't tell whether it installed them.
func removeXbindkeys(ctx context.Context, o sysutil.Opts) error {
	rc, autostart, err := xbindkeysPaths()
	if err != nil {
		return err
	}
	_ = mouseRun(ctx, o, "killall", "xbindkeys")
	for _, f := range []struct{ path, src string }{
		{autostart, xbindkeysAutoSrc},
		{rc, xbindkeysrcSrc},
	} {
		if !fileExists(f.path) {
			continue
		}
		if !isCtdevFile(f.path, f.src) {
			fmt.Fprintf(o.Stdout, "Leaving %s: it differs from ctdev's copy\n", f.path)
			continue
		}
		if err := removeUserFile(o, f.path); err != nil {
			return err
		}
	}
	return nil
}

// ── Keep the Logitech mouse working after a KVM switch ──────────────────

// logitechFixPresent reports whether either piece of the KVM fix is on disk.
func logitechFixPresent() bool {
	if fileExists(logitechUdevRule) {
		return true
	}
	svc, err := solaarServicePath()
	return err == nil && fileExists(svc)
}

// gateLogitechKVMFix shows the fix when a Logi Bolt receiver is plugged in,
// or when the fix is installed so it can still be turned off.
func gateLogitechKVMFix() bool {
	return gateLinux() && (logitechFixPresent() || detectLogitechBolt())
}

// detectLogitechKVMFix reports "installed" when both the udev rule and the
// Solaar restart user service are in place.
func detectLogitechKVMFix(_ context.Context) string {
	svc, err := solaarServicePath()
	if err == nil && fileExists(logitechUdevRule) && fileExists(svc) {
		return "installed"
	}
	return "not installed"
}

// setLogitechKVMFix turns the KVM fix on ("installed") or off.
func setLogitechKVMFix(ctx context.Context, o sysutil.Opts, v string) error {
	if v == "not installed" {
		return removeLogitechKVMFix(ctx, o)
	}
	return applyLogitechKVMFix(ctx, o)
}

// applyLogitechKVMFix installs a udev rule and systemd user service to restart Solaar
// when the Logi Bolt receiver reconnects after a KVM switch.
func applyLogitechKVMFix(ctx context.Context, o sysutil.Opts) error {
	// Deploy udev rule
	udevContent, err := Configs.ReadFile(logitechRuleSrc)
	if err != nil {
		return fmt.Errorf("read embedded udev rule: %w", err)
	}
	if err := mouseSudoWrite(ctx, o, string(udevContent), logitechUdevRule); err != nil {
		return fmt.Errorf("copy udev rule: %w", err)
	}

	// Deploy systemd user service
	serviceDst, err := solaarServicePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(serviceDst), 0o755); err != nil {
		return fmt.Errorf("mkdir systemd user dir: %w", err)
	}
	serviceContent, err := Configs.ReadFile(solaarServiceSrc)
	if err != nil {
		return fmt.Errorf("read embedded service file: %w", err)
	}
	if err := os.WriteFile(serviceDst, serviceContent, 0o644); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}

	// Enable the user service and reload udev
	_ = mouseRun(ctx, o, "systemctl", "--user", "daemon-reload")
	_ = mouseRun(ctx, o, "systemctl", "--user", "enable", solaarService)
	if err := mouseSudoRun(ctx, o, "udevadm", "control", "--reload-rules"); err != nil {
		return fmt.Errorf("reload udev rules: %w", err)
	}

	return nil
}

// removeLogitechKVMFix disables and removes the Solaar restart user service
// and the udev rule that triggers it.
func removeLogitechKVMFix(ctx context.Context, o sysutil.Opts) error {
	svc, err := solaarServicePath()
	if err != nil {
		return err
	}
	if fileExists(svc) {
		_ = mouseRun(ctx, o, "systemctl", "--user", "disable", solaarService)
		if err := removeUserFile(o, svc); err != nil {
			return err
		}
		_ = mouseRun(ctx, o, "systemctl", "--user", "daemon-reload")
	}
	if fileExists(logitechUdevRule) {
		if err := mouseSudoRun(ctx, o, "rm", "-f", logitechUdevRule); err != nil {
			return fmt.Errorf("remove logitech kvm udev rule: %w", err)
		}
		if err := mouseSudoRun(ctx, o, "udevadm", "control", "--reload-rules"); err != nil {
			return fmt.Errorf("reload udev rules: %w", err)
		}
	}
	return nil
}
