//go:build unix

package backupapp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

const masterPassword = "correct horse example staple"

// resticKeyring replaces the folder's stand-in restic with keyStandinScript,
// rooted under the folder's temporary HOME.
func resticKeyring(t *testing.T, d Deps) {
	t.Helper()
	t.Setenv("STANDIN_ROOT", d.Machine.Home)
	if err := os.WriteFile(filepath.Join(d.Machine.Home, ".local", "bin", "restic"), []byte(keyStandinScript), 0o755); err != nil {
		t.Fatal(err)
	}
}

// another is a second computer in the same home, sharing the stand-in restic.
func another(t *testing.T, d Deps, host string) Deps {
	t.Helper()
	m := &backup.Machine{Dir: filepath.Join(t.TempDir(), "backup"), Home: d.Machine.Home, Hostname: host, Username: "example",
		Restic: filepath.Join(d.Machine.Home, ".local", "bin", "restic"), Opts: sysutil.Opts{Stdout: &bytes.Buffer{}}}
	return Deps{Machine: m, Cwd: d.Cwd, Scheduler: &fakeScheduler{}}
}

func localPlace(name, path string) Values {
	return Values{"name": name, "store": "local", "local.path": path}
}

func newPassword(pw string) Values { return Values{"master": pw, "again": pw} }

