package setup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// fakeMouseSystem stands in for the shell-outs behind the mouse settings.
// Root writes and `rm -f` land on the real (temp) filesystem so tests can
// check exactly what is left behind; everything else is only recorded.
type fakeMouseSystem struct {
	calls  [][]string
	pkgs   map[string]bool
	starts int // times xbindkeys was started
}

func (f *fakeMouseSystem) packages(_ context.Context, o sysutil.Opts, pkgs []string) error {
	f.calls = append(f.calls, append([]string{"apt-get-install"}, pkgs...))
	if !o.DryRun {
		for _, p := range pkgs {
			f.pkgs[p] = true
		}
	}
	return nil
}

func (f *fakeMouseSystem) run(_ context.Context, o sysutil.Opts, name string, args ...string) error {
	f.calls = append(f.calls, append([]string{name}, args...))
	return nil
}

func (f *fakeMouseSystem) sudoRun(_ context.Context, o sysutil.Opts, name string, args ...string) error {
	f.calls = append(f.calls, append([]string{"sudo", name}, args...))
	if o.DryRun {
		return nil
	}
	switch {
	case name == "rm" && len(args) == 2 && args[0] == "-f":
		if err := os.Remove(args[1]); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (f *fakeMouseSystem) sudoWrite(_ context.Context, o sysutil.Opts, content, path string) error {
	f.calls = append(f.calls, []string{"sudo-write", path})
	if o.DryRun {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func (f *fakeMouseSystem) called(want ...string) bool {
	for _, c := range f.calls {
		if reflect.DeepEqual(c, want) {
			return true
		}
	}
	return false
}

// useFakeMouseSystem points HOME and the udev rule at a temp dir and swaps
// the mouse seams for a fake. Returns the fake and the temp home.
func useFakeMouseSystem(t *testing.T) (*fakeMouseSystem, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	udevDir := filepath.Join(t.TempDir(), "rules.d")
	if err := os.MkdirAll(udevDir, 0o755); err != nil {
		t.Fatal(err)
	}

	f := &fakeMouseSystem{pkgs: map[string]bool{}}
	run, sudoRun, sudoWrite, pkgs, installed, start, rule :=
		mouseRun, mouseSudoRun, mouseSudoWrite, mousePackages, mousePkgInstalled, mouseStartXbind, logitechUdevRule
	t.Cleanup(func() {
		mouseRun, mouseSudoRun, mouseSudoWrite, mousePackages, mousePkgInstalled, mouseStartXbind, logitechUdevRule =
			run, sudoRun, sudoWrite, pkgs, installed, start, rule
	})
	mouseRun = f.run
	mouseSudoRun = f.sudoRun
	mouseSudoWrite = f.sudoWrite
	mousePackages = f.packages
	mousePkgInstalled = func(_ context.Context, p string) bool { return f.pkgs[p] }
	mouseStartXbind = func() error { f.starts++; return nil }
	logitechUdevRule = filepath.Join(udevDir, "99-logitech-kvm-fix.rules")
	return f, home
}

func mouseSetting(t *testing.T, name string) *Setting {
	t.Helper()
	for i := range Registry {
		if Registry[i].Name == name {
			return &Registry[i]
		}
	}
	t.Fatalf("setting %q not found in Registry", name)
	return nil
}

// listFiles returns every regular file under dir, relative to it.
func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

const (
	middleClickName = "Middle-click opens the window overview"
	logitechName    = "Keep the Logitech mouse working after a KVM switch"
)

func TestMouseCategoryHoldsBothInstallSettings(t *testing.T) {
	mouse := map[string]Setting{}
	for _, s := range FilterBySlug(Registry, "mouse") {
		mouse[s.Name] = s
	}
	for _, name := range []string{middleClickName, logitechName} {
		s, ok := mouse[name]
		if !ok {
			t.Errorf("configure mouse has no %q", name)
			continue
		}
		if s.OneWay {
			t.Errorf("%q is OneWay; it needs an off switch", name)
		}
		if s.Control != ControlToggle || s.Default != "installed" {
			t.Errorf("%q: want an installed/not installed toggle, got control %v default %q", name, s.Control, s.Default)
		}
		if s.DetectFunc == nil || s.ApplyFunc == nil || s.HardwareFn == nil {
			t.Errorf("%q: needs DetectFunc, ApplyFunc and HardwareFn", name)
		}
	}
	for _, s := range FilterBySlug(Registry, "system") {
		if strings.Contains(strings.ToLower(s.Name), "logitech") {
			t.Errorf("configure system still has %q", s.Name)
		}
	}
	for _, s := range Registry {
		if s.Name == "Mouse bindings (xbindkeys)" || s.Name == "Logitech KVM mouse fix" {
			t.Errorf("old name %q is still in Registry", s.Name)
		}
	}
}

func TestMouseSettingsHaveOneLineWhy(t *testing.T) {
	for _, s := range FilterBySlug(Registry, "mouse") {
		if s.Description == "" || strings.Contains(s.Description, "\n") || len(s.Description) > 110 {
			t.Errorf("%q: want a one-line why, got %q", s.Name, s.Description)
		}
	}
}

func TestMouseSpeedDefault(t *testing.T) {
	s := mouseSetting(t, "Mouse speed")
	if s.Default != "0.46" {
		t.Errorf("Mouse speed default = %q, want 0.46", s.Default)
	}
	// The recommended value has to be one the slider can land on.
	if got := AdjustSlider(s, AdjustSlider(s, s.Default, 1), -1); got != s.Default {
		t.Errorf("slider can't step back to the default: got %q", got)
	}
}

func TestMiddleClickOffRemovesWhatOnInstalled(t *testing.T) {
	f, home := useFakeMouseSystem(t)
	s := mouseSetting(t, middleClickName)
	ctx := context.Background()
	o := sysutil.Opts{Stdout: &bytes.Buffer{}}

	// Something of the user's that ctdev must not touch.
	other := filepath.Join(home, ".config", "autostart", "other.desktop")
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := listFiles(t, home)

	if got := s.DetectFunc(ctx); got != "not installed" {
		t.Fatalf("before on: detect = %q", got)
	}
	if err := s.ApplyFunc(ctx, o, "installed"); err != nil {
		t.Fatal(err)
	}
	if got := s.DetectFunc(ctx); got != "installed" {
		t.Fatalf("after on: detect = %q", got)
	}
	for _, p := range []string{".xbindkeysrc", ".config/autostart/xbindkeys.desktop"} {
		if _, err := os.Stat(filepath.Join(home, p)); err != nil {
			t.Errorf("on didn't install %s: %v", p, err)
		}
	}

	if err := s.ApplyFunc(ctx, o, "not installed"); err != nil {
		t.Fatal(err)
	}
	if got := s.DetectFunc(ctx); got != "not installed" {
		t.Errorf("after off: detect = %q", got)
	}
	if after := listFiles(t, home); !reflect.DeepEqual(after, before) {
		t.Errorf("off left %v, want %v", after, before)
	}
	if !f.called("killall", "xbindkeys") {
		t.Error("off didn't stop xbindkeys")
	}
}

func TestMiddleClickOffKeepsAnEditedConfig(t *testing.T) {
	_, home := useFakeMouseSystem(t)
	s := mouseSetting(t, middleClickName)
	ctx := context.Background()
	out := &bytes.Buffer{}
	o := sysutil.Opts{Stdout: out}

	if err := s.ApplyFunc(ctx, o, "installed"); err != nil {
		t.Fatal(err)
	}
	rc := filepath.Join(home, ".xbindkeysrc")
	if err := os.WriteFile(rc, []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyFunc(ctx, o, "not installed"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rc); err != nil {
		t.Errorf("off removed the user's edited .xbindkeysrc: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "autostart", "xbindkeys.desktop")); !os.IsNotExist(err) {
		t.Errorf("off left ctdev's autostart entry: %v", err)
	}
	if !strings.Contains(out.String(), "Leaving "+rc) {
		t.Errorf("off didn't say it left %s; output:\n%s", rc, out)
	}
	if got := s.DetectFunc(ctx); got != "not installed" {
		t.Errorf("after off: detect = %q", got)
	}
}

func TestLogitechKVMFixOffRemovesWhatOnInstalled(t *testing.T) {
	f, home := useFakeMouseSystem(t)
	s := mouseSetting(t, logitechName)
	ctx := context.Background()
	o := sysutil.Opts{Stdout: &bytes.Buffer{}}
	homeBefore := listFiles(t, home)
	rulesDir := filepath.Dir(logitechUdevRule)

	if got := s.DetectFunc(ctx); got != "not installed" {
		t.Fatalf("before on: detect = %q", got)
	}
	if err := s.ApplyFunc(ctx, o, "installed"); err != nil {
		t.Fatal(err)
	}
	if got := s.DetectFunc(ctx); got != "installed" {
		t.Fatalf("after on: detect = %q", got)
	}
	if !f.called("systemctl", "--user", "enable", "solaar-restart.service") {
		t.Error("on didn't enable the user service")
	}
	if !logitechFixPresent() {
		t.Error("logitechFixPresent = false after on")
	}

	f.calls = nil
	if err := s.ApplyFunc(ctx, o, "not installed"); err != nil {
		t.Fatal(err)
	}
	if got := s.DetectFunc(ctx); got != "not installed" {
		t.Errorf("after off: detect = %q", got)
	}
	if after := listFiles(t, home); !reflect.DeepEqual(after, homeBefore) {
		t.Errorf("off left %v in home, want %v", after, homeBefore)
	}
	if left := listFiles(t, rulesDir); len(left) != 0 {
		t.Errorf("off left %v in rules.d", left)
	}
	for _, want := range [][]string{
		{"systemctl", "--user", "disable", "solaar-restart.service"},
		{"systemctl", "--user", "daemon-reload"},
		{"sudo", "rm", "-f", logitechUdevRule},
		{"sudo", "udevadm", "control", "--reload-rules"},
	} {
		if !f.called(want...) {
			t.Errorf("off didn't run %v; ran %v", want, f.calls)
		}
	}
	if logitechFixPresent() {
		t.Error("logitechFixPresent = true after off")
	}
}

func TestMouseOffWhenNothingInstalledIsANoOp(t *testing.T) {
	f, _ := useFakeMouseSystem(t)
	ctx := context.Background()
	o := sysutil.Opts{Stdout: &bytes.Buffer{}}
	if err := setLogitechKVMFix(ctx, o, "not installed"); err != nil {
		t.Fatal(err)
	}
	if err := setXbindkeys(ctx, o, "not installed"); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if c[0] == "sudo" {
			t.Errorf("off with nothing installed ran %v", c)
		}
	}
}

func TestMouseOffDryRunTouchesNothing(t *testing.T) {
	_, home := useFakeMouseSystem(t)
	ctx := context.Background()
	if err := applyLogitechKVMFix(ctx, sysutil.Opts{Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	if err := applyXbindkeys(ctx, sysutil.Opts{Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	before := listFiles(t, home)

	out := &bytes.Buffer{}
	o := sysutil.Opts{Stdout: out, DryRun: true}
	if err := removeLogitechKVMFix(ctx, o); err != nil {
		t.Fatal(err)
	}
	if err := removeXbindkeys(ctx, o); err != nil {
		t.Fatal(err)
	}
	if after := listFiles(t, home); !reflect.DeepEqual(after, before) {
		t.Errorf("dry-run off changed home: %v → %v", before, after)
	}
	if _, err := os.Stat(logitechUdevRule); err != nil {
		t.Errorf("dry-run off removed the udev rule: %v", err)
	}
	if !strings.Contains(out.String(), "[dry-run] remove") {
		t.Errorf("dry-run off didn't say what it would remove:\n%s", out)
	}
}

func TestRoundMouseSpeed(t *testing.T) {
	for raw, want := range map[string]string{
		"0.46218487394957974": "0.46",
		"0.65":                "0.65",
		"0":                   "0",
		"-0.25":               "-0.25",
		"":                    "",
		"unset":               "unset",
	} {
		if got := roundMouseSpeed(raw); got != want {
			t.Errorf("roundMouseSpeed(%q) = %q, want %q", raw, got, want)
		}
	}
}

// applyThroughStates drives one setting the way the wizard and settings
// browser do: detect, queue desired, and apply only if NeedsApply says so.
func applyThroughStates(t *testing.T, name, desired string) {
	t.Helper()
	ctx := context.Background()
	states := InitStates(ctx, []Setting{*mouseSetting(t, name)})
	st := &states[0]
	st.DesiredValue = desired
	if !st.NeedsApply(false) {
		t.Fatalf("%q: current %q, desired %q: wizard wouldn't apply", name, st.CurrentValue, desired)
	}
	if err := st.Setting.ApplyFunc(ctx, sysutil.Opts{Stdout: &bytes.Buffer{}}, st.DesiredValue); err != nil {
		t.Fatal(err)
	}
}

func TestMouseOffReachableFromPartialInstall(t *testing.T) {
	cases := []struct {
		name    string
		setting string
		partial func(t *testing.T, home string) // starts from a full install
	}{
		{"edited xbindkeysrc", middleClickName, func(t *testing.T, home string) {
			if err := os.WriteFile(filepath.Join(home, ".xbindkeysrc"), []byte("# mine\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"only the udev rule", logitechName, func(t *testing.T, home string) {
			if err := os.Remove(filepath.Join(home, ".config", "systemd", "user", solaarService)); err != nil {
				t.Fatal(err)
			}
		}},
		{"only the user service", logitechName, func(t *testing.T, _ string) {
			if err := os.Remove(logitechUdevRule); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, home := useFakeMouseSystem(t)
			s := mouseSetting(t, c.setting)
			ctx := context.Background()
			if err := s.ApplyFunc(ctx, sysutil.Opts{Stdout: &bytes.Buffer{}}, "installed"); err != nil {
				t.Fatal(err)
			}
			c.partial(t, home)
			if got := s.DetectFunc(ctx); got != "partly installed" {
				t.Fatalf("detect = %q, want partly installed", got)
			}

			applyThroughStates(t, c.setting, "not installed")
			if got := s.DetectFunc(ctx); got != "not installed" {
				t.Errorf("after off: detect = %q", got)
			}
			if c.setting == logitechName && logitechFixPresent() {
				t.Error("off left a piece of the Logitech fix")
			}
			autostart := filepath.Join(home, ".config", "autostart", "xbindkeys.desktop")
			if _, err := os.Stat(autostart); c.setting == middleClickName && !os.IsNotExist(err) {
				t.Errorf("off left ctdev's autostart entry: %v", err)
			}
		})
	}
}

func TestMouseOnRepairsPartialInstall(t *testing.T) {
	_, _ = useFakeMouseSystem(t)
	ctx := context.Background()
	if err := applyLogitechKVMFix(ctx, sysutil.Opts{Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(logitechUdevRule); err != nil {
		t.Fatal(err)
	}
	applyThroughStates(t, logitechName, "installed")
	if got := detectLogitechKVMFix(ctx); got != "installed" {
		t.Errorf("after on: detect = %q", got)
	}
}

func TestMiddleClickOffGivesBackTheUsersOwnConfig(t *testing.T) {
	f, home := useFakeMouseSystem(t)
	s := mouseSetting(t, middleClickName)
	ctx := context.Background()
	o := sysutil.Opts{Stdout: &bytes.Buffer{}}

	rc := filepath.Join(home, ".xbindkeysrc")
	mine := "# my own bindings\n"
	if err := os.WriteFile(rc, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	before := listFiles(t, home)

	if err := s.ApplyFunc(ctx, o, "installed"); err != nil {
		t.Fatal(err)
	}
	if !isCtdevFile(rc, xbindkeysrcSrc) {
		t.Fatal("on didn't deploy ctdev's .xbindkeysrc")
	}
	f.calls, f.starts = nil, 0

	if err := s.ApplyFunc(ctx, o, "not installed"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(rc)
	if err != nil || string(got) != mine {
		t.Errorf(".xbindkeysrc after off = %q, %v; want the user's %q", got, err, mine)
	}
	if after := listFiles(t, home); !reflect.DeepEqual(after, before) {
		t.Errorf("off left %v, want %v", after, before)
	}
	if !f.called("killall", "xbindkeys") || f.starts != 1 {
		t.Errorf("off should restart xbindkeys on the user's config: calls %v, starts %d", f.calls, f.starts)
	}
}

func TestMiddleClickOffLeavesXbindkeysRunningWhenConfigIsTheUsers(t *testing.T) {
	f, home := useFakeMouseSystem(t)
	s := mouseSetting(t, middleClickName)
	ctx := context.Background()
	o := sysutil.Opts{Stdout: &bytes.Buffer{}}
	if err := s.ApplyFunc(ctx, o, "installed"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".xbindkeysrc"), []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.calls, f.starts = nil, 0
	if err := s.ApplyFunc(ctx, o, "not installed"); err != nil {
		t.Fatal(err)
	}
	if f.called("killall", "xbindkeys") || f.starts != 0 {
		t.Errorf("off touched xbindkeys running on the user's config: calls %v, starts %d", f.calls, f.starts)
	}
}

func TestMiddleClickInstallsPackagesThroughApplyPackages(t *testing.T) {
	f, _ := useFakeMouseSystem(t)
	if err := applyXbindkeys(context.Background(), sysutil.Opts{Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	if !f.called("apt-get-install", "xbindkeys", "xdotool") {
		t.Errorf("on didn't install the packages; ran %v", f.calls)
	}
}

// seedBackup writes content as a DeployFile-style backup of path.
func seedBackup(t *testing.T, path, stamp, content string) string {
	t.Helper()
	bak := path + "." + stamp + ".bak"
	if err := os.WriteFile(bak, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return bak
}

func TestMiddleClickOffDoesNotRestoreAFormerCtdevConfig(t *testing.T) {
	f, home := useFakeMouseSystem(t)
	ctx := context.Background()
	o := sysutil.Opts{Stdout: &bytes.Buffer{}}
	old, err := os.ReadFile("testdata/xbindkeysrc-before-59ad0786")
	if err != nil {
		t.Fatal(err)
	}
	rc := filepath.Join(home, ".xbindkeysrc")
	if err := applyXbindkeys(ctx, o); err != nil {
		t.Fatal(err)
	}
	// The machine first applied an older ctdev, so its newest backup is that copy.
	seedBackup(t, rc, "2026-01-02T03-04-05", string(old))
	f.calls, f.starts = nil, 0

	if err := removeXbindkeys(ctx, o); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(rc); !os.IsNotExist(err) {
		t.Errorf("off left a .xbindkeysrc: %v", err)
	}
	if left := listFiles(t, home); len(left) != 0 {
		t.Errorf("off left %v; a former ctdev backup should go too", left)
	}
	if f.starts != 0 {
		t.Errorf("off started xbindkeys %d times with no user config", f.starts)
	}
}

func TestMiddleClickOffRestoresTheUsersFileBehindAFormerCtdevBackup(t *testing.T) {
	_, home := useFakeMouseSystem(t)
	ctx := context.Background()
	o := sysutil.Opts{Stdout: &bytes.Buffer{}}
	old, err := os.ReadFile("testdata/xbindkeysrc-before-59ad0786")
	if err != nil {
		t.Fatal(err)
	}
	rc := filepath.Join(home, ".xbindkeysrc")
	if err := applyXbindkeys(ctx, o); err != nil {
		t.Fatal(err)
	}
	seedBackup(t, rc, "2025-06-01T00-00-00", "# mine\n")
	seedBackup(t, rc, "2026-01-02T03-04-05", string(old))
	if err := removeXbindkeys(ctx, o); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(rc); err != nil || string(got) != "# mine\n" {
		t.Errorf(".xbindkeysrc = %q, %v; want the user's file", got, err)
	}
}

func TestMiddleClickOffRemovesAFormerCtdevConfig(t *testing.T) {
	_, home := useFakeMouseSystem(t)
	old, err := os.ReadFile("testdata/xbindkeysrc-before-59ad0786")
	if err != nil {
		t.Fatal(err)
	}
	rc := filepath.Join(home, ".xbindkeysrc")
	if err := os.WriteFile(rc, old, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeXbindkeys(context.Background(), sysutil.Opts{Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(rc); !os.IsNotExist(err) {
		t.Errorf("off left an older ctdev .xbindkeysrc: %v", err)
	}
}

func TestNewestBackupMatchesOnlyBackupPathNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".xbindkeysrc")
	for _, name := range []string{
		".xbindkeysrc.old.bak",
		".xbindkeysrc.zzz.bak",
		".xbindkeysrc.2026-01-02T03-04-05.bak.orig",
		".xbindkeysrc.2025-06-01T00-00-00.bak",
		".xbindkeysrc.2026-01-02T03-04-05.bak",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := backupsNewestFirst(path)
	want := []string{
		path + ".2026-01-02T03-04-05.bak",
		path + ".2025-06-01T00-00-00.bak",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("backupsNewestFirst = %v, want %v", got, want)
	}
	// BackupPath's own format must match the pattern.
	if !backupName(path).MatchString(sysutil.BackupPath(path)) {
		t.Errorf("pattern doesn't match sysutil.BackupPath(%q) = %q", path, sysutil.BackupPath(path))
	}
}

func TestMiddleClickOffRemovesADanglingRCSymlink(t *testing.T) {
	_, home := useFakeMouseSystem(t)
	rc := filepath.Join(home, ".xbindkeysrc")
	if err := os.Symlink(filepath.Join(home, "gone", ".xbindkeysrc"), rc); err != nil {
		t.Fatal(err)
	}
	if err := removeXbindkeys(context.Background(), sysutil.Opts{Stdout: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(rc); !os.IsNotExist(err) {
		t.Errorf("off left the dangling symlink: %v", err)
	}
}

func TestMiddleClickFormerCtdevConfigAloneIsPartlyInstalled(t *testing.T) {
	_, home := useFakeMouseSystem(t)
	old, err := os.ReadFile("testdata/xbindkeysrc-before-59ad0786")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".xbindkeysrc"), old, 0o644); err != nil {
		t.Fatal(err)
	}
	applyThroughStates(t, middleClickName, "not installed")
	if left := listFiles(t, home); len(left) != 0 {
		t.Errorf("off left %v", left)
	}
}
