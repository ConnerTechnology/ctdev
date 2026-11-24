package backupapp

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/charmbracelet/x/ansi"
)

const home = "/home/example"

var now = time.Date(2026, 11, 23, 11, 0, 0, 0, time.Local)

// configured is a healthy, scheduled folder with four backups.
func configured() backup.Status {
	cfg := backup.NewConfig("example-backups")
	root := home + "/Projects/website"
	return backup.Status{
		Folder: backup.Folder{Cwd: root, Root: root, Config: &cfg}, Target: root,
		Destination: "example-backups", Backend: "Backblaze B2",
		Size: 1_200_000_000, Files: 4812,
		Last:      &backup.Run{LastAttempt: now.Add(-14 * time.Hour), LastSuccess: now.Add(-14 * time.Hour)},
		Scheduled: true, SchedulerInstalled: true, NextDue: now.Add(10 * time.Hour), At: now,
		Backups: 4, Oldest: now.Add(-4 * 24 * time.Hour), Latest: now.Add(-14 * time.Hour), Stored: 1_600_000_000,
		DataCheckRead: true, DataCheck: &backup.DataCheck{Finished: now.Add(-3 * 24 * time.Hour), OK: true},
	}
}

// unconfigured is a folder that isn't backed up, on a computer with one
// destination.
func unconfigured() backup.Status {
	root := home + "/Projects/website"
	return backup.Status{Folder: backup.Folder{Cwd: root}, Target: root, Destination: "example-backups",
		Backend: "Backblaze B2", Size: 1_200_000_000, Files: 4812, At: now}
}

type dashboardWant struct {
	badge   Badge
	focus   string // the key focused
	dimmed  []string
	label   string // Back up now's label
	message string // in the message lines
	body    string // in the rows
	absent  string // not in the rows
}

