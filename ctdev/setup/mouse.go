package setup

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
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
	mousePackages     = applyPackages
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

// installState names a setting's state from whether all, some or none of
// what ctdev installs is in place. "partly installed" matches neither toggle
// value, so the wizard applies whichever one the user picks.
func installState(all, any bool) string {
	switch {
	case all:
		return "installed"
	case any:
		return "partly installed"
	}
	return "not installed"
}

// newestBackup returns the most recent backup DeployFile made of path, or "".
// Backup names end in a sortable timestamp (sysutil.BackupPath).
func newestBackup(path string) string {
	matches, err := filepath.Glob(path + ".*.bak")
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	return matches[len(matches)-1]
}

// restoreBackup puts back the newest backup of path, honoring dry-run.
// Reports whether one was restored.
func restoreBackup(o sysutil.Opts, path string) (bool, error) {
	backup := newestBackup(path)
	if backup == "" {
		return false, nil
	}
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] restore %s from %s\n", path, backup)
		return true, nil
	}
	if err := os.Rename(backup, path); err != nil {
		return false, fmt.Errorf("restore %s: %w", path, err)
	}
	fmt.Fprintf(o.Stdout, "Restored %s from %s\n", path, backup)
	return true, nil
}

// ── Middle-click opens the window overview ──────────────────────────────

// detectXbindkeys reports "installed" when xbindkeys and xdotool are present
// and ctdev's binding config and autostart entry are both in place, and
// "partly installed" when only one of ctdev's files is.
func detectXbindkeys(ctx context.Context) string {
	rc, autostart, err := xbindkeysPaths()
	if err != nil {
		return "not installed"
	}
	ours := isCtdevFile(rc, xbindkeysrcSrc)
	ourAutostart := isCtdevFile(autostart, xbindkeysAutoSrc)
	all := ours && ourAutostart &&
		mousePkgInstalled(ctx, "xbindkeys") && mousePkgInstalled(ctx, "xdotool")
	return installState(all, ours || ourAutostart)
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
	if err := mousePackages(ctx, o, []string{"xbindkeys", "xdotool"}); err != nil {
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

// removeXbindkeys removes ctdev's binding config and autostart entry and
// puts back whatever ctdev backed up when it installed them. A file that no
// longer matches ctdev's copy was edited by the user, so it stays. xbindkeys
// is restarted only when its config changed: stopped if ctdev's was removed,
// started again if the user's own config is back. The xbindkeys and xdotool
// packages stay: other bindings may use them, and ctdev can't tell whether it
// installed them.
func removeXbindkeys(ctx context.Context, o sysutil.Opts) error {
	rc, autostart, err := xbindkeysPaths()
	if err != nil {
		return err
	}
	removedRC, restoredRC := false, false
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
		restored, err := restoreBackup(o, f.path)
		if err != nil {
			return err
		}
		if f.path == rc {
			removedRC, restoredRC = true, restored
		}
	}
	if removedRC {
		_ = mouseRun(ctx, o, "killall", "xbindkeys")
		if restoredRC && !o.DryRun {
			_ = mouseStartXbind()
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
// Solaar restart user service are in place, and "partly installed" when
// only one is.
func detectLogitechKVMFix(_ context.Context) string {
	svc, err := solaarServicePath()
	all := err == nil && fileExists(logitechUdevRule) && fileExists(svc)
	return installState(all, logitechFixPresent())
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
