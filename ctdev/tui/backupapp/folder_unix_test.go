//go:build unix

package backupapp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

const exampleSummary = `{"message_type":"summary","snapshot_id":"0123456789abcdef","total_files_processed":1,"total_bytes_processed":15,"data_added":15,"backup_end":"2026-11-23T11:04:00Z"}`

// standinFolder is ~/Projects/website in a temporary home, with a restic
// stand-in at ~/.local/bin/restic, where ctdev looks first. backup is the
// stand-in's backup case, forget its forget case. Each named destination is
// set up on this computer; the folder is configured for the first when
// configure is set.
func standinFolder(t *testing.T, configure bool, backup_, forget string, dests ...string) (Deps, *bytes.Buffer) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "Projects", "website")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>example</p>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	m, err := backup.ThisMachine(sysutil.Opts{Stdout: out})
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(home, ".local", "bin", "restic")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
echo "$*" >> "` + filepath.Join(home, "restic.log") + `"
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
backup) ` + backup_ + ` ;;
snapshots) echo '[]' ;;
stats) echo '{"total_size": 0}' ;;
forget) ` + forget + ` ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range dests {
		env := filepath.Join(m.Dir, "destinations", name+".env")
		if err := os.MkdirAll(filepath.Dir(env), 0o700); err != nil {
			t.Fatal(err)
		}
		repo := filepath.Join(t.TempDir(), "repo")
		if err := os.WriteFile(env, []byte("RESTIC_REPOSITORY="+repo+"\nCTDEV_COMPUTER_KEY=example\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if configure {
		if err := backup.WriteConfig(dir, backup.NewConfig(dests[0])); err != nil {
			t.Fatal(err)
		}
	}
	return Deps{Machine: m, Cwd: dir}, out
}

func resticLog(t *testing.T, d Deps) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(d.Machine.Home, "restic.log"))
	return string(b)
}

const (
	backs   = `echo '` + exampleSummary + `'`
	forgets = `true`
	fails   = `echo "Fatal: example failure" >&2; exit 1`
)

// Back up now confirms, backs up, says so, applies the keep rules, and puts
// focus on Quit (R2 → R3 → R4).
func TestBackUpNow(t *testing.T) {
	d, _ := standinFolder(t, true, `echo '{"message_type":"status","percent_done":0.5,"total_files":2,"files_done":1,"total_bytes":30,"bytes_done":15,"seconds_remaining":2}'; `+backs, forgets, "example-backups")
	ui := &fakeUI{t: t, Dashboards: []any{"b"}, Confirms: []any{true}}
	if err := folderFlow(context.Background(), ui, d); err != ErrCancelled {
		t.Fatalf("err = %v, want the flow to end when the script does", err)
	}
	if got := ui.said(OK); !slices.Equal(got, []string{"✓ Backed up 1 file (15 B), 15 B new, as 01234567."}) {
		t.Errorf("said %+v", ui.Said)
	}
	c := requestsOf[Confirm](ui)
	if len(c) != 1 || c[0].Question != "Back up now" || !strings.Contains(c[0].Consequence, "All of ~/Projects/website") || !strings.Contains(c[0].Consequence, "example-backups") {
		t.Errorf("confirm = %+v", c)
	}
	runs := requestsOf[Running](ui)
	if len(runs) != 4 || runs[1].Page.Badge != (Badge{Busy, "BACKING UP"}) || runs[2].Label != "Applying the keep rules…" {
		t.Errorf("runs = %+v, want loading, backing up, keep rules, refresh", runs)
	}
	if len(ui.Progress) != 1 || ui.Progress[0].FilesDone != 1 {
		t.Errorf("progress = %+v, want restic's report", ui.Progress)
	}
	if !strings.Contains(resticLog(t, d), "forget") {
		t.Error("the keep rules didn't run")
	}
	dash := requestsOf[Dashboard](ui)
	if len(dash) != 2 || dash[1].Focus != "q" {
		t.Errorf("dashboards = %+v, want focus on Quit after the backup", dash)
	}
}

