package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

const exampleExe = "/opt/example tools/bin/ctdev"

// schedulerMachine is a dry-run machine on goos whose ctdev is exampleExe.
func schedulerMachine(t *testing.T, goos string) (*Machine, *bytes.Buffer) {
	t.Helper()
	m := testMachine(t, "example-laptop")
	out := &bytes.Buffer{}
	m.Opts.Stdout, m.Opts.DryRun = out, true
	m.GOOS, m.Exe = goos, exampleExe
	// No agent is loaded, and the real launchctl is never asked.
	m.capture = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("Could not find service")
	}
	return m, out
}

func TestSystemdUnitsRunTheScheduledBackupHourly(t *testing.T) {
	m, _ := schedulerMachine(t, "linux")
	service, timer := m.systemdUnits()

	for _, want := range []string{
		"[Service]\nType=oneshot\n",
		`ExecStart="/opt/example tools/bin/ctdev" backup run-scheduled` + "\n",
	} {
		if !strings.Contains(service, want) {
			t.Errorf("service lacks %q:\n%s", want, service)
		}
	}
	for _, want := range []string{
		"[Timer]\nOnCalendar=hourly\nPersistent=true\n",
		"Unit=ctdev-backup.service\n",
		"[Install]\nWantedBy=timers.target\n",
	} {
		if !strings.Contains(timer, want) {
			t.Errorf("timer lacks %q:\n%s", want, timer)
		}
	}
	// No linger: a user timer runs while the user is logged in, and ctdev
	// never changes that.
	if strings.Contains(service+timer, "linger") {
		t.Error("the units mention linger")
	}
}

func TestLaunchdAgentRunsTheScheduledBackupHourly(t *testing.T) {
	m, _ := schedulerMachine(t, "darwin")
	plist := m.launchdPlist()
	for _, want := range []string{
		"<key>Label</key>\n\t<string>io.connertechnology.ctdev.backup</string>",
		"<key>ProgramArguments</key>\n\t<array>\n\t\t<string>/opt/example tools/bin/ctdev</string>\n\t\t<string>backup</string>\n\t\t<string>run-scheduled</string>\n\t</array>",
		"<key>StartCalendarInterval</key>\n\t<dict>\n\t\t<key>Minute</key>\n\t\t<integer>0</integer>\n\t</dict>",
		"<key>RunAtLoad</key>\n\t<true/>",
		"<string>" + filepath.Join(m.Home, "Library", "Logs", "ctdev-backup.log") + "</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist lacks %q:\n%s", want, plist)
		}
	}
	if !strings.HasPrefix(plist, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Errorf("plist doesn't start with the XML declaration:\n%s", plist)
	}
}

// The binary path goes into XML escaped.
func TestLaunchdPlistEscapesThePath(t *testing.T) {
	m, _ := schedulerMachine(t, "darwin")
	m.Exe = "/opt/a&b/ctdev"
	if plist := m.launchdPlist(); !strings.Contains(plist, "<string>/opt/a&amp;b/ctdev</string>") {
		t.Errorf("plist doesn't escape &:\n%s", plist)
	}
}

