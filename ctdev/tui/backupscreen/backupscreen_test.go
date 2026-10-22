package backupscreen

import (
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
		Scheduled: true, NextDue: time.Date(2026, 10, 22, 21, 12, 0, 0, time.Local),
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

func TestComingSoonItemsSaySoAndStay(t *testing.T) {
	m := New(configured(), home)
	press(m, key(tea.KeyDown), key(tea.KeyEnter))
	if m.Chosen() != None {
		t.Errorf("List backups isn't built yet; chosen = %v", m.Chosen())
	}
	if !strings.Contains(plain(m.View().Content), "List backups is coming soon") {
		t.Errorf("view doesn't say it's coming soon:\n%s", plain(m.View().Content))
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
