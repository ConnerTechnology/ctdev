//go:build unix

package backupapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
)

// example is a backup in the restic stand-in's list.
type example struct {
	id, when, host string
	files          int
	size, added    int64
}

var (
	older = example{"aaaaaaaa11111111", "2026-11-20T21:10:00Z", "example-laptop", 4800, 1_150_000_000, 1_140_000_000}
	newer = example{"bbbbbbbb22222222", "2026-11-21T21:12:00Z", "example-desktop", 4812, 1_200_000_000, 12_000_000}
	third = example{"cccccccc33333333", "2026-11-22T09:00:00Z", "example-laptop", 4815, 1_200_000_000, 3_000_000}
)

func snapshotsJSON(examples ...example) string {
	var objs []string
	for _, e := range examples {
		objs = append(objs, fmt.Sprintf(`{"id":%q,"short_id":%q,"time":%q,"hostname":%q,"summary":{"backup_start":"2026-11-21T21:12:00Z","backup_end":"2026-11-21T21:12:04Z","files_new":12,"files_changed":3,"total_files_processed":%d,"total_bytes_processed":%d,"data_added":%d}}`,
			e.id, e.id[:8], e.when, e.host, e.files, e.size, e.added))
	}
	return "[" + strings.Join(objs, ",") + "]"
}

