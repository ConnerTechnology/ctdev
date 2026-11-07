package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeNotifier records the notifications it's asked to send.
type fakeNotifier struct{ sent []string }

func (n *fakeNotifier) Notify(_ context.Context, title, body string) error {
	n.sent = append(n.sent, title+": "+body)
	return nil
}

// A real restic backs up the due folder and leaves the one that isn't.
func TestRunScheduledBacksUpOnlyTheDueFolder(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	n := &fakeNotifier{}
	m.Notifier = n
	d, _ := createExample(t, m)

	due := exampleFolder(t, m)
	notDue := filepath.Join(m.Home, "Documents", "notes")
	if err := os.MkdirAll(notDue, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notDue, "todo.txt"), []byte("example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dueCfg, notDueCfg := NewConfig(d.Name), NewConfig(d.Name)
	at(m, noon.Add(-48*time.Hour))
	for root, cfg := range map[string]Config{due: dueCfg, notDue: notDueCfg} {
		if err := m.Configure(ctx, d, root, cfg); err != nil {
			t.Fatal(err)
		}
	}
	// notDue last succeeded an hour ago, on a daily schedule.
	lastHour := noon.Add(-time.Hour)
	if err := m.recordRun(notDue, lastHour, nil); err != nil {
		t.Fatal(err)
	}

	at(m, noon)
	results, err := m.RunScheduled(ctx)
	if err != nil {
		t.Fatal(err)
	}
	outcome := map[string]Outcome{}
	for _, r := range results {
		outcome[r.Path] = r.Outcome
	}
	if outcome[due] != BackedUp || outcome[notDue] != NotDue {
		t.Errorf("outcomes = %v, want %s backed up and %s not due", outcome, due, notDue)
	}

	for root, want := range map[string]int{due: 1, notDue: 0} {
		cfg, err := ReadConfig(root)
		if err != nil {
			t.Fatal(err)
		}
		snaps, err := d.Snapshots(ctx, m.Ref(root, &cfg))
		if err != nil {
			t.Fatal(err)
		}
		if len(snaps) != want {
			t.Errorf("%s has %d backups, want %d", root, len(snaps), want)
		}
	}

	runs, err := m.runs()
	if err != nil {
		t.Fatal(err)
	}
	if r := runs[due]; !r.LastAttempt.Equal(noon) || !r.LastSuccess.Equal(noon) || r.Failed() {
		t.Errorf("due folder's state = %+v, want a success at %v", r, noon)
	}
	if r := runs[notDue]; !r.LastSuccess.Equal(lastHour) || !r.LastAttempt.Equal(lastHour) {
		t.Errorf("not-due folder's state changed: %+v", r)
	}
	if len(n.sent) != 0 {
		t.Errorf("notifications on success: %v", n.sent)
	}
}

// listed puts root on m's schedule list with a config file holding id.
func listed(t *testing.T, m *Machine, root string, cfg Config) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, root, cfg)
	if err := m.addFolder(root, cfg.ID, cfg.Schedule); err != nil {
		t.Fatal(err)
	}
}

// A folder that is gone, has lost its config file, or holds another folder's
// ID is skipped and recorded as missing, without touching restic.
func TestRunScheduledRecordsMissingFolders(t *testing.T) {
	m := testMachine(t, "example-laptop")
	m.Restic = filepath.Join(t.TempDir(), "no-restic-here")
	m.Notifier = &fakeNotifier{}
	at(m, noon)
	cfg := NewConfig("example-backups")

	gone := filepath.Join(m.Home, "gone")
	listed(t, m, gone, cfg)
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	noConfig := filepath.Join(m.Home, "no-config")
	listed(t, m, noConfig, NewConfig("example-backups"))
	if err := os.Remove(filepath.Join(noConfig, ConfigFileName)); err != nil {
		t.Fatal(err)
	}
	otherID := filepath.Join(m.Home, "other-id")
	listed(t, m, otherID, NewConfig("example-backups"))
	writeConfig(t, otherID, NewConfig("example-backups"))

	results, err := m.RunScheduled(context.Background())
	if err == nil {
		t.Error("RunScheduled with missing folders returned no error")
	}
	for _, r := range results {
		if r.Outcome != Missing {
			t.Errorf("%s: %s, want missing", r.Path, r.Outcome)
		}
	}
	h := health(t, m)
	if len(h) != 3 {
		t.Fatalf("health lists %d folders, want 3", len(h))
	}
	for _, f := range h {
		if f.State != HealthMissing || !f.Run.LastAttempt.Equal(noon) || f.Run.Error == "" {
			t.Errorf("%s: %+v, want missing at %v with a reason", f.Path, f, noon)
		}
	}
}

