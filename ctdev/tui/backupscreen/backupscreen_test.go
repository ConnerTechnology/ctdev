package backupscreen

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/charmbracelet/x/ansi"
)

const home = "/home/example"

var (
	website = home + "/Projects/website"
	docs    = website + "/docs"
	cfg     = backup.NewConfig("example-backups")
)

func configured() backup.Status {
	return backup.Status{
		Folder: backup.Folder{Cwd: website, Root: website, Config: &cfg},
		Target: website, Destination: "example-backups", Backend: "Backblaze B2",
		Size: 1_200_000_000, Files: 4812,
		Last:      &backup.Run{LastAttempt: time.Date(2026, 10, 21, 21, 12, 0, 0, time.Local), LastSuccess: time.Date(2026, 10, 21, 21, 12, 0, 0, time.Local)},
		Scheduled: true, SchedulerInstalled: true, NextDue: time.Date(2026, 10, 22, 21, 12, 0, 0, time.Local),
		// The status carries the time it was taken, so no test depends on today.
		At:      time.Date(2026, 10, 22, 12, 0, 0, 0, time.Local),
		Backups: 14, Oldest: time.Date(2026, 9, 2, 2, 0, 0, 0, time.Local), Stored: 1_600_000_000,
	}
}

func unconfigured(backups int) backup.Status {
	return backup.Status{
		Folder: backup.Folder{Cwd: website}, Target: website,
		Destination: "example-backups", Backend: "local path", Files: 2, Size: 30, Backups: backups,
	}
}

func subfolder() backup.Status {
	s := configured()
	s.Cwd = docs
	return s
}

func plain(s string) string { return ansi.Strip(s) }

func firstLine(m *Model) string {
	line, _, _ := strings.Cut(plain(m.View().Content), "\n")
	return line
}

func TestHeaderLine(t *testing.T) {
	for name, tc := range map[string]struct {
		s    backup.Status
		want string
	}{
		"at the folder":  {configured(), "Backed up folder: ~/Projects/website"},
		"in a subfolder": {subfolder(), "Backed up folder: ~/Projects/website  (you're in ~/Projects/website/docs)"},
		"not backed up":  {unconfigured(0), "Not backed up: ~/Projects/website"},
		"one-offs only":  {unconfigured(2), "Not backed up: ~/Projects/website"},
	} {
		if got := firstLine(New(tc.s, home)); got != tc.want {
			t.Errorf("%s: header = %q, want %q", name, got, tc.want)
		}
	}
}

func items(m *Model) (labels, dimmed []string) {
	for _, it := range m.Items() {
		labels = append(labels, it.Label)
		if it.Dimmed {
			dimmed = append(dimmed, it.Label)
		}
	}
	return labels, dimmed
}

func TestMenuPerFolderState(t *testing.T) {
	for name, tc := range map[string]struct {
		s              backup.Status
		labels, dimmed string
	}{
		"configured": {configured(),
			"Back up now,List backups,Configure,Stop backing up,Delete all backups,Quit", ""},
		"subfolder": {subfolder(),
			"Back up now,List backups,Configure,Stop backing up,Delete all backups,Quit", ""},
		"not configured, no backups": {unconfigured(0),
			"Configure,Back up now (once),List backups,Stop backing up,Delete all backups,Quit",
			"List backups,Stop backing up,Delete all backups"},
		"not configured, with backups": {unconfigured(1),
			"Configure,Back up now (once),List backups,Stop backing up,Delete all backups,Quit",
			"Stop backing up"},
	} {
		labels, dimmed := items(New(tc.s, home))
		if got := strings.Join(labels, ","); got != tc.labels {
			t.Errorf("%s: items = %s\nwant %s", name, got, tc.labels)
		}
		if got := strings.Join(dimmed, ","); got != tc.dimmed {
			t.Errorf("%s: dimmed = %q, want %q", name, got, tc.dimmed)
		}
	}
}

func key(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }

func press(m *Model, keys ...tea.KeyPressMsg) {
	for _, k := range keys {
		m.Update(k)
	}
}