func TestInstallAndRemoveTheScheduler(t *testing.T) {
	uid := os.Getuid()
	for goos, tc := range map[string]struct {
		files   []string
		install []string
		remove  []string
	}{
		"linux": {
			files: []string{".config/systemd/user/ctdev-backup.service", ".config/systemd/user/ctdev-backup.timer"},
			install: []string{
				"[dry-run] systemctl --user daemon-reload",
				"[dry-run] systemctl --user enable --now ctdev-backup.timer",
			},
			remove: []string{
				"[dry-run] systemctl --user disable --now ctdev-backup.timer",
				"[dry-run] systemctl --user daemon-reload",
			},
		},
		"darwin": {
			files: []string{"Library/LaunchAgents/io.connertechnology.ctdev.backup.plist"},
			install: []string{
				fmt.Sprintf("[dry-run] launchctl bootout gui/%d/io.connertechnology.ctdev.backup", uid),
				fmt.Sprintf("[dry-run] launchctl bootstrap gui/%d HOME/Library/LaunchAgents/io.connertechnology.ctdev.backup.plist", uid),
			},
			remove: []string{
				fmt.Sprintf("[dry-run] launchctl bootout gui/%d/io.connertechnology.ctdev.backup", uid),
			},
		},
	} {
		t.Run(goos, func(t *testing.T) {
			m, out := schedulerMachine(t, goos)
			ctx := context.Background()
			if err := m.InstallScheduler(ctx); err != nil {
				t.Fatal(err)
			}
			got := strings.ReplaceAll(out.String(), m.Home, "HOME")
			for _, f := range tc.files {
				if want := "[dry-run] write HOME/" + f; !strings.Contains(got, want) {
					t.Errorf("install output lacks %q:\n%s", want, got)
				}
				if _, err := os.Stat(filepath.Join(m.Home, f)); err == nil {
					t.Errorf("a dry run wrote %s", f)
				}
			}
			for _, want := range tc.install {
				if !strings.Contains(got, want) {
					t.Errorf("install output lacks %q:\n%s", want, got)
				}
			}
			if strings.Contains(got, "linger") {
				t.Errorf("install enables linger:\n%s", got)
			}

			out.Reset()
			if err := m.RemoveScheduler(ctx); err != nil {
				t.Fatal(err)
			}
			got = strings.ReplaceAll(out.String(), m.Home, "HOME")
			for _, want := range tc.remove {
				if !strings.Contains(got, want) {
					t.Errorf("remove output lacks %q:\n%s", want, got)
				}
			}
			for _, f := range tc.files {
				if want := "[dry-run] remove HOME/" + f; !strings.Contains(got, want) {
					t.Errorf("remove output lacks %q:\n%s", want, got)
				}
			}
		})
	}
}

// writeSchedulerFiles puts the scheduler's files in place as an install
// would, without running anything.
func writeSchedulerFiles(t *testing.T, m *Machine) {
	t.Helper()
	for _, f := range m.schedulerFiles() {
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// enableTimer makes the link `systemctl --user enable` would.
func enableTimer(t *testing.T, m *Machine) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(m.timerWants()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(m.systemdDir(), systemdUnit+".timer"), m.timerWants()); err != nil {
		t.Fatal(err)
	}
}

// The scheduler counts as installed when its files are there, and on Linux
// when the timer is enabled too; a dry run installs nothing.
func TestSchedulerInstalledFollowsItsFiles(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		m, _ := schedulerMachine(t, goos)
		if m.SchedulerInstalled() {
			t.Errorf("%s: installed before anything was written", goos)
		}
		writeSchedulerFiles(t, m)
		if goos == "linux" {
			if m.SchedulerInstalled() {
				t.Error("linux: installed with the units written but the timer not enabled")
			}
			enableTimer(t, m)
		}
		if !m.SchedulerInstalled() {
			t.Errorf("%s: not installed with its files in place", goos)
		}
	}
}

// fakeCommands answers the scheduler's systemctl and launchctl commands
// instead of running them, failing the ones named in fail. loaded, when set,
// is the program and arguments `launchctl print` reports for the agent.
type fakeCommands struct {
	ran    []string
	fail   map[string]bool // "systemctl --user enable", ...
	loaded []string
	// before runs ahead of each command, to stand in for its side effects.
	before func(line string)
}

func (f *fakeCommands) run(_ context.Context, _ sysutil.Opts, name string, args ...string) error {
	line := strings.Join(append([]string{name}, args...), " ")
	f.ran = append(f.ran, line)
	if f.before != nil {
		f.before(line)
	}
	for prefix := range f.fail {
		if strings.HasPrefix(line, prefix) {
			return errors.New("example failure")
		}
	}
	return nil
}

func (f *fakeCommands) capture(_ context.Context, name string, args ...string) ([]byte, error) {
	if f.loaded == nil {
		return nil, errors.New("Could not find service")
	}
	out := "gui/501/io.connertechnology.ctdev.backup = {\n\tactive count = 0\n\tpath = /example/plist\n\tprogram = " +
		f.loaded[0] + "\n\targuments = {\n"
	for _, a := range f.loaded {
		out += "\t\t" + a + "\n"
	}
	return []byte(out + "\t}\n\n\tinherited environment = {\n\t\tHOME => /example\n\t}\n}\n"), nil
}

// realMachine is a machine that writes files but whose commands go to a fake.
func realMachine(t *testing.T, goos string, fail ...string) (*Machine, *fakeCommands, *bytes.Buffer) {
	t.Helper()
	m, out := schedulerMachine(t, goos)
	m.Opts.DryRun = false
	f := &fakeCommands{fail: map[string]bool{}}
	for _, c := range fail {
		f.fail[c] = true
	}
	m.run, m.capture = f.run, f.capture
	return m, f, out
}