// A folder that comes back is no longer missing, even before it's due.
func TestAFolderThatComesBackIsNoLongerMissing(t *testing.T) {
	m := testMachine(t, "example-laptop")
	m.Notifier = &fakeNotifier{}
	at(m, noon)
	root := filepath.Join(m.Home, "example")
	listed(t, m, root, NewConfig("example-backups"))
	if err := m.recordRun(root, noon.Add(-time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	if err := m.recordMissing(root, noon.Add(-time.Minute), "example: gone"); err != nil {
		t.Fatal(err)
	}
	results, err := m.RunScheduled(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Outcome != NotDue {
		t.Errorf("results = %+v, want not due", results)
	}
	if h := health(t, m); h[0].State != HealthOK {
		t.Errorf("health = %q, want ok", h[0].State)
	}
}

// failingMachine has one listed folder whose destination isn't set up, so
// every run of it fails without restic.
func failingMachine(t *testing.T) (*Machine, string, *fakeNotifier) {
	t.Helper()
	m := testMachine(t, "example-laptop")
	n := &fakeNotifier{}
	m.Notifier = n
	at(m, noon)
	root := filepath.Join(m.Home, "Projects", "website")
	listed(t, m, root, NewConfig("example-backups"))
	return m, root, n
}

func TestNotifiesOnceWhenAFolderStartsFailingAndOnceWhenItRecovers(t *testing.T) {
	m, root, n := failingMachine(t)
	ctx := context.Background()
	if err := m.recordRun(root, noon.Add(-25*time.Hour), nil); err != nil {
		t.Fatal(err)
	}

	// OK -> failing: one notification.
	if _, err := m.RunScheduled(ctx); err == nil {
		t.Fatal("a run with no destination succeeded")
	}
	if len(n.sent) != 1 || !strings.Contains(n.sent[0], "failing") || !strings.Contains(n.sent[0], "~/Projects/website") {
		t.Fatalf("after the first failure: %q, want one failing notification naming the folder", n.sent)
	}

	// Still failing, hour after hour: nothing more.
	for i := 1; i <= 3; i++ {
		at(m, noon.Add(time.Duration(i)*time.Hour))
		if _, err := m.RunScheduled(ctx); err == nil {
			t.Fatal("a run with no destination succeeded")
		}
	}
	if len(n.sent) != 1 {
		t.Fatalf("repeated failures notified again: %q", n.sent)
	}

	// failing -> OK: one notification. A success recorded by Back up now
	// counts as well as one by the runner.
	at(m, noon.Add(4*time.Hour))
	if err := m.recordRun(root, noon.Add(4*time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RunScheduled(ctx); err != nil {
		t.Fatal(err)
	}
	if len(n.sent) != 2 || !strings.Contains(n.sent[1], "backing up again") {
		t.Fatalf("after recovering: %q, want a second, recovered, notification", n.sent)
	}
	at(m, noon.Add(5*time.Hour))
	if _, err := m.RunScheduled(ctx); err != nil {
		t.Fatal(err)
	}
	if len(n.sent) != 2 {
		t.Errorf("staying OK notified again: %q", n.sent)
	}
}

// A folder that goes missing notifies the same way as one that fails.
func TestNotifiesWhenAFolderGoesMissing(t *testing.T) {
	m, root, n := failingMachine(t)
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	_, _ = m.RunScheduled(context.Background())
	if len(n.sent) != 1 || !strings.Contains(n.sent[0], "failing") {
		t.Errorf("notifications = %q, want one failing", n.sent)
	}
}

// The desktop notifier sends nothing where there's no desktop or no tool.
func TestDesktopNotifierCommands(t *testing.T) {
	for name, tc := range map[string]struct {
		goos    string
		env     map[string]string
		tools   []string
		want    string // the dry-run line, or "" for nothing sent
		wantErr bool
	}{
		"linux desktop": {
			goos: "linux", env: map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:path=/run/user/1000/bus"},
			tools: []string{"notify-send"},
			want:  "[dry-run] notify-send --app-name ctdev Backup failing ~/example: example error\n",
		},
		"linux, no session bus": {
			goos: "linux", tools: []string{"notify-send"},
		},
		"linux, no notify-send": {
			goos: "linux", env: map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:path=/run/user/1000/bus"},
		},
		"macos": {
			goos: "darwin", tools: []string{"osascript"},
			want: "[dry-run] osascript -e on run argv -e display notification (item 2 of argv) with title (item 1 of argv) -e end run Backup failing ~/example: example error\n",
		},
		"macos, no osascript": {goos: "darwin"},
		"windows":             {goos: "windows", tools: []string{"notify-send", "osascript"}},
	} {
		out := &bytes.Buffer{}
		dn := desktopNotifier{
			goos: tc.goos,
			env:  func(k string) string { return tc.env[k] },
			lookPath: func(name string) (string, error) {
				for _, tool := range tc.tools {
					if tool == name {
						return name, nil
					}
				}
				return "", errors.New("not found")
			},
		}
		dn.opts.Stdout, dn.opts.DryRun = out, true
		err := dn.Notify(context.Background(), "Backup failing", "~/example: example error")
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v", name, err)
		}
		if out.String() != tc.want {
			t.Errorf("%s: sent %q, want %q", name, out.String(), tc.want)
		}
	}
}
