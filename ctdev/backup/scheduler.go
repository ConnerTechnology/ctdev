package backup

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

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

// runCmd runs a scheduler command: sysutil.Run, or the fake a test set.
func (m *Machine) runCmd(ctx context.Context, o sysutil.Opts, name string, args ...string) error {
	if m.run != nil {
		return m.run(ctx, o, name, args...)
	}
	return sysutil.Run(ctx, o, name, args...)
}

type schedulerFile struct {
	path, content string
}

func (m *Machine) systemdDir() string { return filepath.Join(m.Home, ".config", "systemd", "user") }

// timerWants is the link `systemctl --user enable` makes; without it the
// timer doesn't start at login.
func (m *Machine) timerWants() string {
	return filepath.Join(m.systemdDir(), "timers.target.wants", systemdUnit+".timer")
}

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
	// systemd expands % specifiers in ExecStart, so a literal % is %%.
	exe := strings.ReplaceAll(strconv.Quote(m.Exe), "%", "%%")
	service = "# Written by ctdev: backs up the folders whose schedule is due.\n" +
		"[Unit]\nDescription=ctdev scheduled folder backups\n\n" +
		"[Service]\nType=oneshot\n" +
		"ExecStart=" + exe + " backup run-scheduled\n" +
		"Nice=10\nIOSchedulingClass=idle\n"
	timer = "# Written by ctdev: runs ctdev backup run-scheduled every hour.\n" +
		"[Unit]\nDescription=Run ctdev scheduled folder backups hourly\n\n" +
		"[Timer]\nOnCalendar=hourly\nPersistent=true\n" +
		"Unit=" + systemdUnit + ".service\n\n" +
		"[Install]\nWantedBy=timers.target\n"
	return service, timer
}

// launchdPlist is the user agent. It runs at the top of every hour; launchd
// runs a calendar interval missed while the computer slept once on wake, as
// systemd's Persistent=true does. RunAtLoad runs it once when it's loaded.
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
	<key>StartCalendarInterval</key>
	<dict>
		<key>Minute</key>
		<integer>0</integer>
	</dict>
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
// place: its files, and on Linux the link enabling the timer. It only looks
// at files, so it's read-only and quick.
func (m *Machine) SchedulerInstalled() bool {
	files := m.schedulerFiles()
	for _, f := range files {
		if _, err := os.Stat(f.path); err != nil {
			return false
		}
	}
	if m.goos() == "linux" {
		if _, err := os.Lstat(m.timerWants()); err != nil {
			return false
		}
	}
	return len(files) > 0
}

// lingering reports whether systemd keeps this user's timers running after
// logout. ctdev never turns it on; it only reads whether it's on.
func (m *Machine) lingering() bool {
	dir := m.lingerDir
	if dir == "" {
		dir = "/var/lib/systemd/linger"
	}
	if m.Username == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, m.Username))
	return err == nil
}

// ErrNoScheduler means this OS has no scheduler ctdev can install.
var ErrNoScheduler = errors.New("scheduled backups run on Linux (systemd) and macOS only")

// InstallScheduler writes and starts this computer's hourly runner. It's
// safe to run again: it rewrites the files, which picks up a ctdev that has
// moved. When it fails, nothing of it is left behind, so the status says
// "not scheduled on this computer".
func (m *Machine) InstallScheduler(ctx context.Context) error {
	if !filepath.IsAbs(m.Exe) {
		// The runner gets no PATH to find ctdev on.
		return fmt.Errorf("can't schedule backups: ctdev's own path %q isn't absolute", m.Exe)
	}
	switch m.goos() {
	case "linux":
		return m.installSystemd(ctx)
	case "darwin":
		return m.installLaunchd(ctx)
	}
	return ErrNoScheduler
}

func (m *Machine) writeSchedulerFile(f schedulerFile) error {
	if m.Opts.DryRun {
		fmt.Fprintf(m.out(), "[dry-run] write %s\n", f.path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(f.path, []byte(f.content), 0o644)
}

func (m *Machine) installSystemd(ctx context.Context) error {
	files := m.schedulerFiles()
	for _, f := range files {
		if err := m.writeSchedulerFile(f); err != nil {
			return err
		}
	}
	o := m.Opts
	err := m.runCmd(ctx, o, "systemctl", "--user", "daemon-reload")
	if err == nil {
		err = m.runCmd(ctx, o, "systemctl", "--user", "enable", "--now", systemdUnit+".timer")
	}
	if err != nil && !m.Opts.DryRun {
		// Units systemd wouldn't take are no use; leave none behind.
		for _, f := range files {
			_ = os.Remove(f.path)
		}
		_ = m.runCmd(ctx, o, "systemctl", "--user", "daemon-reload")
		return fmt.Errorf("enable the hourly timer: %w", err)
	}
	return err
}

// installLaunchd writes the agent and loads it. Reloading stops a running
// agent, so it leaves a loaded agent with the same plist alone, and when the
// plist changed during a scheduled run it leaves the new one for the next
// load rather than kill the backup in progress.
func (m *Machine) installLaunchd(ctx context.Context) error {
	f := m.schedulerFiles()[0]
	target := m.launchdDomain() + "/" + launchdLabel
	quiet := m.Opts // launchctl's complaints about an agent that isn't loaded are noise
	if !quiet.DryRun {
		quiet.Stdout = io.Discard
	}
	old, err := os.ReadFile(f.path)
	if err == nil && string(old) == f.content {
		if m.runCmd(ctx, quiet, "launchctl", "print", target) == nil {
			return nil // loaded, and already what it would be
		}
	} else if err := m.writeSchedulerFile(f); err != nil {
		return err
	}
	if !m.Opts.DryRun && m.runInProgress() {
		fmt.Fprintln(m.out(), "A scheduled backup is running, so the new schedule takes effect at your next login.")
		return nil
	}
	// bootout fails when the agent isn't loaded yet, which is fine.
	_ = m.runCmd(ctx, quiet, "launchctl", "bootout", target)
	if err := m.runCmd(ctx, m.Opts, "launchctl", "bootstrap", m.launchdDomain(), f.path); err != nil {
		if !m.Opts.DryRun {
			_ = os.Remove(f.path)
		}
		return fmt.Errorf("load the hourly agent: %w", err)
	}
	return nil
}

// RemoveScheduler stops this computer's hourly runner and deletes its files.
func (m *Machine) RemoveScheduler(ctx context.Context) error {
	o := m.Opts
	if m.goos() == "darwin" {
		// bootout fails when the agent isn't loaded, which is the goal anyway.
		_ = m.runCmd(ctx, o, "launchctl", "bootout", m.launchdDomain()+"/"+launchdLabel)
	} else {
		_ = m.runCmd(ctx, o, "systemctl", "--user", "disable", "--now", systemdUnit+".timer")
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
		return m.runCmd(ctx, o, "systemctl", "--user", "daemon-reload")
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