// A timer systemd wouldn't enable leaves no units and no enable link
// behind, so the status shows "not scheduled on this computer".
func TestAFailedEnableLeavesNoScheduler(t *testing.T) {
	m, cmds, _ := realMachine(t, "linux", "systemctl --user enable")
	// enable can make its link and still fail.
	cmds.before = func(line string) {
		if strings.HasPrefix(line, "systemctl --user enable") {
			enableTimer(t, m)
		}
	}
	if err := m.InstallScheduler(context.Background()); err == nil {
		t.Fatal("InstallScheduler succeeded though enable failed")
	}
	for _, f := range m.schedulerFiles() {
		if _, err := os.Stat(f.path); err == nil {
			t.Errorf("%s was left behind", f.path)
		}
	}
	if _, err := os.Lstat(m.timerWants()); err == nil {
		t.Error("the enable link was left behind")
	}
	if m.SchedulerInstalled() {
		t.Error("a scheduler that failed to enable counts as installed")
	}
	if got := cmds.ran[len(cmds.ran)-1]; got != "systemctl --user daemon-reload" {
		t.Errorf("last command %q, want a daemon-reload after removing the units", got)
	}

	m, _, _ = realMachine(t, "linux", "systemctl --user daemon-reload")
	if err := m.InstallScheduler(context.Background()); err == nil || m.SchedulerInstalled() {
		t.Errorf("a failed daemon-reload: err %v, installed %v", err, m.SchedulerInstalled())
	}
}

// A failed reinstall over a working scheduler puts the old units back and
// keeps the enable link: the scheduler that worked keeps working.
func TestAFailedReinstallKeepsTheWorkingScheduler(t *testing.T) {
	m, _, _ := realMachine(t, "linux", "systemctl --user enable")
	m.Exe = "/opt/example/old/ctdev"
	writeSchedulerFiles(t, m)
	enableTimer(t, m)
	oldService, oldTimer := m.systemdUnits()

	m.Exe = exampleExe
	if err := m.InstallScheduler(context.Background()); err == nil {
		t.Fatal("InstallScheduler succeeded though enable failed")
	}
	files := m.schedulerFiles()
	for i, want := range []string{oldService, oldTimer} {
		if got, _ := os.ReadFile(files[i].path); string(got) != want {
			t.Errorf("%s = %q, want its old contents back", files[i].path, got)
		}
	}
	if !m.SchedulerInstalled() {
		t.Error("the working scheduler was torn down")
	}
}

