package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/state"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// The audio and bluetooth categories split one stack cleanly: bluetooth owns
// bluez, Blueman and bluetooth.service; audio owns PipeWire and everything
// Bluetooth audio needs. Installs skip apt's recommends, because Blueman
// recommends pulseaudio-module-bluetooth, an audio package that would also
// fight PipeWire for the sound server.

// Package sets per setting. PipeWire is listed by part rather than as the
// pipewire-audio metapackage, which hard-depends on libspa-0.2-bluetooth:
// removing the Bluetooth codecs would then take the whole sound system with it.
var (
	soundSystemPackages    = []string{"pipewire", "pipewire-pulse", "pipewire-alsa", "wireplumber", "pulseaudio-utils"}
	bluetoothAudioPackages = []string{"libspa-0.2-bluetooth"}
	bluetoothPackages      = []string{"bluez", "blueman"}
)

const (
	bluetoothService = "bluetooth.service"
	ldacConfDir      = "/etc/wireplumber/wireplumber.conf.d"
	ldacConfPath     = ldacConfDir + "/51-ldac-hq.conf"
)

// Kernel class directories the hardware gates read; tests point them at fake
// trees.
var (
	sysClassBluetooth = "/sys/class/bluetooth"
	sysClassSound     = "/sys/class/sound"
)

// gateBluetoothAdapter reports whether this Linux host has a Bluetooth
// adapter. The kernel lists each one under /sys/class/bluetooth.
func gateBluetoothAdapter() bool {
	return gateLinux() && dirHasEntry(sysClassBluetooth, "")
}

// gateSoundCard reports whether this Linux host has a sound card. The kernel
// lists each one as /sys/class/sound/card<N>.
func gateSoundCard() bool {
	return gateLinux() && dirHasEntry(sysClassSound, "card")
}

// dirHasEntry reports whether dir holds an entry whose name starts with prefix.
func dirHasEntry(dir, prefix string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			return true
		}
	}
	return false
}

// packageInstalled and systemctlQuery are the probes the apply paths use;
// tests swap in fakes.
var (
	packageInstalled = detectPackageInstalled
	systemctlQuery   = func(ctx context.Context, verb, unit string) string {
		out, _ := runOutput(ctx, "systemctl", verb, unit)
		return out
	}
)

// addedDir holds one file per setting listing what ctdev added for it, so
// turning the setting off removes exactly that and nothing the machine
// already had.
var addedDir = func() string { return filepath.Join(state.StateDir(), "configure-added") }

func readAdded(key string) []string {
	data, err := os.ReadFile(filepath.Join(addedDir(), key))
	if err != nil {
		return nil
	}
	return strings.Fields(string(data))
}

func recordAdded(o sysutil.Opts, key string, items ...string) error {
	if o.DryRun || len(items) == 0 {
		return nil
	}
	all := append(readAdded(key), items...)
	if err := os.MkdirAll(addedDir(), 0o755); err != nil {
		return fmt.Errorf("record what ctdev added: %w", err)
	}
	return os.WriteFile(filepath.Join(addedDir(), key), []byte(strings.Join(all, "\n")+"\n"), 0o644)
}