// standinBackups is standinFolder with a restic stand-in that lists each
// destination's backups (setBackups), forgets with forget, and dumps with
// dump. Its default forget leaves the destination's backups after forget.
func standinBackups(t *testing.T, configure bool, forget, dump string, dests ...string) Deps {
	t.Helper()
	d, _ := standinFolder(t, configure, "true", "true", dests...)
	d.Scheduler = &removals{}
	if forget == "" {
		forget = `cp "$RESTIC_REPOSITORY.after" "$RESTIC_REPOSITORY.json"`
	}
	script := `#!/bin/sh
echo "$*" >> "` + filepath.Join(d.Machine.Home, "restic.log") + `"
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
snapshots) case "$*" in *ctdev-maintenance*) echo '[]' ;; *) cat "$RESTIC_REPOSITORY.json" 2>/dev/null || echo '[]' ;; esac ;;
stats) echo '{"total_size": 1600000000}' ;;
forget) ` + forget + ` ;;
dump) ` + dump + ` ;;
esac
`
	if err := os.WriteFile(filepath.Join(d.Machine.Home, ".local", "bin", "restic"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return d
}

// setBackups makes dest list now, and after a forget, after.
func setBackups(t *testing.T, d Deps, dest string, now, after []example) {
	t.Helper()
	dst, err := d.Machine.Destination(dest)
	if err != nil {
		t.Fatal(err)
	}
	for ext, list := range map[string][]example{".json": now, ".after": after} {
		if err := os.WriteFile(dst.Address+ext, []byte(snapshotsJSON(list...)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// forgetLine is the restic forget the stand-in ran.
func forgetLine(t *testing.T, d Deps) string {
	t.Helper()
	for _, l := range strings.Split(resticLog(t, d), "\n") {
		if strings.HasPrefix(l, "forget ") {
			return l
		}
	}
	return ""
}

func configExists(d Deps) bool {
	_, err := os.Stat(filepath.Join(d.Cwd, backup.ConfigFileName))
	return err == nil
}

// List backups reads the folder's backups with a spinner, lists them newest
// first, opens one, downloads it beside the folder, and goes back.
func TestListBackupsAndDownload(t *testing.T) {
	d := standinBackups(t, true, "", "true", "example-backups")
	setBackups(t, d, "example-backups", []example{older, newer}, nil)
	ui := &fakeUI{t: t, Dashboards: []any{"l", "z", "esc"}, Picks: []any{1, ErrBack}}
	folderFlow(context.Background(), ui, d)

	runs := requestsOf[Running](ui)
	if len(runs) < 2 || runs[1].Label != "Listing the backups of ~/Projects/website…" || runs[1].Page.Title != "~/Projects/website › Backups" {
		t.Errorf("runs = %+v, want the list loading", runs)
	}
	p := requestsOf[Picker](ui)
	if len(p) != 2 {
		t.Fatalf("pickers = %+v, want the list, then the list again after the backup", p)
	}
	items := p[0].Items
	if len(items) != 2 || items[0].Cells[0] != Absolute(mustTime(newer.when)) || !strings.Contains(items[0].Cells[len(items[0].Cells)-1], "latest") {
		t.Errorf("items = %+v, want the newest first, tagged latest", items)
	}
	if want := []string{Absolute(mustTime(older.when)), "1.1 GB", "1.1 GB", "4,800"}; !slices.Equal(items[1].Cells, want) {
		t.Errorf("older row = %q, want %q", items[1].Cells, want)
	}
	for _, want := range []string{"2 backups", "1.6 GB", "stored in", "example-backups"} {
		if !strings.Contains(p[0].Page.Subtitle, want) {
			t.Errorf("subtitle %q is missing %q", p[0].Page.Subtitle, want)
		}
	}
	if len(p[0].Columns) != 4 || p[1].Cursor != 1 {
		t.Errorf("columns %+v, cursor back on %d; want four columns and the cursor on the backup opened", p[0].Columns, p[1].Cursor)
	}

	dash := requestsOf[Dashboard](ui)
	detail := dash[1]
	if detail.Page.Title != "~/Projects/website › Backups › "+Absolute(mustTime(older.when)) {
		t.Errorf("detail title = %q", detail.Page.Title)
	}
	for _, want := range []string{"SIZE", "1.1 GB", "4,800 files", "NEW DATA", "3 changed · 12 added", "4s", "example-laptop", "aaaaaaaa"} {
		if !strings.Contains(detail.Page.Body, want) {
			t.Errorf("detail is missing %q:\n%s", want, detail.Page.Body)
		}
	}
	if strings.Contains(detail.Page.Body, "NOTE") || strings.Contains(detail.Page.Subtitle, "latest") {
		t.Errorf("detail of an older backup with no warning: %+v", detail.Page)
	}
	zip := filepath.Join(filepath.Dir(d.Cwd), "website-20261120T211000Z.zip")
	if ok := ui.said(OK); !slices.Equal(ok, []string{"✓ Saved ~/Projects/website-20261120T211000Z.zip"}) {
		t.Errorf("said %+v", ui.Said)
	}
	if _, err := os.Stat(zip); err != nil {
		t.Errorf("the zip isn't there: %v", err)
	}
	if !strings.Contains(resticLog(t, d), "dump --retry-lock 30m --archive zip") {
		t.Errorf("restic ran %q", resticLog(t, d))
	}
	if dash[len(dash)-1].Focus != "l" {
		t.Errorf("focus after the list = %q, want List backups", dash[len(dash)-1].Focus)
	}
}

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// A failed download is said in red, and the backup stays on screen to try
// again.
func TestDownloadFailed(t *testing.T) {
	d := standinBackups(t, true, "", `echo "Fatal: example failure" >&2; exit 1`, "example-backups")
	setBackups(t, d, "example-backups", []example{newer}, nil)
	ui := &fakeUI{t: t, Dashboards: []any{"l", "z"}, Picks: []any{0}}
	folderFlow(context.Background(), ui, d)
	if e := ui.said(Error); len(e) != 1 || !strings.HasPrefix(e[0], "Download failed: ") || !strings.Contains(e[0], "example failure") {
		t.Errorf("errors = %q", e)
	}
	dash := requestsOf[Dashboard](ui)
	if len(dash) != 3 || dash[2].Page.Title != dash[1].Page.Title || dash[2].Focus != "z" {
		t.Errorf("dashboards = %+v, want the backup again, focus on Download", dash)
	}
	if !strings.Contains(dash[1].Page.Subtitle, "latest") {
		t.Errorf("the only backup's subtitle %q doesn't say latest", dash[1].Page.Subtitle)
	}
}

// Delete this backup asks y-only, forgets the one backup, says when the
// space is freed, and the list comes back without it, the cursor where it
// was.
func TestDeleteOneBackup(t *testing.T) {
	d := standinBackups(t, true, "", "true", "example-backups")
	setBackups(t, d, "example-backups", []example{older, newer}, []example{newer})
	ui := &fakeUI{t: t, Dashboards: []any{"l", "D"}, Picks: []any{1, ErrBack}, Confirms: []any{true}}
	folderFlow(context.Background(), ui, d)

	c := requestsOf[Confirm](ui)
	if len(c) != 1 || c[0].Question != "Delete the backup from "+Absolute(mustTime(older.when))+"?" || !c[0].Destructive ||
		!strings.Contains(c[0].Consequence, "aaaaaaaa · 1.1 GB · 4,800 files · made on example-laptop") {
		t.Errorf("confirm = %+v", c)
	}
	if !strings.Contains(resticLog(t, d), "forget --retry-lock 30m aaaaaaaa11111111") {
		t.Errorf("restic ran %q", resticLog(t, d))
	}
	want := []Line{{OK, "✓ Deleted backup aaaaaaaa."}, {Info, "Space is freed at the weekly maintenance, which is due within the hour."}}
	if !slices.Equal(ui.Said, want) {
		t.Errorf("said %+v, want %+v", ui.Said, want)
	}
	p := requestsOf[Picker](ui)
	if len(p) != 2 || len(p[1].Items) != 1 || p[1].Cursor != 0 || !strings.Contains(p[1].Page.Subtitle, "1 backup") {
		t.Errorf("pickers = %+v, want the refreshed list of one", p)
	}
}

// Anything but y keeps the backup, with a dim note.
func TestDeleteOneCancelled(t *testing.T) {
	d := standinBackups(t, true, "", "true", "example-backups")
	setBackups(t, d, "example-backups", []example{older, newer}, nil)
	ui := &fakeUI{t: t, Dashboards: []any{"l", "D"}, Picks: []any{0}, Confirms: []any{false}}
	folderFlow(context.Background(), ui, d)
	if strings.Contains(resticLog(t, d), "forget") || !slices.Equal(ui.Said, []Line{{Info, "Nothing was deleted."}}) {
		t.Errorf("said %+v, restic %q", ui.Said, resticLog(t, d))
	}
	dash := requestsOf[Dashboard](ui)
	if len(dash) != 3 || dash[2].Focus != "D" {
		t.Errorf("dashboards = %+v, want the backup again, focus on Delete", dash)
	}
}

// A busy destination is a wait in yellow, and the backup stays on screen.
func TestDeleteOneWhenBusy(t *testing.T) {
	d := standinBackups(t, true, `echo "Fatal: repository is already locked" >&2; exit 11`, "true", "example-backups")
	setBackups(t, d, "example-backups", []example{newer}, nil)
	ui := &fakeUI{t: t, Dashboards: []any{"l", "D"}, Picks: []any{0}, Confirms: []any{true}}
	folderFlow(context.Background(), ui, d)
	if w := ui.said(Warning); len(w) != 2 || len(ui.said(OK)) > 0 || len(ui.said(Error)) > 0 {
		t.Errorf("said %+v, want the busy lines", ui.Said)
	}
	if dash := requestsOf[Dashboard](ui); len(dash) != 3 || dash[2].Page.Title != dash[1].Page.Title {
		t.Errorf("dashboards = %+v, want the backup again", dash)
	}
}

// Deleting the last backup leaves the list saying there are none, with
// esc back to the dashboard.
func TestDeletingTheLastBackup(t *testing.T) {
	d := standinBackups(t, true, "", "true", "example-backups")
	setBackups(t, d, "example-backups", []example{newer}, nil)
	ui := &fakeUI{t: t, Dashboards: []any{"l", "D", "esc"}, Picks: []any{0}, Confirms: []any{true}}
	folderFlow(context.Background(), ui, d)
	dash := requestsOf[Dashboard](ui)
	if len(dash) != 4 {
		t.Fatalf("dashboards = %+v", dash)
	}
	none := dash[2]
	if !strings.Contains(none.Page.Body, "~/Projects/website has no backups.") || !strings.Contains(none.Page.Subtitle, "0 backups") {
		t.Errorf("no-backups screen = %+v", none.Page)
	}
	if last := dash[3]; !slices.ContainsFunc(last.Actions, func(a Action) bool { return a.Key == "l" && a.Dimmed }) {
		t.Errorf("after: %+v, want List backups dimmed", last.Actions)
	}
}

// An unconfigured folder's one-offs are listed from every destination,
// with a column naming each backup's, and a delete dates the space line by
// the backup's own destination.
func TestListOneOffsInEveryDestination(t *testing.T) {
	d := standinBackups(t, false, "", "true", "example-archive", "example-backups")
	setBackups(t, d, "example-archive", []example{newer}, nil)
	setBackups(t, d, "example-backups", []example{older, third}, []example{older})
	ui := &fakeUI{t: t, Dashboards: []any{"l", "D"}, Picks: []any{0, ErrBack}, Confirms: []any{true}}
	folderFlow(context.Background(), ui, d)
	p := requestsOf[Picker](ui)
	if len(p) != 2 {
		t.Fatalf("pickers = %+v", p)
	}
	if n := len(p[0].Columns); n != 5 || p[0].Columns[4].Title != "DESTINATION" {
		t.Errorf("columns = %+v, want a DESTINATION column", p[0].Columns)
	}
	var dests []string
	for _, it := range p[0].Items {
		dests = append(dests, it.Cells[4])
	}
	if !slices.Equal(dests, []string{"example-backups", "example-archive", "example-backups"}) {
		t.Errorf("destinations = %q, want newest first across both", dests)
	}
	for _, want := range []string{"one-off backups · ", "3 backups", "example-archive", "example-backups"} {
		if !strings.Contains(p[0].Page.Subtitle, want) {
			t.Errorf("subtitle %q is missing %q", p[0].Page.Subtitle, want)
		}
	}
	if !strings.Contains(resticLog(t, d), "forget --retry-lock 30m cccccccc33333333") || len(p[1].Items) != 2 {
		t.Errorf("restic ran %q; list after %+v", resticLog(t, d), p[1].Items)
	}
}

// Delete all lists the folder's backups now, asks y-only, has the folder's
// name typed, and forgets exactly those.
func TestDeleteAll(t *testing.T) {
	d := standinBackups(t, true, "", "true", "example-backups")
	setBackups(t, d, "example-backups", []example{older, newer}, nil)
	ui := &fakeUI{t: t, Dashboards: []any{"D"}, Confirms: []any{true, true}}
	folderFlow(context.Background(), ui, d)
	c := requestsOf[Confirm](ui)
	if len(c) != 2 || c[0].Question != "Delete all 2 backups of ~/Projects/website?" || !c[0].Destructive ||
		!strings.Contains(c[0].Consequence, "2 backups · 1.6 GB stored in example-backups. The folder stays backed up.") ||
		c[1].TypeToConfirm != "website" || c[1].Question != c[0].Question {
		t.Errorf("confirms = %+v", c)
	}
	if forget := forgetLine(t, d); !strings.Contains(forget, "aaaaaaaa11111111") || !strings.Contains(forget, "bbbbbbbb22222222") {
		t.Errorf("restic ran %q", resticLog(t, d))
	}
	want := []Line{{OK, "✓ Deleted 2 backups of ~/Projects/website."}, {Info, "Space is freed at the weekly maintenance, which is due within the hour."}}
	if !slices.Equal(ui.Said, want) {
		t.Errorf("said %+v, want %+v", ui.Said, want)
	}
	runs := requestsOf[Running](ui)
	if len(runs) != 4 || runs[2].Page.Badge != (Badge{Busy, "DELETING"}) || runs[2].Label != "Deleting 2 backups of ~/Projects/website…" {
		t.Errorf("runs = %+v, want loading, listing, deleting, refresh", runs)
	}
	if dash := requestsOf[Dashboard](ui); dash[len(dash)-1].Focus != "q" {
		t.Errorf("focus after = %q, want Quit", dash[len(dash)-1].Focus)
	}
	if !configExists(d) {
		t.Error("Delete all stopped backing up the folder")
	}
}

// Anything but y, or esc while typing the name, deletes nothing, with a
// dim note and focus back on Delete all.
func TestDeleteAllCancelled(t *testing.T) {
	for name, answers := range map[string][]any{"no": {false}, "name not typed": {true, false}} {
		t.Run(name, func(t *testing.T) {
			d := standinBackups(t, true, "", "true", "example-backups")
			setBackups(t, d, "example-backups", []example{newer}, nil)
			ui := &fakeUI{t: t, Dashboards: []any{"D"}, Confirms: answers}
			folderFlow(context.Background(), ui, d)
			if c := requestsOf[Confirm](ui); c[0].Question != "Delete the only backup of ~/Projects/website?" {
				t.Errorf("question = %q", c[0].Question)
			}
			if strings.Contains(resticLog(t, d), "forget") || !slices.Equal(ui.Said, []Line{{Info, "Nothing was deleted."}}) {
				t.Errorf("said %+v, restic %q", ui.Said, resticLog(t, d))
			}
			if dash := requestsOf[Dashboard](ui); dash[len(dash)-1].Focus != "D" {
				t.Errorf("focus after = %q, want Delete all", dash[len(dash)-1].Focus)
			}
		})
	}
}

// One-offs in two destinations are deleted from each, and the space line
// can't give one date.
func TestDeleteAllFromEveryDestination(t *testing.T) {
	d := standinBackups(t, false, "", "true", "example-archive", "example-backups")
	setBackups(t, d, "example-archive", []example{newer}, nil)
	setBackups(t, d, "example-backups", []example{older}, nil)
	ui := &fakeUI{t: t, Dashboards: []any{"D"}, Confirms: []any{true, true}}
	folderFlow(context.Background(), ui, d)
	log := resticLog(t, d)
	if !strings.Contains(log, "forget --retry-lock 30m bbbbbbbb22222222") || !strings.Contains(log, "forget --retry-lock 30m aaaaaaaa11111111") {
		t.Errorf("restic ran %q", log)
	}
	if c := requestsOf[Confirm](ui); strings.Contains(c[0].Consequence, "stays backed up") {
		t.Errorf("consequence %q for a folder that isn't backed up", c[0].Consequence)
	}
	if got := ui.said(Info); !slices.Equal(got, []string{"Space is freed at the next weekly maintenance."}) {
		t.Errorf("said %+v", ui.Said)
	}
}

// When the list comes back empty though the status counted backups,
// Delete all says so and asks nothing.
func TestDeleteAllWithNoBackups(t *testing.T) {
	d := standinBackups(t, true, "", "true", "example-backups")
	s := d.Machine.Status(context.Background(), d.Cwd)
	ui := &fakeUI{t: t}
	if err := deleteAllBackups(context.Background(), ui, d, s); !errors.Is(err, ErrBack) {
		t.Errorf("err = %v, want back", err)
	}
	if !slices.Equal(ui.Said, []Line{{Plain, "~/Projects/website has no backups."}}) || len(requestsOf[Confirm](ui)) > 0 {
		t.Errorf("said %+v, asked %+v", ui.Said, ui.Requests)
	}
}

// Under --dry-run Delete all deletes nothing and says no success; restic's
// command goes to the log.
func TestDeleteAllDryRun(t *testing.T) {
	d := standinBackups(t, true, "", "true", "example-backups")
	setBackups(t, d, "example-backups", []example{newer}, nil)
	out := &strings.Builder{}
	d.Machine.Opts.Stdout = out
	d.Machine.Opts.DryRun = true
	ui := &fakeUI{t: t, Dashboards: []any{"D"}, Confirms: []any{true, true}}
	folderFlow(context.Background(), ui, d)
	if len(ui.Said) > 0 || strings.Contains(resticLog(t, d), "forget") || !strings.Contains(out.String(), "[dry-run] restic forget") {
		t.Errorf("said %+v, restic %q, log %q", ui.Said, resticLog(t, d), out)
	}
}

// Stop backing up, keeping the backups: the folder comes off the schedule,
// the runner is removed if idle, and it says where the backups are.
func TestStopBackingUpKeepingTheBackups(t *testing.T) {
	d := standinBackups(t, true, "", "true", "example-backups")
	setBackups(t, d, "example-backups", []example{older, newer}, nil)
	ui := &fakeUI{t: t, Dashboards: []any{"s"}, Confirms: []any{true, false}}
	folderFlow(context.Background(), ui, d)
	c := requestsOf[Confirm](ui)
	if len(c) != 2 || c[0].Question != "Stop backing up ~/Projects/website?" || !c[0].Destructive ||
		!strings.Contains(c[0].Consequence, "Removes .ctdev-backup.json and takes the folder off this computer's schedule.") ||
		c[1].Question != "Also delete all 2 backups of this folder, from every computer?" || c[1].Keep != "Keep them" {
		t.Errorf("confirms = %+v", c)
	}
	if configExists(d) || strings.Contains(resticLog(t, d), "forget") {
		t.Errorf("configured %v, restic %q; want stopped, nothing deleted", configExists(d), resticLog(t, d))
	}
	if n := d.Scheduler.(*removals).n; n != 1 {
		t.Errorf("RemoveIfIdle called %d times, want once", n)
	}
	want := []Line{{OK, "✓ Stopped backing up ~/Projects/website."},
		{Plain, "Its 2 backups are kept under this folder's ID."},
		{Plain, "ctdev backup --all lists them, and Configure → \"Connect to an existing folder's backups\" resumes the history."}}
	if !slices.Equal(ui.Said, want) {
		t.Errorf("said %+v, want %+v", ui.Said, want)
	}
}

// Stop backing up and delete: after the name is typed, the backups listed
// go, then the config and schedule entry; a backup made meanwhile is kept
// and said, and so is a runner that couldn't be removed.
func TestStopBackingUpAndDelete(t *testing.T) {
	d := standinBackups(t, true, "", "true", "example-backups")
	setBackups(t, d, "example-backups", []example{newer}, []example{third})
	d.Scheduler = &removals{err: errors.New("example failure")}
	ui := &fakeUI{t: t, Dashboards: []any{"s"}, Confirms: []any{true, true, true}}
	folderFlow(context.Background(), ui, d)
	c := requestsOf[Confirm](ui)
	if len(c) != 3 || c[1].Question != "Also delete the only backup of this folder, from every computer?" || c[2].TypeToConfirm != "website" {
		t.Errorf("confirms = %+v", c)
	}
	if configExists(d) || !strings.Contains(resticLog(t, d), "forget --retry-lock 30m bbbbbbbb22222222") {
		t.Errorf("configured %v, restic %q; want stopped and deleted", configExists(d), resticLog(t, d))
	}
	want := []Line{{OK, "✓ Stopped backing up ~/Projects/website and deleted its backups."},
		{Info, "Space is freed at the weekly maintenance, which is due within the hour."},
		{Warning, "A backup was made while deleting, so it's kept under this folder's ID: ctdev backup --all lists it."},
		{Warning, "Couldn't remove the hourly backup runner: example failure"}}
	if !slices.Equal(ui.Said, want) {
		t.Errorf("said %+v, want %+v", ui.Said, want)
	}
	if runs := requestsOf[Running](ui); runs[2].Page.Badge != (Badge{Busy, "DELETING"}) || runs[2].Label != "Deleting 1 backup of ~/Projects/website…" {
		t.Errorf("runs = %+v", runs)
	}
}

// Every way out before the last step changes nothing: no on the first
// question, esc on the second, and the name not typed.
func TestStopBackingUpCancelled(t *testing.T) {
	for name, answers := range map[string][]any{
		"no":             {false},
		"esc on delete?": {true, ErrBack},
		"name not typed": {true, true, false},
	} {
		t.Run(name, func(t *testing.T) {
			d := standinBackups(t, true, "", "true", "example-backups")
			setBackups(t, d, "example-backups", []example{newer}, nil)
			ui := &fakeUI{t: t, Dashboards: []any{"s"}, Confirms: answers}
			folderFlow(context.Background(), ui, d)
			if !configExists(d) || strings.Contains(resticLog(t, d), "forget") || d.Scheduler.(*removals).n > 0 {
				t.Errorf("configured %v, restic %q; want nothing changed", configExists(d), resticLog(t, d))
			}
			if !slices.Equal(ui.Said, []Line{{Info, "Nothing was changed."}}) {
				t.Errorf("said %+v", ui.Said)
			}
			if dash := requestsOf[Dashboard](ui); dash[len(dash)-1].Focus != "s" {
				t.Errorf("focus after = %q, want Stop backing up", dash[len(dash)-1].Focus)
			}
		})
	}
}

// A folder with no backups is stopped after the one question.
func TestStopBackingUpWithNoBackups(t *testing.T) {
	d := standinBackups(t, true, "", "true", "example-backups")
	ui := &fakeUI{t: t, Dashboards: []any{"s"}, Confirms: []any{true}}
	folderFlow(context.Background(), ui, d)
	if configExists(d) || !slices.Equal(ui.Said, []Line{{OK, "✓ Stopped backing up ~/Projects/website."}}) {
		t.Errorf("configured %v, said %+v", configExists(d), ui.Said)
	}
}
