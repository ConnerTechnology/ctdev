package backup

import "time"

// Due reports whether a folder on schedule s is due a backup at now: when it
// has never succeeded, or its last success is at least one interval ago.
// There's no time of day (CTD-87), so a computer that was asleep catches up
// on its next hourly run.
func Due(s Schedule, last Run, now time.Time) bool {
	return last.LastSuccess.IsZero() || now.Sub(last.LastSuccess) >= s.Interval()
}

// overdue reports whether a folder has had no success within twice its
// interval. since is its last success, or when it was put on the schedule if
// it has never succeeded; zero means there's nothing to measure from.
func overdue(interval time.Duration, since, now time.Time) bool {
	return !since.IsZero() && now.Sub(since) > 2*interval
}

// HealthState is how a scheduled folder is doing on this computer.
type HealthState string

const (
	HealthOK      HealthState = "ok"
	HealthFailed  HealthState = "failed"  // the last attempt failed
	HealthOverdue HealthState = "overdue" // no success within twice the interval
	HealthMissing HealthState = "missing" // the folder or its config file is gone
)

// FolderHealth is one scheduled folder's health.
type FolderHealth struct {
	Path     string // absolute
	Schedule Schedule
	State    HealthState
	Run      Run // the last attempt, zero when there hasn't been one
}

// judge is a folder's health from its list entry and its last run. A failure
// outranks overdue: it carries the error worth showing.
func judge(e folderEntry, r Run, now time.Time) HealthState {
	since := r.LastSuccess
	if since.IsZero() {
		since = e.Added
	}
	switch {
	case r.Missing:
		return HealthMissing
	case r.Failed():
		return HealthFailed
	case overdue(e.schedule().Interval(), since, now):
		return HealthOverdue
	}
	return HealthOK
}

// Health judges every folder on this computer's schedule list. It reads only
// folders.json and state.json: no folder, no destination and no network, so
// ctdev doctor can call it on any machine without root.
func (m *Machine) Health() ([]FolderHealth, error) {
	folders, err := m.scheduledFolders()
	if err != nil {
		return nil, err
	}
	runs, err := m.runs()
	if err != nil {
		return nil, err
	}
	now := m.now()
	out := make([]FolderHealth, 0, len(folders))
	for _, e := range folders {
		r := runs[e.Path]
		out = append(out, FolderHealth{Path: e.Path, Schedule: e.schedule(), State: judge(e, r, now), Run: r})
	}
	return out, nil
}

// schedule is the entry's schedule: daily for an entry written before the
// schedule was recorded, the same default Interval gives.
func (e folderEntry) schedule() Schedule {
	if e.Schedule == "" {
		return Daily
	}
	return e.Schedule
}

// now is the machine's clock: Now when a test set it, else the real time.
func (m *Machine) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}
