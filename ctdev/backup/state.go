package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/secretfile"
)

// folderEntry is one folder this computer backs up on a schedule. Schedule
// and Added are copied here so ctdev doctor can judge the folder's health from
// this file and state.json alone, without opening the folder.
type folderEntry struct {
	Path     string    `json:"path"`
	ID       string    `json:"id"`
	Schedule Schedule  `json:"schedule,omitempty"`
	Added    time.Time `json:"added,omitzero"`
}

type foldersFile struct {
	Folders []folderEntry `json:"folders"`
}

func (m *Machine) foldersFile() string { return filepath.Join(m.Dir, "folders.json") }
func (m *Machine) stateFile() string   { return filepath.Join(m.Dir, "state.json") }

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return secretfile.WriteAtomic(path, append(data, '\n'), 0o600)
}

// addFolder puts the folder on this computer's schedule list, replacing any
// entry for the same path. Reconfiguring keeps when the folder was first
// added, so a folder that has never backed up can't be made to look new.
//
// A folder that was moved keeps its ID in its config file. An entry with the
// same ID at a path that no longer exists, which a scheduled run has already
// recorded as missing, is the folder's old place: it's dropped, with its
// state, and the caller is told. One that isn't recorded missing stays: it
// may be on a drive that isn't mounted right now, and it was fine at its last
// run. So does one at a path that exists, a copy of the folder.
func (m *Machine) addFolder(root, id string, schedule Schedule) error {
	unlock, err := m.stateLock()
	if err != nil {
		return err
	}
	defer unlock()
	var f foldersFile
	if err := readJSON(m.foldersFile(), &f); err != nil {
		return err
	}
	var s stateFile
	if err := readJSON(m.stateFile(), &s); err != nil {
		return err
	}
	entry := folderEntry{Path: root, ID: id, Schedule: schedule, Added: m.now()}
	kept := f.Folders[:0]
	var moved []string
	for _, e := range f.Folders {
		switch {
		case e.Path == root:
			if e.ID == id && !e.Added.IsZero() {
				entry.Added = e.Added
			}
		case e.ID == id && s.Folders[e.Path].Missing && !exists(e.Path):
			// The new place starts fresh: due now, and not overdue
			// until twice its interval has passed without a success.
			moved = append(moved, e.Path)
		default:
			kept = append(kept, e)
		}
	}
	f.Folders = append(kept, entry)
	sort.Slice(f.Folders, func(i, j int) bool { return f.Folders[i].Path < f.Folders[j].Path })
	if err := writeJSON(m.foldersFile(), f); err != nil {
		return err
	}
	if len(moved) == 0 {
		return nil
	}
	for _, p := range moved {
		delete(s.Folders, p)
		fmt.Fprintf(m.out(), "Removed %s from this computer's list: it moved to %s.\n",
			DisplayPath(m.Home, p), DisplayPath(m.Home, root))
	}
	return writeJSON(m.stateFile(), s)
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return !errors.Is(err, os.ErrNotExist)
}

// setSchedule records a folder's schedule on the list when its config file
// has changed it, such as after a git pull, so doctor judges it by the new one.
func (m *Machine) setSchedule(root string, schedule Schedule) error {
	unlock, err := m.stateLock()
	if err != nil {
		return err
	}
	defer unlock()
	var f foldersFile
	if err := readJSON(m.foldersFile(), &f); err != nil {
		return err
	}
	changed := false
	for i, e := range f.Folders {
		if e.Path == root && e.Schedule != schedule {
			f.Folders[i].Schedule, changed = schedule, true
		}
	}
	if !changed {
		return nil
	}
	return writeJSON(m.foldersFile(), f)
}

// removeFolder takes the folder at root off this computer's schedule list.
func (m *Machine) removeFolder(root string) error {
	unlock, err := m.stateLock()
	if err != nil {
		return err
	}
	defer unlock()
	var f foldersFile
	if err := readJSON(m.foldersFile(), &f); err != nil {
		return err
	}
	kept := f.Folders[:0]
	for _, e := range f.Folders {
		if e.Path != root {
			kept = append(kept, e)
		}
	}
	if len(kept) == len(f.Folders) {
		return nil
	}
	f.Folders = kept
	return writeJSON(m.foldersFile(), f)
}

// scheduledFolders is this computer's schedule list. Writes replace the file
// atomically, so a read needs no lock.
func (m *Machine) scheduledFolders() ([]folderEntry, error) {
	var f foldersFile
	err := readJSON(m.foldersFile(), &f)
	return f.Folders, err
}

// scheduled reports whether this computer's list holds the folder at root
// with this ID.
func (m *Machine) scheduled(root, id string) (bool, error) {
	_, ok, err := m.scheduledEntry(root, id)
	return ok, err
}

// scheduledEntry finds the folder at root with this ID on this computer's list.
func (m *Machine) scheduledEntry(root, id string) (folderEntry, bool, error) {
	folders, err := m.scheduledFolders()
	if err != nil {
		return folderEntry{}, false, err
	}
	for _, e := range folders {
		if e.Path == root && e.ID == id {
			return e, true, nil
		}
	}
	return folderEntry{}, false, nil
}

// Run is the record of a folder's last backup on this computer.
type Run struct {
	LastAttempt time.Time `json:"last_attempt"`
	LastSuccess time.Time `json:"last_success,omitzero"`
	Error       string    `json:"error,omitempty"`
	// Missing means a scheduled run found the folder gone, or its config
	// file gone or holding another folder's ID, and skipped it.
	Missing bool `json:"missing,omitempty"`
	// Alerted means a "failing" notification went out and no "recovered"
	// one has since, so a folder that keeps failing notifies once.
	Alerted bool `json:"alerted,omitempty"`
}

// Failed reports whether the last attempt failed.
func (r Run) Failed() bool { return r.Error != "" }

type stateFile struct {
	Folders map[string]Run `json:"folders"`
}

// updateRun changes the folder's record in state.json.
func (m *Machine) updateRun(root string, change func(*Run)) error {
	unlock, err := m.stateLock()
	if err != nil {
		return err
	}
	defer unlock()
	var s stateFile
	if err := readJSON(m.stateFile(), &s); err != nil {
		return err
	}
	if s.Folders == nil {
		s.Folders = map[string]Run{}
	}
	r := s.Folders[root]
	change(&r)
	s.Folders[root] = r
	return writeJSON(m.stateFile(), s)
}

// recordRun notes a backup attempt of the folder at root in state.json.
func (m *Machine) recordRun(root string, at time.Time, runErr error) error {
	return m.updateRun(root, func(r *Run) {
		r.LastAttempt, r.Error, r.Missing = at, "", false
		if runErr != nil {
			r.Error = runErr.Error()
		} else {
			r.LastSuccess = at
		}
	})
}

// recordMissing notes that a scheduled run skipped the folder at root
// because it, or its config file, wasn't where the list said.
func (m *Machine) recordMissing(root string, at time.Time, why string) error {
	return m.updateRun(root, func(r *Run) {
		r.LastAttempt, r.Error, r.Missing = at, why, true
	})
}

// runs is every folder's record in state.json.
func (m *Machine) runs() (map[string]Run, error) {
	var s stateFile
	err := readJSON(m.stateFile(), &s)
	return s.Folders, err
}

// lastRun is the folder's last recorded attempt on this computer.
func (m *Machine) lastRun(root string) (Run, bool, error) {
	runs, err := m.runs()
	if err != nil {
		return Run{}, false, err
	}
	r, ok := runs[root]
	return r, ok, nil
}
