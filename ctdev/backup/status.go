package backup

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

// Interval is how long after the last successful backup the next is due.
func (s Schedule) Interval() time.Duration {
	switch s {
	case Hourly:
		return time.Hour
	case Weekly:
		return 7 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// Status is everything `ctdev backup` reports about the folder it runs in.
type Status struct {
	Folder

	// Target is the folder actions apply to: the backed-up folder, or the
	// current directory when nothing at or above it is set up.
	Target string

	// Destination names the folder's destination, or for a folder that isn't
	// configured, the destinations this computer has. Backend is its kind.
	Destination        string
	Backend            string
	DestinationMissing bool // configured for a destination this computer isn't set up on

	Size  int64 // bytes in the folder now
	Files int   // files in the folder now

	// Unreadable counts files and folders the size walk couldn't read; a
	// backup would skip them too.
	Unreadable int

	Last      *Run      // this computer's last attempt, if it made one
	Scheduled bool      // on this computer's schedule list
	NextDue   time.Time // when Scheduled
	// Overdue means a scheduled folder has had no success on this computer
	// within twice its interval.
	Overdue bool
	// SchedulerInstalled is whether this computer's hourly runner (the
	// systemd user timer or launchd agent) is installed, when Scheduled.
	SchedulerInstalled bool
	// LoggedInOnly says scheduled backups run only while the user is
	// logged in: ctdev never enables lingering on Linux (CTD-87).
	LoggedInOnly bool

	// At is when the status was taken; "due now" is judged against it.
	At time.Time

	Backups int       // the folder's backups in the destination
	Oldest  time.Time // the oldest of them
	Latest  time.Time // the newest of them, from any computer
	// LatestHost is the computer that made the newest backup.
	LatestHost string
	// Hostname is this computer's, to tell its backups from others'.
	Hostname string
	Stored   int64 // space they take in the destination

	// DataCheck is the configured folder's destination's last weekly
	// maintenance, read from the destination; nil when none has run.
	// DataCheckRead says it was read, so nil means "not run yet".
	DataCheck     *DataCheck
	DataCheckRead bool

	ReadErr error // the destination couldn't be read
}

// HasBackups reports whether the folder has at least one backup.
func (s Status) HasBackups() bool { return s.Backups > 0 }

// Failed reports whether this computer's last backup of the folder failed.
func (s Status) Failed() bool { return s.Last != nil && s.Last.Failed() }

// Status reports on the backed-up folder at or above cwd. Problems reading
// the folder or the destination land in ReadErr, so a status can always be shown.
func (m *Machine) Status(ctx context.Context, cwd string) Status {
	s := Status{Hostname: m.Hostname, At: m.now()}
	f, err := Find(cwd)
	s.Folder = f
	if err != nil {
		s.ReadErr = err
		return s
	}
	s.Target = f.Root
	if s.Target == "" {
		s.Target = f.Cwd
	}
	if f.ConfigErr != nil {
		// Without the config there's no destination or ID to look up.
		s.Size, s.Files, s.Unreadable, err = folderSize(s.Target)
		s.ReadErr = errors.Join(s.ReadErr, err)
		return s
	}
	s.Size, s.Files, s.Unreadable, err = folderSize(s.Target)
	s.ReadErr = errors.Join(s.ReadErr, err)

	if run, ok, err := m.lastRun(s.Target); err != nil {
		s.ReadErr = errors.Join(s.ReadErr, err)
	} else if ok {
		s.Last = &run
	}

	var dests []*Destination
	var entry folderEntry
	if f.Config != nil {
		s.Destination = f.Config.Destination
		if entry, s.Scheduled, err = m.scheduledEntry(f.Root, f.Config.ID); err != nil {
			s.ReadErr = errors.Join(s.ReadErr, err)
		}
		if names, err := m.Destinations(); err != nil {
			// Couldn't list them: that's a read error, not a missing destination.
			s.ReadErr = errors.Join(s.ReadErr, err)
		} else if !contains(names, s.Destination) {
			s.DestinationMissing = true
		} else if d, err := m.Destination(s.Destination); err != nil {
			s.ReadErr = errors.Join(s.ReadErr, err)
		} else {
			dests = append(dests, d)
			if s.DataCheck, err = d.LastDataCheck(ctx); err != nil {
				s.ReadErr = errors.Join(s.ReadErr, err)
			} else {
				s.DataCheckRead = true
			}
		}
	} else {
		names, err := m.Destinations()
		s.ReadErr = errors.Join(s.ReadErr, err)
		s.Destination = strings.Join(names, ", ")
		for _, name := range names {
			d, err := m.Destination(name)
			if err != nil {
				s.ReadErr = errors.Join(s.ReadErr, err)
				continue
			}
			dests = append(dests, d)
		}
	}
	var backends []string
	for _, d := range dests {
		backends = append(backends, d.Backend())
	}
	s.Backend = strings.Join(backends, ", ")

	ref := m.Ref(s.Target, f.Config)
	for _, d := range dests {
		snaps, err := d.Snapshots(ctx, ref)
		if err != nil {
			s.ReadErr = errors.Join(s.ReadErr, err)
			continue
		}
		s.Backups += len(snaps)
		if len(snaps) > 0 {
			if first := snaps[0].Time; s.Oldest.IsZero() || first.Before(s.Oldest) {
				s.Oldest = first
			}
			if last := snaps[len(snaps)-1]; last.Time.After(s.Latest) {
				s.Latest, s.LatestHost = last.Time, last.Host
			}
			stored, err := d.Stored(ctx, snaps)
			s.ReadErr = errors.Join(s.ReadErr, err)
			s.Stored += stored
		}
	}

	if s.Scheduled {
		now := s.At
		// No time of day: the next backup is due an interval after the last
		// success, and right away when there hasn't been one.
		s.NextDue = now
		var last Run
		if s.Last != nil {
			last = *s.Last
		}
		if !last.LastSuccess.IsZero() {
			s.NextDue = last.LastSuccess.Add(f.Config.Schedule.Interval())
		}
		entry.Schedule = f.Config.Schedule
		s.Overdue = judge(entry, Run{LastSuccess: last.LastSuccess}, now) == HealthOverdue
		s.SchedulerInstalled = m.SchedulerInstalled()
		// Lingering keeps a user timer running after logout. ctdev never turns
		// it on, but someone may have, and then the note would be wrong.
		s.LoggedInOnly = m.goos() == "linux" && !m.lingering()
	}
	return s
}

// folderSize counts the regular files under dir and their bytes, the way the
// backup will see them. What it can't read is counted and skipped, as restic
// would skip it; only an unreadable dir itself is an error.
func folderSize(dir string) (size int64, files, unreadable int, err error) {
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir {
				return err
			}
			unreadable++
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				unreadable++
				return nil
			}
			size += info.Size()
			files++
		}
		return nil
	})
	return size, files, unreadable, err
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