// Back up now applies the keep rules through Retain, so a failure is
// recorded on the folder and shows on the next dashboard (U7).
func TestBackUpNowRecordsARetentionFailure(t *testing.T) {
	d, _ := standinFolder(t, true, backs, fails, "example-backups")
	ui := &fakeUI{t: t, Dashboards: []any{"b"}, Confirms: []any{true}}
	folderFlow(context.Background(), ui, d)
	if w := ui.said(Warning); len(w) != 1 || !strings.HasPrefix(w[0], "Applying the keep rules failed, so older backups weren't thinned: ") {
		t.Errorf("warnings = %q, want the keep-rules warning", w)
	}
	s := d.Machine.Status(context.Background(), d.Cwd)
	if s.Last == nil || s.Last.Failed() || !strings.Contains(s.Last.RetentionError, "example failure") {
		t.Errorf("last run = %+v, want a success with the keep-rules failure recorded", s.Last)
	}
	dash := requestsOf[Dashboard](ui)
	if body := dash[len(dash)-1].Page.Body; !strings.Contains(body, "retention failed") {
		t.Errorf("the refreshed rows don't show the keep-rules failure:\n%s", body)
	}
}

// When the destination stays locked, Back up now says it's busy and to try
// again, in yellow, and the run is deferred, not failed (S3).
func TestBackUpNowSaysWhenTheDestinationIsBusy(t *testing.T) {
	d, _ := standinFolder(t, true, `echo "Fatal: repository is already locked" >&2; exit 11`, forgets, "example-backups")
	ui := &fakeUI{t: t, Dashboards: []any{"b"}, Confirms: []any{true}}
	folderFlow(context.Background(), ui, d)
	want := []string{"The destination is busy (weekly maintenance or a delete holds its lock).", "Try again in a few minutes."}
	if got := ui.said(Warning); !slices.Equal(got, want) || len(ui.said(Error)) > 0 {
		t.Errorf("said %+v, want %q in yellow", ui.Said, want)
	}
	if s := d.Machine.Status(context.Background(), d.Cwd); s.Last == nil || s.Last.Failed() || s.Last.Deferred == "" {
		t.Errorf("last run = %+v, want deferred, not failed", s.Last)
	}
	dash := requestsOf[Dashboard](ui)
	if last := dash[len(dash)-1]; last.Focus != "b" || last.Actions[0].Label != "Try again" {
		t.Errorf("after busy: focus %q, b reads %q; want Try again, focused", last.Focus, last.Actions[0].Label)
	}
}

// A backup restic made while skipping unreadable files is still a backup,
// with a note (U6).
func TestBackUpNowNotesAPartialBackup(t *testing.T) {
	d, _ := standinFolder(t, true, backs+`; exit 3`, forgets, "example-backups")
	ui := &fakeUI{t: t, Dashboards: []any{"b"}, Confirms: []any{true}}
	folderFlow(context.Background(), ui, d)
	if len(ui.said(OK)) != 1 || !slices.Equal(ui.said(Warning), []string{"Note: some files couldn't be read."}) {
		t.Errorf("said %+v, want the success and the note", ui.Said)
	}
}

// A failed backup is said in red, recorded, and offers to try again (U8).
func TestBackUpNowFails(t *testing.T) {
	d, _ := standinFolder(t, true, fails, forgets, "example-backups")
	ui := &fakeUI{t: t, Dashboards: []any{"b"}, Confirms: []any{true}}
	folderFlow(context.Background(), ui, d)
	if e := ui.said(Error); len(e) != 1 || !strings.HasPrefix(e[0], "backup failed: ") {
		t.Errorf("errors = %q", e)
	}
	dash := requestsOf[Dashboard](ui)
	if last := dash[len(dash)-1]; last.Page.Badge.Text != "LAST BACKUP FAILED" || last.Focus != "b" {
		t.Errorf("after the failure: %+v", last)
	}
}

