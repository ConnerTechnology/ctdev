package backup

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// The scheduler is one hourly runner per computer, as the user: a systemd
// user timer on Linux, a launchd agent on macOS. It runs the hidden
// `ctdev backup run-scheduled`, which backs up whichever listed folders are
// due. ctdev never enables lingering, so on Linux it runs only while the user
// is logged in (CTD-87).
const (
	systemdUnit  = "ctdev-backup"
	launchdLabel = "io.connertechnology.ctdev.backup"
)

func (m *Machine) goos() string {
	if m.GOOS != "" {
		return m.GOOS
	}
	return runtime.GOOS
}

type schedulerFile struct {
	path, content string
}

func (m *Machine) systemdDir() string { return filepath.Join(m.Home, ".config", "systemd", "user") }

func (m *Machine) launchdPath() string {
	return filepath.Join(m.Home, "Library", "LaunchAgents", launchdLabel+".plist")
}

// schedulerFiles are the files the scheduler is made of on this computer,
// with their contents.
func (m *Machine) schedulerFiles() []schedulerFile {
	switch m.goos() {
	case "linux":
		service, timer := m.systemdUnits()
		return []schedulerFile{
			{filepath.Join(m.systemdDir(), systemdUnit+".service"), service},
			{filepath.Join(m.systemdDir(), systemdUnit+".timer"), timer},
		}
	case "darwin":
		return []schedulerFile{{m.launchdPath(), m.launchdPlist()}}
	}
	return nil
}

// systemdUnits are the user service and its hourly timer. Persistent=true
// runs a missed hour once after the computer wakes or boots. The service
// runs at low priority so a backup doesn't slow the desktop down.
func (m *Machine) systemdUnits() (service, timer string) {
	service = "# Written by ctdev: backs up the folders whose schedule is due.\n" +
		"[Unit]\nDescription=ctdev scheduled folder backups\n\n" +
		"[Service]\nType=oneshot\n" +
		"ExecStart=" + strconv.Quote(m.Exe) + " backup run-scheduled\n" +
		"Nice=10\nIOSchedulingClass=idle\n"
	timer = "# Written by ctdev: runs ctdev backup run-scheduled every hour.\n" +
		"[Unit]\nDescription=Run ctdev scheduled folder backups hourly\n\n" +
		"[Timer]\nOnCalendar=hourly\nPersistent=true\n" +
		"Unit=" + systemdUnit + ".service\n\n" +
		"[Install]\nWantedBy=timers.target\n"
	return service, timer
}

// launchdPlist is the user agent. launchd runs a missed StartInterval once
// after the computer wakes.
func (m *Machine) launchdPlist() string {
	esc := func(s string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	logFile := esc(filepath.Join(m.Home, "Library", "Logs", "ctdev-backup.log"))
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<!-- Written by ctdev: runs ctdev backup run-scheduled every hour. -->
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + launchdLabel + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + esc(m.Exe) + `</string>
		<string>backup</string>
		<string>run-scheduled</string>
	</array>
	<key>StartInterval</key>
	<integer>3600</integer>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Background</string>
	<key>StandardOutPath</key>
	<string>` + logFile + `</string>
	<key>StandardErrorPath</key>
	<string>` + logFile + `</string>
</dict>
</plist>
`
}

// SchedulerInstalled reports whether this computer's hourly runner is in
// place.
func (m *Machine) SchedulerInstalled() bool {
	files := m.schedulerFiles()
	for _, f := range files {
		if _, err := os.Stat(f.path); err != nil {
			return false
		}
	}
	return len(files) > 0
}

// ErrNoScheduler means this OS has no scheduler ctdev can install.
var ErrNoScheduler = errors.New("scheduled backups run on Linux (systemd) and macOS only")

// InstallScheduler writes and starts this computer's hourly runner. It's
// safe to run again: it rewrites the files, which picks up a ctdev that has
// moved, and restarts the runner.
func (m *Machine) InstallScheduler(ctx context.Context) error {
	if !filepath.IsAbs(m.Exe) {
		// The runner gets no PATH to find ctdev on.
		return fmt.Errorf("can't schedule backups: ctdev's own path %q isn't absolute", m.Exe)
	}
	files := m.schedulerFiles()
	if len(files) == 0 {
		return ErrNoScheduler
	}
	for _, f := range files {
		if m.Opts.DryRun {
			fmt.Fprintf(m.out(), "[dry-run] write %s\n", f.path)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil {
			return err
		}
	}
	o := m.Opts
	if m.goos() == "darwin" {
		// bootout fails when the agent isn't loaded yet, which is fine.
		_ = sysutil.Run(ctx, o, "launchctl", "bootout", m.launchdDomain()+"/"+launchdLabel)
		return sysutil.Run(ctx, o, "launchctl", "bootstrap", m.launchdDomain(), m.launchdPath())
	}
	if err := sysutil.Run(ctx, o, "systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	return sysutil.Run(ctx, o, "systemctl", "--user", "enable", "--now", systemdUnit+".timer")
}

// RemoveScheduler stops this computer's hourly runner and deletes its files.
func (m *Machine) RemoveScheduler(ctx context.Context) error {
	o := m.Opts
	if m.goos() == "darwin" {
		// bootout fails when the agent isn't loaded, which is the goal anyway.
		_ = sysutil.Run(ctx, o, "launchctl", "bootout", m.launchdDomain()+"/"+launchdLabel)
	} else {
		_ = sysutil.Run(ctx, o, "systemctl", "--user", "disable", "--now", systemdUnit+".timer")
	}
	for _, f := range m.schedulerFiles() {
		if m.Opts.DryRun {
			fmt.Fprintf(m.out(), "[dry-run] remove %s\n", f.path)
			continue
		}
		if err := os.Remove(f.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if m.goos() == "linux" {
		return sysutil.Run(ctx, o, "systemctl", "--user", "daemon-reload")
	}
	return nil
}

// RemoveSchedulerIfIdle removes the hourly runner once no folder is left on
// this computer's list. Stop backing up calls it after taking a folder off.
func (m *Machine) RemoveSchedulerIfIdle(ctx context.Context) (removed bool, err error) {
	folders, err := m.scheduledFolders()
	if err != nil || len(folders) > 0 {
		return false, err
	}
	return true, m.RemoveScheduler(ctx)
}

// launchdDomain is the user's GUI session, where agents in
// ~/Library/LaunchAgents load.
func (m *Machine) launchdDomain() string {
	return "gui/" + strconv.Itoa(os.Getuid())
}