func TestEnterChoosesTheItemUnderTheCursor(t *testing.T) {
	m := New(configured(), home)
	press(m, key(tea.KeyEnter))
	if m.Chosen() != BackUpNow {
		t.Errorf("chosen = %v, want Back up now", m.Chosen())
	}

	m = New(unconfigured(0), home)
	press(m, key(tea.KeyDown), key(tea.KeyEnter))
	if m.Chosen() != BackUpNow {
		t.Errorf("chosen = %v, want Back up now (once)", m.Chosen())
	}
}

// The cursor passes over dimmed items, so they can't be chosen.
func TestDimmedItemsCantBeChosen(t *testing.T) {
	m := New(unconfigured(0), home)
	press(m, key(tea.KeyDown), key(tea.KeyDown), key(tea.KeyEnter))
	if m.Chosen() != Quit {
		t.Errorf("chosen = %v, want Quit: the three dimmed items are skipped", m.Chosen())
	}
}

func TestEveryAvailableItemCanBeChosen(t *testing.T) {
	for i, want := range []Action{BackUpNow, ListBackups, Configure, StopBackingUp, DeleteAll, Quit} {
		m := New(configured(), home)
		for range i {
			press(m, key(tea.KeyDown))
		}
		press(m, key(tea.KeyEnter))
		if m.Chosen() != want {
			t.Errorf("item %d: chosen = %v, want %v", i, m.Chosen(), want)
		}
	}
	if strings.Contains(plain(New(configured(), home).View().Content), "coming soon") {
		t.Error("nothing on the menu is coming soon any more")
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{{Code: 'q', Text: "q"}, key(tea.KeyEscape)} {
		m := New(configured(), home)
		press(m, k)
		if m.Chosen() != Quit {
			t.Errorf("%v: chosen = %v, want Quit", k, m.Chosen())
		}
	}
}

func TestStatusLines(t *testing.T) {
	styles.Disable()
	got := Status(configured(), home)
	for _, want := range []string{
		"Backed up folder: ~/Projects/website",
		"Destination", "example-backups (Backblaze B2)",
		"Size now", "1.2 GB · 4,812 files",
		"Last backup", "Oct 21, 9:12 PM · succeeded",
		"Schedule", "daily · next due Oct 22, 9:12 PM",
		"Keeping", "24 hourly, 7 daily, 4 weekly, 12 monthly, 3 yearly",
		"Backups", "14 · oldest Sep 2 · 1.6 GB stored",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("status is missing %q:\n%s", want, got)
		}
	}
}

// A config file that can't be read: the header says so, and only Quit is left.
func TestUnreadableConfig(t *testing.T) {
	s := backup.Status{
		Folder: backup.Folder{Cwd: docs, Root: website, ConfigErr: errors.New(`schedule "monthly" isn't hourly, daily or weekly`)},
		Target: website, Backups: 3,
	}
	want := `Backed up folder: ~/Projects/website  (you're in ~/Projects/website/docs) · config can't be read: schedule "monthly" isn't hourly, daily or weekly`
	if got := firstLine(New(s, home)); got != want {
		t.Errorf("header = %q\nwant     %q", got, want)
	}
	if got := plain(Status(s, home)); strings.Contains(got, "isn't configured") || strings.Contains(got, "Destination") {
		t.Errorf("an unreadable config isn't an unconfigured folder:\n%s", got)
	}
	m := New(s, home)
	for _, it := range m.Items() {
		if it.Dimmed != (it.Action != Quit) {
			t.Errorf("%s: dimmed %v; want everything but Quit dimmed", it.Label, it.Dimmed)
		}
	}
	press(m, key(tea.KeyEnter))
	if m.Chosen() != Quit {
		t.Errorf("chosen = %v, want Quit, the only item left", m.Chosen())
	}
}

func TestScheduleIsDueOnceNextDuePasses(t *testing.T) {
	s := configured()
	s.At = s.NextDue.Add(time.Minute)
	if got := plain(Status(s, home)); !strings.Contains(got, "daily · due now") {
		t.Errorf("status:\n%s", got)
	}
}