// Configure on a new folder creates a destination, refusing a place inside
// the folder, shows its kit and Recovery Code once, and saves the folder's
// config with the scheduler installed. From a subfolder it then reconfigures
// the folder above, starting from its current values.
func TestConfigureCreatesADestinationAndConfiguresTheFolder(t *testing.T) {
	d, _ := standinFolder(t, false, backs, forgets)
	resticKeyring(t, d)
	sched := &fakeScheduler{}
	d.Scheduler = sched
	m := d.Machine
	repo := filepath.Join(t.TempDir(), "example-backups")
	ctx := context.Background()

	ui := &fakeUI{t: t,
		Picks: []any{0, 0}, // Create a new destination; Hourly
		Forms: []any{
			localPlace("example-backups", "~/Projects/website/.backups"), // inside the folder: refused
			localPlace("example-backups", repo),
			newPassword(masterPassword),
			Values{"hourly": "24", "daily": "7", "weekly": "4", "monthly": "12", "yearly": "3"},
		},
		Kits: []error{nil},
	}
	if err := configureFolder(ctx, ui, d, m.Status(ctx, d.Cwd)); err != nil {
		t.Fatalf("configureFolder: %v (said %+v)", err, ui.Said)
	}
	forms := requestsOf[Form](ui)
	if len(forms) != 4 || !strings.Contains(fieldOf(forms[1], "local.path").Error, "back the repository up into itself: choose another place.") {
		t.Fatalf("forms = %+v, want the place inside the folder refused", forms)
	}
	if !strings.Contains(forms[0].Page.Subtitle, "step 1 of 4") || !strings.Contains(forms[3].Page.Subtitle, "step 3 of 3") {
		t.Errorf("subtitles %q, %q: a new destination has no folders to continue", forms[0].Page.Subtitle, forms[3].Page.Subtitle)
	}
	kits := requestsOf[KitScreen](ui)
	if len(kits) != 1 || kits[0].New || !strings.Contains(kits[0].Kit, "Secret Key:") || kits[0].RecoveryCode == "" ||
		!slices.ContainsFunc(kits[0].Lines, func(l Line) bool { return strings.Contains(l.Text, "can't be decrypted") }) {
		t.Errorf("kit screens = %+v", kits)
	}
	if sched.installs != 1 {
		t.Errorf("installed the scheduler %d times, want once", sched.installs)
	}
	if got := ui.said(OK); !slices.Equal(got, []string{"✓ ~/Projects/website backs up hourly to example-backups."}) {
		t.Errorf("said %+v", ui.Said)
	}

	cfg, err := backup.ReadConfig(d.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Destination != "example-backups" || cfg.Schedule != backup.Hourly || cfg.Keep != backup.DefaultKeep {
		t.Errorf("config = %+v", cfg)
	}
	data, _ := os.ReadFile(filepath.Join(d.Cwd, backup.ConfigFileName))
	if strings.Contains(string(data), "CT1-") || strings.Contains(string(data), masterPassword) || strings.Contains(string(data), repo) {
		t.Errorf("the folder's config file holds a secret or the address:\n%s", data)
	}
	if s := m.Status(ctx, d.Cwd); !s.Scheduled || s.DestinationMissing {
		t.Errorf("after Configure: scheduled %v, destination missing %v", s.Scheduled, s.DestinationMissing)
	}

	docs := filepath.Join(d.Cwd, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	ui = &fakeUI{t: t,
		Picks: []any{0, 2}, // example-backups, the current one; Weekly
		Forms: []any{Values{"hourly": "24", "daily": "7", "weekly": "4", "monthly": "12", "yearly": "3"}},
	}
	if err := configureFolder(ctx, ui, d, m.Status(ctx, docs)); err != nil {
		t.Fatalf("configureFolder from a subfolder: %v", err)
	}
	picks := requestsOf[Picker](ui)
	if len(picks) != 2 || picks[0].Cursor != 0 || !strings.HasSuffix(picks[0].Items[0].Cells[0], "(current)") ||
		!strings.Contains(picks[0].Page.Subtitle, "step 1 of 3") || picks[1].Cursor != 0 {
		t.Errorf("pickers = %+v, want each opening on the current value", picks)
	}
	again, err := backup.ReadConfig(d.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	if again.Schedule != backup.Weekly || again.ID != cfg.ID || again.Keep != cfg.Keep {
		t.Errorf("reconfigured from a subfolder: %+v, want weekly with the same ID and retention", again)
	}
	if _, err := os.Stat(filepath.Join(docs, backup.ConfigFileName)); !os.IsNotExist(err) {
		t.Error("no config file may be written in the subfolder")
	}
}

func fieldOf(f Form, key string) Field {
	for _, fd := range f.Fields {
		if fd.Key == key {
			return fd
		}
	}
	return Field{}
}

// The wizard steps back with esc, keeping what was chosen, and a failed
// scheduler install is said, with the folder still configured.
func TestConfigureStepsBackAndSaysASchedulerFailure(t *testing.T) {
	d, _ := standinFolder(t, false, backs, forgets, "example-backups")
	resticKeyring(t, d)
	d.Scheduler = &fakeScheduler{err: errors.New("example: no user session")}
	keep := Values{"hourly": "24", "daily": "7", "weekly": "4", "monthly": "12", "yearly": "0"}
	ui := &fakeUI{t: t,
		Dashboards: []any{"c"},
		Picks:      []any{0, 2, 0}, // example-backups; Weekly; then, back from the keep rules: Hourly
		Forms:      []any{ErrBack, keep},
	}
	folderFlow(context.Background(), ui, d)
	picks := requestsOf[Picker](ui)
	if len(picks) != 3 || picks[2].Cursor != 2 {
		t.Errorf("pickers = %+v, want How often again on Weekly after esc", picks)
	}
	cfg, err := backup.ReadConfig(d.Cwd)
	if err != nil || cfg.Schedule != backup.Hourly || cfg.Keep.Yearly != 0 {
		t.Fatalf("config = %+v, %v", cfg, err)
	}
	if got := ui.said(Warning); !slices.Equal(got, []string{"Couldn't schedule backups on this computer: example: no user session"}) {
		t.Errorf("warnings = %q", got)
	}
	dash := requestsOf[Dashboard](ui)
	if len(dash) != 2 || dash[1].Focus != "c" {
		t.Errorf("dashboards = %+v, want focus on Configure to retry", dash)
	}
}

// Under --dry-run Create and Connect are dimmed, and saving writes nothing:
// Configure says what it would write.
func TestConfigureDryRun(t *testing.T) {
	d, out := standinFolder(t, false, backs, forgets, "example-backups")
	resticKeyring(t, d)
	d.Machine.Opts.DryRun = true
	d.Scheduler = &fakeScheduler{}
	ui := &fakeUI{t: t, Picks: []any{0, 1},
		Forms: []any{Values{"hourly": "24", "daily": "7", "weekly": "4", "monthly": "12", "yearly": "3"}}}
	if err := configureFolder(context.Background(), ui, d, d.Machine.Status(context.Background(), d.Cwd)); err != nil {
		t.Fatal(err)
	}
	items := requestsOf[Picker](ui)[0].Items
	if !items[1].Dimmed || !items[2].Dimmed || items[1].Detail != "not in a dry run" {
		t.Errorf("destination items = %+v", items)
	}
	if _, err := os.Stat(filepath.Join(d.Cwd, backup.ConfigFileName)); !os.IsNotExist(err) {
		t.Error("a dry run wrote the config")
	}
	if !strings.Contains(out.String(), "[dry-run] write") || len(ui.said(OK)) != 0 {
		t.Errorf("output %q, said %+v", out, ui.Said)
	}
}

// linkFixture is a destination holding a folder another computer stopped
// backing up, which a new folder here may continue.
func linkFixture(t *testing.T) Deps {
	t.Helper()
	d, _ := standinFolder(t, false, backs, forgets, "example-backups")
	resticKeyring(t, d)
	d.Scheduler = &fakeScheduler{}
	snaps := `[{"id":"5a5a5a5a5a5a5a5a","short_id":"5a5a5a5a","time":"2026-11-21T18:40:00Z","hostname":"example-laptop",
"tags":["ctdev-folder:0a0a0a0a-0000-4000-8000-000000000001","ctdev-path:~/Code/website","ctdev-host:example-laptop"],
"summary":{"total_files_processed":3,"total_bytes_processed":1100}}]`
	if err := os.WriteFile(filepath.Join(d.Machine.Home, "snapshots.json"), []byte(snaps), 0o644); err != nil {
		t.Fatal(err)
	}
	return d
}

var defaultKeep = Values{"hourly": "24", "daily": "7", "weekly": "4", "monthly": "12", "yearly": "3"}

// A new folder can continue an existing folder's history: picking
// it gives the folder that ID; esc in the list keeps a new history; leaving
// the wizard saves nothing.
func TestConfigureBackupHistory(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name      string
		picks     []any
		continues bool
		cancelled bool
	}{
		{name: "esc", picks: []any{0, 1, ErrBack, 0, 1}},
		{name: "ctrl+c", picks: []any{0, 1}, cancelled: true},
		// Connect, the folder, then Next on Connect; back from How often
		// finds it still chosen.
		{name: "enter", picks: []any{0, 1, 0, 1, ErrBack, 1, 1}, continues: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := linkFixture(t)
			var forms []any
			if !c.cancelled {
				forms = []any{defaultKeep}
			}
			ui := &fakeUI{t: t, Picks: c.picks, Forms: forms}
			err := configureFolder(ctx, ui, d, d.Machine.Status(ctx, d.Cwd))
			cfg, readErr := backup.ReadConfig(d.Cwd)
			if c.cancelled {
				if !errors.Is(err, ErrCancelled) || !os.IsNotExist(readErr) {
					t.Errorf("want Configure cancelled with no config written, got %v, %v", err, readErr)
				}
				return
			}
			if err != nil || readErr != nil {
				t.Fatalf("Configure: %v, %v", err, readErr)
			}
			if (cfg.ID == "0a0a0a0a-0000-4000-8000-000000000001") != c.continues {
				t.Errorf("ID %s: continues = %v, want %v", cfg.ID, cfg.ID == "0a0a0a0a-0000-4000-8000-000000000001", c.continues)
			}
			picks := requestsOf[Picker](ui)
			if !strings.Contains(picks[1].Page.Subtitle, "step 2 of 4") || !strings.Contains(picks[2].Items[0].Group, "example-laptop") {
				t.Errorf("pickers = %+v", picks)
			}
			if c.continues {
				w11 := picks[3]
				if w11.Cursor != 1 || w11.Items[1].Detail != "~/Projects/website continues the backups of ~/Code/website on example-laptop." {
					t.Errorf("Backup history after the pick = %+v", w11)
				}
				if back := picks[5]; back.Cursor != 1 || back.Items[1].Detail == "" {
					t.Errorf("Backup history after esc = %+v, want the folder still chosen", back)
				}
			}
		})
	}
}

// Creating a destination under --all refuses a local path relative to the
// current directory, which --all can be run from anywhere, but takes one
// named in full, even under the current directory: ~/Backups from ~.
func TestCreatingADestinationUnderAllNeedsAPathNamedInFull(t *testing.T) {
	d, _ := standinFolder(t, false, backs, forgets)
	home := d.Machine.Home
	t.Chdir(home)
	ui := &fakeUI{t: t, Forms: []any{localPlace("example-backups", "backups"), localPlace("example-backups", "~/Backups")}}
	_, err := createDestination(context.Background(), ui, d, "", d.Machine.CheckNewAddressFromAll)
	if !errors.Is(err, ErrCancelled) {
		t.Errorf("want the setup cancelled when the script ends, got %v", err)
	}
	forms := requestsOf[Form](ui)
	if len(forms) != 3 {
		t.Fatalf("forms = %+v, want the place twice, then the master password", forms)
	}
	if e := fieldOf(forms[1], "local.path").Error; !strings.Contains(e, "give an absolute path or one starting with ~/") {
		t.Errorf("the relative path's refusal = %q", e)
	}
	if !strings.Contains(forms[2].Intro, "never stores the master password") || forms[2].Page.Title != "Backup destination › Create" {
		t.Errorf("~/Backups should be taken, moving on to the master password: %+v", forms[2])
	}
	if _, err := os.Stat(filepath.Join(home, "Backups")); !os.IsNotExist(err) {
		t.Errorf("a cancelled setup made ~/Backups (%v)", err)
	}
}

// createdKit is a destination the laptop created, and its setup.
func createdKit(t *testing.T) (Deps, backup.Setup) {
	t.Helper()
	d, _ := standinFolder(t, false, backs, forgets)
	resticKeyring(t, d)
	setup, err := d.Machine.CreateDestination(context.Background(),
		backup.NewDestination{Name: "example-backups", Address: filepath.Join(t.TempDir(), "repo")}, masterPassword)
	if err != nil {
		t.Fatal(err)
	}
	return d, setup
}

// Connecting with a kit: a wrong master password offers another try or the
// Recovery Code, three end it with nothing saved, and the right one sets
// this computer up with its own key.
func TestConnectWithAKit(t *testing.T) {
	laptop, setup := createdKit(t)
	desktop := another(t, laptop, "example-desktop")
	ctx := context.Background()

	kit := Values{"kit": setup.Kit}
	wrong := Values{"master": "wrong example password"}
	ui := &fakeUI{t: t, Forms: []any{kit, wrong, wrong, wrong}, Picks: []any{0, 0, 0}}
	if _, err := connectDestination(ctx, ui, desktop); err == nil || err.Error() != "three wrong tries; nothing was saved" {
		t.Fatalf("three wrong passwords: %v", err)
	}
	if picks := requestsOf[Picker](ui); len(picks) != 3 || picks[2].Note != "3 of 3 tries used." {
		t.Errorf("pickers = %+v", picks)
	}
	if names, _ := desktop.Machine.Destinations(); len(names) != 0 {
		t.Errorf("a wrong password saved %v", names)
	}

	ui = &fakeUI{t: t, Forms: []any{kit, wrong, Values{"master": masterPassword}}, Picks: []any{0}}
	dest, err := connectDestination(ctx, ui, desktop)
	if err != nil {
		t.Fatal(err)
	}
	if got := ui.said(OK); !slices.Equal(got, []string{"✓ This computer is set up on example-backups with its own key."}) || dest.Name != "example-backups" {
		t.Errorf("said %+v", ui.Said)
	}
	if forms := requestsOf[Form](ui); !strings.Contains(forms[1].Intro, "Pasted an Emergency Kit for example-backups (local path)") {
		t.Errorf("the password form = %+v", forms[1])
	}
}

// A wrong master password can be answered with the Recovery Code, which sets
// a new master password and Secret Key and shows the new kit once; a wrong
// code is said under it.
func TestConnectWithTheRecoveryCode(t *testing.T) {
	laptop, setup := createdKit(t)
	desktop := another(t, laptop, "example-desktop")
	reset := func(code string) Values {
		return Values{"code": code, "master": "new example password", "again": "new example password", "newcode": "yes"}
	}
	ui := &fakeUI{t: t,
		Forms: []any{Values{"kit": setup.Kit}, Values{"master": "wrong example password"}, reset("wrong-example-code"), reset(setup.RecoveryCode)},
		Picks: []any{1}, Kits: []error{nil}}
	dest, err := connectDestination(context.Background(), ui, desktop)
	if err != nil {
		t.Fatalf("%v (said %+v)", err, ui.Said)
	}
	forms := requestsOf[Form](ui)
	if e := fieldOf(forms[3], "code").Error; e != "That Recovery Code doesn't open the destination. Nothing was changed." {
		t.Errorf("the wrong code's error = %q", e)
	}
	kits := requestsOf[KitScreen](ui)
	if len(kits) != 1 || !kits[0].New || kits[0].RecoveryCode == "" || kits[0].RecoveryCode == setup.RecoveryCode || kits[0].Kit == setup.Kit {
		t.Errorf("kit screens = %+v, want the new kit and code", kits)
	}
	if _, err := backup.ParseKit(kits[0].Kit); err != nil || dest.Name != "example-backups" {
		t.Errorf("new kit: %v, destination %+v", err, dest)
	}
	if runs := requestsOf[Running](ui); runs[len(runs)-1].ChangesKeysOn != "example-backups" {
		t.Errorf("the reset can be cancelled: %+v", runs[len(runs)-1])
	}
}

// A kit for a destination this computer is already on updates it with no
// master password: its own key vouches for the kit.
func TestConnectWithAKitThisComputerIsOnUpdatesIt(t *testing.T) {
	d, setup := createdKit(t)
	ui := &fakeUI{t: t, Forms: []any{Values{"kit": setup.Kit}}}
	dest, err := connectDestination(context.Background(), ui, d)
	if err != nil {
		t.Fatal(err)
	}
	if forms := requestsOf[Form](ui); len(forms) != 1 {
		t.Errorf("forms = %+v, want only the paste: no master password", forms)
	}
	if got := ui.said(OK); !slices.Equal(got, []string{"✓ Saved the kit's credentials and Secret Key for example-backups on this computer."}) ||
		len(ui.said(Warning)) != 0 || dest.Name != "example-backups" {
		t.Errorf("said %+v", ui.Said)
	}

	// With this computer's key gone, the kit is refused under the field,
	// for another paste.
	removeStandinHostKey(t, d.Machine.Home, dest.Address, d.Machine.Hostname)
	ui = &fakeUI{t: t, Forms: []any{Values{"kit": setup.Kit}}}
	if _, err := pasteUpdatedKit(context.Background(), ui, d); !errors.Is(err, ErrCancelled) {
		t.Errorf("want the script to end at the next paste, got %v", err)
	}
	forms := requestsOf[Form](ui)
	if len(forms) != 2 || !strings.Contains(forms[1].Fields[0].Error, "This computer's key doesn't open the destination the kit describes") {
		t.Errorf("forms = %+v", forms)
	}
}

// esc while Saving takes in the folder's earlier one-offs leaves the wizard,
// since the config is already saved: it says what was saved and what wasn't,
// and doesn't install the scheduler. Configuring again finishes it.
func TestCancellingSaveAfterTheConfigIsWrittenSaysWhatWasSaved(t *testing.T) {
	d, _ := standinFolder(t, false, backs, forgets, "example-backups")
	sched := &fakeScheduler{}
	d.Scheduler = sched
	home := d.Machine.Home
	fifo := filepath.Join(t.TempDir(), "tagging")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	oneOff := `[{"id":"6b6b6b6b6b6b6b6b","short_id":"6b6b6b6b","time":"2026-11-21T18:40:00Z","hostname":"` + d.Machine.Hostname + `",
"tags":["ctdev-path:~/Projects/website","ctdev-host:` + d.Machine.Hostname + `"],"summary":{"total_files_processed":1,"total_bytes_processed":15}}]`
	script := `#!/bin/sh
echo "$*" >> "` + home + `/restic.log"
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
snapshots) if [ -e "` + home + `/tagged" ]; then echo '[]'; else echo '` + oneOff + `'; fi ;;
stats) echo '{"total_size": 0}' ;;
key) echo '[]' ;;
tag)
  if [ -e "` + home + `/tag-blocks" ]; then
    trap 'exit 130' INT
    echo tagging > "` + fifo + `"
    sleep 60 >/dev/null 2>&1 &
    wait $!
  fi
  touch "` + home + `/tagged" ;;
esac
`
	if err := os.WriteFile(filepath.Join(home, ".local", "bin", "restic"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "tag-blocks"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tagging := make(cancelWhen)
	go func() {
		_, _ = os.ReadFile(fifo)
		close(tagging)
	}()
	ui := &fakeUI{t: t,
		Dashboards: []any{"c"},
		Picks:      []any{0, 0}, // example-backups; Hourly
		Forms:      []any{defaultKeep},
		Runs:       []error{nil, nil, tagging}, // the status; reading the destination; Saving
	}
	folderFlow(context.Background(), ui, d)
	if _, err := backup.ReadConfig(d.Cwd); err != nil {
		t.Fatalf("the config wasn't saved: %v", err)
	}
	if got := ui.said(Warning); !slices.Equal(got, []string{"Saving stopped."}) {
		t.Errorf("warnings = %q", got)
	}
	if got := ui.said(Plain); len(got) != 1 || !strings.Contains(got[0], "Saved ~/Projects/website's settings") ||
		!strings.Contains(got[0], "aren't in its history yet") || !strings.Contains(got[0], "choose Configure again to finish") {
		t.Errorf("said %+v", ui.Said)
	}
	if sched.installs != 0 {
		t.Errorf("scheduler installed %d times after the stop, want none", sched.installs)
	}
	if forms := requestsOf[Form](ui); len(forms) != 1 {
		t.Errorf("forms = %d, want the wizard left, not the keep form again", len(forms))
	}
	dash := requestsOf[Dashboard](ui)
	if len(dash) != 2 || dash[1].Focus != "c" {
		t.Errorf("dashboards = %+v, want focus on Configure to finish", dash)
	}

	// Configuring again takes in the one-off and installs the scheduler.
	os.Remove(filepath.Join(home, "tag-blocks"))
	again := &fakeUI{t: t, Dashboards: []any{"c"}, Picks: []any{0, 0}, Forms: []any{defaultKeep}}
	folderFlow(context.Background(), again, d)
	if _, err := os.Stat(filepath.Join(home, "tagged")); err != nil {
		t.Error("configuring again didn't take in the one-off")
	}
	if sched.installs != 1 || len(again.said(Warning)) != 0 {
		t.Errorf("installs = %d, said %+v", sched.installs, again.Said)
	}
}