// esc while backing up cancels it: nothing is recorded, and the dashboard
// comes back with a dim note (U9).
func TestCancelledBackUpNow(t *testing.T) {
	d, _ := standinFolder(t, true, `exec sleep 30`, forgets, "example-backups")
	ui := &fakeUI{t: t, Dashboards: []any{"b"}, Confirms: []any{true}, Runs: []error{nil, ErrBack}}
	folderFlow(context.Background(), ui, d)
	if !slices.Equal(ui.said(Info), []string{"Cancelled — no backup was made."}) || len(ui.said(Error)) > 0 {
		t.Errorf("said %+v", ui.Said)
	}
	if s := d.Machine.Status(context.Background(), d.Cwd); s.Last != nil {
		t.Errorf("last run = %+v, want none recorded", s.Last)
	}
}

// esc on the confirm backs nothing up.
func TestBackUpNowConfirmCancelled(t *testing.T) {
	d, _ := standinFolder(t, true, backs, forgets, "example-backups")
	ui := &fakeUI{t: t, Dashboards: []any{"b"}, Confirms: []any{false}}
	folderFlow(context.Background(), ui, d)
	if strings.Contains(resticLog(t, d), "backup") || len(ui.Said) > 0 {
		t.Errorf("restic ran %q, said %+v; want nothing", resticLog(t, d), ui.Said)
	}
}

// A folder that isn't configured is backed up once, to the destination
// chosen among several, with no keep rules (U2 → U3 → U5).
func TestBackUpOnceChoosesTheDestination(t *testing.T) {
	d, _ := standinFolder(t, false, backs, fails, "example-archive", "example-backups")
	ui := &fakeUI{t: t, Dashboards: []any{"b"}, Picks: []any{1}, Confirms: []any{true}}
	folderFlow(context.Background(), ui, d)
	p := requestsOf[Picker](ui)
	if len(p) != 1 || !p[0].Radio || p[0].Items[1].Cells[0] != "example-backups" {
		t.Fatalf("pickers = %+v", p)
	}
	c := requestsOf[Confirm](ui)
	if len(c) != 1 || c[0].Question != "Back up once" || !strings.Contains(c[0].Consequence, "example-backups") || !strings.Contains(c[0].Consequence, "A one-off") {
		t.Errorf("confirm = %+v", c)
	}
	if strings.Contains(resticLog(t, d), "forget") || len(ui.said(OK)) != 1 {
		t.Errorf("said %+v, restic %q; want a backup and no keep rules", ui.Said, resticLog(t, d))
	}
}

// With no destination, Back up once offers to set one up first (U1).
func TestBackUpOnceWithNoDestination(t *testing.T) {
	d, _ := standinFolder(t, false, backs, forgets)
	ui := &fakeUI{t: t, Dashboards: []any{"b"}, Confirms: []any{true}}
	folderFlow(context.Background(), ui, d)
	c := requestsOf[Confirm](ui)
	if len(c) != 1 || c[0].Yes != "Set one up" {
		t.Errorf("confirm = %+v", c)
	}
	if e := ui.said(Error); len(e) != 1 || !strings.Contains(e[0], "isn't built yet") {
		t.Errorf("errors = %q, want the create flow's", e)
	}
}

// Under --dry-run nothing is backed up and no success is said; restic's
// commands go to the log (U10).
func TestBackUpNowDryRun(t *testing.T) {
	d, out := standinFolder(t, true, backs, forgets, "example-backups")
	d.Machine.Opts.DryRun = true
	ui := &fakeUI{t: t, Dashboards: []any{"b"}, Confirms: []any{true}}
	folderFlow(context.Background(), ui, d)
	if len(ui.Said) > 0 || !strings.Contains(out.String(), "[dry-run] restic backup") || !strings.Contains(out.String(), "[dry-run] restic forget") {
		t.Errorf("said %+v, log %q", ui.Said, out)
	}
}