// Each state of the folder's dashboard, as rows R, S and T draw them.
func TestFolderDashboardStates(t *testing.T) {
	for name, c := range map[string]struct {
		s    func(*backup.Status)
		base func() backup.Status
		want dashboardWant
	}{
		"healthy": {base: configured, want: dashboardWant{badge: Badge{Healthy, "HEALTHY"}, focus: "b", label: "Back up now",
			body: "14 hours ago"}},
		"not backed up": {base: unconfigured, want: dashboardWant{badge: Badge{Attention, "NOT BACKED UP"}, focus: "c",
			dimmed: []string{"l", "s", "D"}, label: "Back up now (once)", message: "Configure to back it up on a schedule, or back it up once.",
			body: "none: this folder isn't configured"}},
		"last backup failed": {base: configured, s: func(s *backup.Status) {
			s.Last = &backup.Run{LastAttempt: now.Add(-2 * time.Hour), LastSuccess: now.Add(-14 * time.Hour), Error: "restic backup: example failure"}
		}, want: dashboardWant{badge: Badge{Danger, "LAST BACKUP FAILED"}, focus: "b", label: "Try again",
			message: "Newest backup still on the destination:", body: "failed 2 hours ago"}},
		"not scheduled": {base: configured, s: func(s *backup.Status) { s.SchedulerInstalled = false },
			want: dashboardWant{badge: Badge{Attention, "NOT SCHEDULED"}, focus: "c",
				message: "Nothing will back this folder up on this computer until it's scheduled.", body: "not scheduled on this computer"}},
		"overdue": {base: configured, s: func(s *backup.Status) { s.Overdue = true },
			want: dashboardWant{badge: Badge{Attention, "OVERDUE"}, focus: "b", body: "overdue: no backup since"}},
		"config can't be read": {base: configured, s: func(s *backup.Status) {
			s.Config, s.ConfigErr = nil, errors.New(`schedule "dialy" isn't hourly, daily or weekly`)
		}, want: dashboardWant{badge: Badge{Danger, "CONFIG CAN'T BE READ"}, focus: "q", dimmed: []string{"b", "l", "c", "s", "D"},
			message: "Fix the folder's config file first.", body: "SIZE NOW", absent: "LAST BACKUP"}},
		"destination not set up": {base: configured, s: func(s *backup.Status) { s.DestinationMissing, s.Backups, s.DataCheckRead = true, 0, false },
			want: dashboardWant{badge: Badge{Danger, "DESTINATION NOT SET UP"}, focus: "c", dimmed: []string{"b", "l", "s", "D"},
				message: "example-backups isn't set up on this computer.", body: "unknown: example-backups can't be read", absent: "DATA CHECK"}},
		"keep rules failed": {base: configured, s: func(s *backup.Status) { s.Last.RetentionError = "restic forget: example failure" },
			want: dashboardWant{badge: Badge{Attention, "KEEP RULES FAILED"}, focus: "b", body: "retention failed: restic forget: example failure"}},
		"keep rules failed and can't read": {base: configured, s: func(s *backup.Status) {
			s.Last.RetentionError, s.ReadErr = "restic forget: example failure", errors.New("first\nsecond")
		}, want: dashboardWant{badge: Badge{Danger, "CAN'T READ BACKUPS"}, focus: "b", body: "first; second"}},
		"destination busy": {base: configured, s: func(s *backup.Status) { s.Last.Deferred = "the destination is busy" },
			want: dashboardWant{badge: Badge{Healthy, "HEALTHY"}, focus: "b", body: "waiting: destination busy"}},
		"data check overdue": {base: configured, s: func(s *backup.Status) { s.DataCheckOverdue = true },
			want: dashboardWant{badge: Badge{Attention, "DATA CHECK OVERDUE"}, focus: "b", body: "overdue (last "}},
	} {
		t.Run(name, func(t *testing.T) {
			s := c.base()
			if c.s != nil {
				c.s(&s)
			}
			d := folderDashboard(s, home, s.Cwd, "", false)
			if d.Page.Badge != c.want.badge {
				t.Errorf("badge = %+v, want %+v", d.Page.Badge, c.want.badge)
			}
			if got := newDashboard(d); got.cursor < 0 || d.Actions[got.cursor].Key != c.want.focus {
				t.Errorf("focus = %d in %+v, want %q", got.cursor, d.Actions, c.want.focus)
			}
			var dimmed []string
			for _, a := range d.Actions {
				if a.Dimmed {
					dimmed = append(dimmed, a.Key)
				}
				if a.Key == "b" && c.want.label != "" && a.Label != c.want.label {
					t.Errorf("b reads %q, want %q", a.Label, c.want.label)
				}
			}
			slices.Sort(dimmed)
			want := slices.Clone(c.want.dimmed)
			slices.Sort(want)
			if !slices.Equal(dimmed, want) {
				t.Errorf("dimmed = %v, want %v", dimmed, want)
			}
			var msg []string
			for _, l := range d.Message {
				msg = append(msg, l.Text)
			}
			if !strings.Contains(strings.Join(msg, "\n"), c.want.message) {
				t.Errorf("message = %q, want %q in it", msg, c.want.message)
			}
			body := ansi.Strip(d.Page.Body)
			if !strings.Contains(body, c.want.body) {
				t.Errorf("rows:\n%s\nwant %q in them", body, c.want.body)
			}
			if c.want.absent != "" && strings.Contains(body, c.want.absent) {
				t.Errorf("rows:\n%s\nwant no %q", body, c.want.absent)
			}
		})
	}
}

// Run from a subfolder, the title stays the backed-up folder and says where
// you are.
func TestFolderDashboardFromASubfolder(t *testing.T) {
	s := configured()
	s.Cwd = s.Root + "/src"
	p := folderDashboard(s, home, s.Cwd, "", false).Page
	if p.Title != "~/Projects/website" || p.Context != "you're in ~/Projects/website/src" {
		t.Errorf("title %q, context %q", p.Title, p.Context)
	}
}

// After an action, focus goes where the loop says, unless that's dimmed.
func TestFolderDashboardFocusAfterAnAction(t *testing.T) {
	if b := newDashboard(folderDashboard(configured(), home, "", "q", false)); b.d.Actions[b.cursor].Key != "q" {
		t.Errorf("focus %d, want Quit", b.cursor)
	}
	d := folderDashboard(configured(), home, "", "b", true)
	if d.Actions[0].Label != "Try again" {
		t.Errorf("after a failed Back up now, b reads %q", d.Actions[0].Label)
	}
}

func TestRelative(t *testing.T) {
	for _, c := range []struct {
		t    time.Time
		want string
	}{
		{now.Add(-20 * time.Second), "just now"},
		{now.Add(-time.Minute), "1 minute ago"},
		{now.Add(-14 * time.Hour), "14 hours ago"},
		{now.Add(10 * time.Hour), "in 10 hours"},
		{now.Add(24*time.Hour - 5*time.Second), "in 24 hours"},
		{now.Add(-3 * 24 * time.Hour), "3 days ago"},
	} {
		if got := Relative(c.t, now); got != c.want {
			t.Errorf("Relative(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}
