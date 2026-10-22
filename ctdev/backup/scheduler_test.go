package backup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const exampleExe = "/opt/example tools/bin/ctdev"

// schedulerMachine is a dry-run machine on goos whose ctdev is exampleExe.
func schedulerMachine(t *testing.T, goos string) (*Machine, *bytes.Buffer) {
	t.Helper()
	m := testMachine(t, "example-laptop")
	out := &bytes.Buffer{}
	m.Opts.Stdout, m.Opts.DryRun = out, true
	m.GOOS, m.Exe = goos, exampleExe
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
		"<key>StartInterval</key>\n\t<integer>3600</integer>",
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

// The scheduler counts as installed when its files are there; a dry run
// installs nothing.
func TestSchedulerInstalledFollowsItsFiles(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		m, _ := schedulerMachine(t, goos)
		if m.SchedulerInstalled() {
			t.Errorf("%s: installed before anything was written", goos)
		}
		for _, f := range m.schedulerFiles() {
			if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if !m.SchedulerInstalled() {
			t.Errorf("%s: not installed with its files in place", goos)
		}
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