func TestStatusOfACloneSaysNotScheduledHere(t *testing.T) {
	s := configured()
	s.Scheduled = false
	if got := plain(Status(s, home)); !strings.Contains(got, "daily · not scheduled on this computer") {
		t.Errorf("status:\n%s", got)
	}
}

// The runner not being installed reads the same as not being on the list,
// in red.
func TestStatusSaysNotScheduledWhenTheRunnerIsMissing(t *testing.T) {
	s := configured()
	s.SchedulerInstalled = false
	got := Status(s, home)
	if !strings.Contains(plain(got), "daily · not scheduled on this computer") {
		t.Errorf("status:\n%s", plain(got))
	}
	if !strings.Contains(got, styles.Error.Render("daily · not scheduled on this computer")) {
		t.Errorf("not scheduled isn't in red:\n%q", got)
	}
}

func TestStatusOfAnOverdueFolder(t *testing.T) {
	s := configured()
	s.Overdue = true
	s.At = time.Date(2026, 10, 24, 9, 0, 0, 0, time.Local)
	got := Status(s, home)
	want := "daily · overdue: no backup since Oct 21, 9:12 PM"
	if !strings.Contains(plain(got), want) {
		t.Errorf("status:\n%s", plain(got))
	}
	if !strings.Contains(got, styles.Error.Render(want)) {
		t.Errorf("overdue isn't in red:\n%q", got)
	}
	s.Last = nil
	if got := plain(Status(s, home)); !strings.Contains(got, "daily · overdue: never backed up") {
		t.Errorf("status:\n%s", got)
	}
}

func TestStatusDueNow(t *testing.T) {
	s := configured()
	s.At = s.NextDue
	if got := plain(Status(s, home)); !strings.Contains(got, "daily · due now") {
		t.Errorf("status:\n%s", got)
	}
}

// On Linux the status says the runner works only while you're logged in:
// ctdev never enables lingering.
func TestStatusSaysScheduledBackupsNeedYouLoggedIn(t *testing.T) {
	s := configured()
	const note = "runs only while you're logged in"
	if got := plain(Status(s, home)); strings.Contains(got, note) {
		t.Errorf("macOS status has the Linux note:\n%s", got)
	}
	s.LoggedInOnly = true
	if got := plain(Status(s, home)); !strings.Contains(got, "Schedule       daily · next due Oct 22, 9:12 PM\n                 "+note+"\n") {
		t.Errorf("status:\n%s", got)
	}
}

func TestStatusNotesWhatCouldntBeRead(t *testing.T) {
	s := configured()
	s.Unreadable = 3
	if got := plain(Status(s, home)); !strings.Contains(got, "1.2 GB · 4,812 files · couldn't read 3") {
		t.Errorf("status:\n%s", got)
	}
}

func TestStatusOfAFailedBackup(t *testing.T) {
	s := configured()
	s.Last.Error = "example failure"
	if got := plain(Status(s, home)); !strings.Contains(got, "failed: example failure") {
		t.Errorf("status:\n%s", got)
	}
	s.Latest, s.LatestHost = time.Date(2026, 10, 22, 8, 30, 0, 0, time.Local), "example-desktop"
	if got := plain(Status(s, home)); !strings.Contains(got, "failed: example failure · newest backup Oct 22, 8:30 AM on example-desktop") {
		t.Errorf("status:\n%s", got)
	}
}

// A backup deferred because the destination was locked is noted in dim
// text under Last backup, not shown as a failure.
func TestStatusOfADeferredBackup(t *testing.T) {
	s := configured()
	s.Last.Deferred = "the destination is busy: something else holds its lock"
	got := Status(s, home)
	const note = "waiting: destination busy"
	if !strings.Contains(plain(got), "Last backup    Oct 21, 9:12 PM · succeeded\n                 "+note+"\n") {
		t.Errorf("status:\n%s", plain(got))
	}
	if !strings.Contains(got, styles.Dimmed.Render(note)) || strings.Contains(got, styles.Error.Render(note)) {
		t.Errorf("the note isn't dim:\n%q", got)
	}
}