func TestInstallSystemdWritesTheUnits(t *testing.T) {
	m, cmds, _ := realMachine(t, "linux")
	if err := m.InstallScheduler(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := "systemctl --user daemon-reload,systemctl --user enable --now ctdev-backup.timer"; strings.Join(cmds.ran, ",") != want {
		t.Errorf("ran %q, want %q", cmds.ran, want)
	}
	service, _ := m.systemdUnits()
	if got, _ := os.ReadFile(m.schedulerFiles()[0].path); string(got) != service {
		t.Errorf("service file = %q", got)
	}
}

func TestLaunchdArgsReadsLaunchctlPrint(t *testing.T) {
	f := &fakeCommands{loaded: []string{"/opt/example tools/bin/ctdev", "backup", "run-scheduled"}}
	out, _ := f.capture(context.Background(), "launchctl")
	if got := launchdArgs(out); strings.Join(got, "|") != "/opt/example tools/bin/ctdev|backup|run-scheduled" {
		t.Errorf("launchdArgs = %q", got)
	}
	if got := launchdArgs([]byte("no agent here")); got != nil {
		t.Errorf("launchdArgs of nothing = %q", got)
	}
}

// Reloading a launchd agent stops a backup it's running, so Configure
// leaves alone an agent whose plist and loaded program are already right.
func TestLaunchdAgentAlreadyRightIsLeftAlone(t *testing.T) {
	m, cmds, _ := realMachine(t, "darwin")
	writeSchedulerFiles(t, m)
	cmds.loaded = []string{exampleExe, "backup", "run-scheduled"}
	if err := m.InstallScheduler(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(cmds.ran) != 0 {
		t.Errorf("ran %q, want nothing", cmds.ran)
	}

	target := m.launchdDomain() + "/" + launchdLabel
	for name, loaded := range map[string][]string{
		"not loaded":                nil,
		"loaded with another ctdev": {"/opt/example/old/ctdev", "backup", "run-scheduled"},
	} {
		m, cmds, _ := realMachine(t, "darwin")
		writeSchedulerFiles(t, m)
		cmds.loaded = loaded
		if err := m.InstallScheduler(context.Background()); err != nil {
			t.Fatal(err)
		}
		reload := "launchctl bootout " + target + ",launchctl bootstrap " + m.launchdDomain() + " " + m.launchdPath()
		if got := strings.Join(cmds.ran, ","); got != reload {
			t.Errorf("%s: ran %q\nwant %q", name, got, reload)
		}
	}
}

// ctdev moved while a scheduled run is going: Configure changes nothing and
// says to come back, rather than kill the run; Configure again after the
// run writes the new plist and reloads the agent.
func TestCtdevMovedDuringARunThenConfigureAgain(t *testing.T) {
	m, cmds, out := realMachine(t, "darwin")
	m.Exe = "/opt/example/old/ctdev"
	writeSchedulerFiles(t, m)
	oldPlist := m.launchdPlist()
	cmds.loaded = []string{m.Exe, "backup", "run-scheduled"}
	m.Exe = "/opt/example/new/ctdev"

	// Another process's scheduled run holds the run lock.
	other := &Machine{Dir: m.Dir}
	unlock, err := other.runLock()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.InstallScheduler(context.Background()); err != nil {
		t.Fatal(err)
	}
	unlock()
	if len(cmds.ran) != 0 {
		t.Errorf("ran %q during a scheduled run, want nothing", cmds.ran)
	}
	if got, _ := os.ReadFile(m.launchdPath()); string(got) != oldPlist {
		t.Error("the plist was changed during a scheduled run")
	}
	if want := "A scheduled backup is running; choose Configure again after it finishes to update the schedule.\n"; out.String() != want {
		t.Errorf("said %q, want %q", out, want)
	}

	if err := m.InstallScheduler(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(m.launchdPath()); !strings.Contains(string(got), "/opt/example/new/ctdev") {
		t.Error("the new plist wasn't written after the run")
	}
	if got := strings.Join(cmds.ran, ","); !strings.Contains(got, "launchctl bootout") || !strings.Contains(got, "launchctl bootstrap") {
		t.Errorf("ran %q, want a reload", got)
	}
}

// systemd expands % specifiers and $ variables in ExecStart, so a path
// holding either is doubled.
func TestExecStartEscapesPercentAndDollar(t *testing.T) {
	m, _ := schedulerMachine(t, "linux")
	m.Exe = "/opt/100%/$HOME/ctdev"
	if service, _ := m.systemdUnits(); !strings.Contains(service, `ExecStart="/opt/100%%/$$HOME/ctdev" backup run-scheduled`) {
		t.Errorf("service:\n%s", service)
	}
}

func TestInstallSchedulerNeedsTheBinaryPath(t *testing.T) {
	m, _ := schedulerMachine(t, "linux")
	m.Exe = ""
	if err := m.InstallScheduler(context.Background()); err == nil {
		t.Error("InstallScheduler with no ctdev path succeeded")
	}
	m.Exe = "ctdev"
	if err := m.InstallScheduler(context.Background()); err == nil {
		t.Error("InstallScheduler with a relative ctdev path succeeded: the timer has no PATH to find it on")
	}
}

// Stop backing up removes the scheduler only once no folder is left on the
// list.
func TestRemoveSchedulerIfIdle(t *testing.T) {
	m, out := schedulerMachine(t, "linux")
	ctx := context.Background()
	if err := m.addFolder("/example/a", "00000000-0000-4000-8000-000000000000", Daily); err != nil {
		t.Fatal(err)
	}
	removed, err := m.RemoveSchedulerIfIdle(ctx)
	if err != nil || removed || out.Len() != 0 {
		t.Errorf("with a folder listed: removed %v, err %v, output %q", removed, err, out)
	}
	if err := os.Remove(m.foldersFile()); err != nil {
		t.Fatal(err)
	}
	removed, err = m.RemoveSchedulerIfIdle(ctx)
	if err != nil || !removed || !strings.Contains(out.String(), "disable --now ctdev-backup.timer") {
		t.Errorf("with none listed: removed %v, err %v, output %q", removed, err, out)
	}
}