func clearAdded(o sysutil.Opts, key string) error {
	if o.DryRun {
		return nil
	}
	if err := os.Remove(filepath.Join(addedDir(), key)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// detectPackages reports "installed" when every package is installed.
func detectPackages(ctx context.Context, pkgs []string) string {
	for _, p := range pkgs {
		if !packageInstalled(ctx, p) {
			return "not installed"
		}
	}
	return "installed"
}

// applyOwnedPackages installs the missing packages in pkgs and records them
// under key. "not installed" removes the packages recorded under key and
// leaves alone any the machine had before ctdev.
func applyOwnedPackages(ctx context.Context, o sysutil.Opts, key string, pkgs []string, v string) error {
	if v == "not installed" {
		added := readAdded(key)
		if len(added) == 0 {
			fmt.Fprintf(o.Stdout, "  ctdev didn't install %s; leaving them in place.\n", strings.Join(pkgs, ", "))
			return nil
		}
		args := append([]string{"apt-get", "remove", "-y", "-qq"}, added...)
		if err := sysutil.SudoRun(ctx, o, args[0], args[1:]...); err != nil {
			return err
		}
		return clearAdded(o, key)
	}

	var missing []string
	for _, p := range pkgs {
		if !packageInstalled(ctx, p) {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	args := append([]string{"apt-get", "install", "-y", "-qq", "--no-install-recommends"}, missing...)
	if err := sysutil.SudoRun(ctx, o, args[0], args[1:]...); err != nil {
		return err
	}
	return recordAdded(o, key, missing...)
}

// detectBluetoothService reports "active" or "inactive".
func detectBluetoothService(ctx context.Context) string {
	if systemctlQuery(ctx, "is-active", bluetoothService) == "active" {
		return "active"
	}
	return "inactive"
}

// applyBluetoothService enables and starts bluetooth.service, recording which
// of the two ctdev did. "inactive" undoes exactly those.
func applyBluetoothService(ctx context.Context, o sysutil.Opts, v string) error {
	const key = "bluetooth-service"
	if v == "inactive" {
		added := readAdded(key)
		if len(added) == 0 {
			fmt.Fprintf(o.Stdout, "  ctdev didn't turn on %s; leaving it as it is.\n", bluetoothService)
			return nil
		}
		for _, verb := range added {
			undo := map[string]string{"enable": "disable", "start": "stop"}[verb]
			if undo == "" {
				continue
			}
			if err := sysutil.SudoRun(ctx, o, "systemctl", undo, bluetoothService); err != nil {
				return fmt.Errorf("systemctl %s %s: %w", undo, bluetoothService, err)
			}
		}
		return clearAdded(o, key)
	}

	var did []string
	if systemctlQuery(ctx, "is-enabled", bluetoothService) != "enabled" {
		if err := sysutil.SudoRun(ctx, o, "systemctl", "enable", bluetoothService); err != nil {
			return fmt.Errorf("systemctl enable %s: %w", bluetoothService, err)
		}
		did = append(did, "enable")
	}
	if systemctlQuery(ctx, "is-active", bluetoothService) != "active" {
		if err := sysutil.SudoRun(ctx, o, "systemctl", "start", bluetoothService); err != nil {
			return fmt.Errorf("systemctl start %s: %w", bluetoothService, err)
		}
		did = append(did, "start")
	}
	return recordAdded(o, key, did...)
}

// applyLDAC writes ctdev's WirePlumber LDAC config, or for "not installed"
// removes it, then restarts the PipeWire stack to pick up the change.
func applyLDAC(ctx context.Context, o sysutil.Opts, v string) error {
	if v == "not installed" {
		if err := sysutil.SudoRun(ctx, o, "rm", "-f", ldacConfPath); err != nil {
			return fmt.Errorf("remove wireplumber config: %w", err)
		}
	} else {
		if err := sysutil.SudoRun(ctx, o, "mkdir", "-p", ldacConfDir); err != nil {
			return fmt.Errorf("mkdir wireplumber conf dir: %w", err)
		}
		content, err := Configs.ReadFile("configs/wireplumber/51-ldac-hq.conf")
		if err != nil {
			return fmt.Errorf("read embedded wireplumber config: %w", err)
		}
		if err := sysutil.SudoWriteFile(ctx, o, string(content), ldacConfPath); err != nil {
			return fmt.Errorf("copy wireplumber config: %w", err)
		}
	}

	// Best-effort: the config applies on the next login if this fails.
	_ = sysutil.Run(ctx, o, "systemctl", "--user", "restart",
		"pipewire", "pipewire-pulse", "wireplumber")
	return nil
}