// The destination's weekly data check has its own line once it's been read:
// not run yet, OK, or failed in red with the reason.
func TestStatusDataCheckLine(t *testing.T) {
	s := configured()
	if got := plain(Status(s, home)); strings.Contains(got, "Data check") {
		t.Errorf("a data check that wasn't read shows:\n%s", got)
	}
	s.DataCheckRead = true
	if got := plain(Status(s, home)); !strings.Contains(got, "  Data check     not run yet\n") {
		t.Errorf("status:\n%s", got)
	}
	at := time.Date(2026, 10, 20, 3, 0, 0, 0, time.Local)
	s.DataCheck = &backup.DataCheck{Host: "example-desktop", Started: at, Finished: at, Slice: 3, OK: true}
	if got := plain(Status(s, home)); !strings.Contains(got, "  Data check     Oct 20 · OK\n") {
		t.Errorf("status:\n%s", got)
	}
	s.DataCheck = &backup.DataCheck{Host: "example-desktop", Started: at, Finished: at, Slice: 3, Step: backup.StepCheck, Error: "damaged pack 0123abcd"}
	got := Status(s, home)
	want := "Oct 20 · failed at check: damaged pack 0123abcd"
	if !strings.Contains(plain(got), "  Data check     "+want+"\n") {
		t.Errorf("status:\n%s", plain(got))
	}
	if !strings.Contains(got, styles.Error.Render(want)) {
		t.Errorf("a failed data check isn't in red:\n%q", got)
	}
}

// A data check more than two weeks old, or none in two weeks, shows as
// overdue in yellow; a failure still shows in red.
func TestStatusOfAnOverdueDataCheck(t *testing.T) {
	s := configured()
	s.DataCheckRead, s.DataCheckOverdue = true, true
	got := Status(s, home)
	if !strings.Contains(got, styles.Warning.Render("overdue (never run)")) {
		t.Errorf("status:\n%q", got)
	}
	at := time.Date(2026, 10, 2, 3, 0, 0, 0, time.Local)
	s.DataCheck = &backup.DataCheck{Started: at, Finished: at, Slice: 3, OK: true}
	got = Status(s, home)
	if !strings.Contains(plain(got), "  Data check     overdue (last Oct 2)\n") || !strings.Contains(got, styles.Warning.Render("overdue (last Oct 2)")) {
		t.Errorf("status:\n%q", got)
	}
	s.DataCheck.OK, s.DataCheck.Step, s.DataCheck.Error = false, backup.StepCheck, "damaged pack 0123abcd"
	if got := Status(s, home); !strings.Contains(got, styles.Error.Render("Oct 2 · failed at check: damaged pack 0123abcd")) {
		t.Errorf("a failure that's also old isn't red:\n%q", got)
	}
}

// A keep-rules failure shows in yellow on the Keeping line; the backup it
// followed still shows as succeeded.
func TestStatusOfARetentionFailure(t *testing.T) {
	s := configured()
	s.Last.RetentionError = "example failure"
	got := Status(s, home)
	want := "24 hourly, 7 daily, 4 weekly, 12 monthly, 3 yearly · retention failed: example failure"
	if !strings.Contains(got, styles.Warning.Render(want)) {
		t.Errorf("the retention failure isn't in yellow on the Keeping line:\n%q", got)
	}
	if !strings.Contains(plain(got), "Oct 21, 9:12 PM · succeeded") {
		t.Errorf("status:\n%s", plain(got))
	}
}

// Last backup is the folder's newest from any computer, named when it isn't
// this one.
func TestLastBackupComesFromTheDestination(t *testing.T) {
	s := configured()
	s.Latest = time.Date(2026, 10, 22, 8, 30, 0, 0, time.Local)
	s.LatestHost, s.Hostname = "example-desktop", "example-laptop"
	if got := plain(Status(s, home)); !strings.Contains(got, "Oct 22, 8:30 AM · succeeded on example-desktop") {
		t.Errorf("status:\n%s", got)
	}
	s.LatestHost = "example-laptop"
	if got := plain(Status(s, home)); !strings.Contains(got, "Oct 22, 8:30 AM · succeeded\n") {
		t.Errorf("status:\n%s", got)
	}
}
