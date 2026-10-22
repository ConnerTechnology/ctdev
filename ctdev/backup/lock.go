package backup

import (
	"errors"
	"os"
	"path/filepath"
)

// errLocked means another process holds the lock: for the run lock, a
// scheduled run is in progress.
var errLocked = errors.New("another ctdev backup run holds the lock")

// Two advisory flocks in ~/.config/ctdev/backup, both released when the
// process exits:
//
//   - The run lock (run.lock) is held for a whole `ctdev backup run-scheduled`,
//     taken without waiting, so only one scheduled run happens at a time.
//   - The state lock (state.lock) is held only for one read-modify-write of
//     folders.json or state.json, so Configure, Back up now and a scheduled
//     run never lose each other's writes, and none of them waits on a
//     scheduled backup for longer than one write.
func (m *Machine) runLockFile() string   { return filepath.Join(m.Dir, "run.lock") }
func (m *Machine) stateLockFile() string { return filepath.Join(m.Dir, "state.lock") }

func openLock(dir, path string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
}

// runLock takes the run lock without waiting: errLocked when a scheduled run
// in another process, or on another *Machine, holds it.
func (m *Machine) runLock() (func(), error) {
	f, err := openLock(m.Dir, m.runLockFile())
	if err != nil {
		return nil, err
	}
	if err := flock(f, false); err != nil {
		f.Close()
		return nil, err
	}
	m.lockMu.Lock()
	m.runHeld = f
	m.lockMu.Unlock()
	return func() {
		m.lockMu.Lock()
		m.runHeld = nil
		m.lockMu.Unlock()
		f.Close() // closing the file releases the flock
	}, nil
}

// stateLock takes the state lock, waiting for it. It's re-entrant per
// *Machine, not per process: a nested call on the same *Machine counts up,
// while two *Machines in one process exclude each other like two processes.
func (m *Machine) stateLock() (func(), error) {
	m.lockMu.Lock()
	defer m.lockMu.Unlock()
	if m.lockDepth > 0 {
		m.lockDepth++
		return m.stateUnlock, nil
	}
	f, err := openLock(m.Dir, m.stateLockFile())
	if err != nil {
		return nil, err
	}
	if err := flock(f, true); err != nil {
		f.Close()
		return nil, err
	}
	m.lockHeld, m.lockDepth = f, 1
	return m.stateUnlock, nil
}

func (m *Machine) stateUnlock() {
	m.lockMu.Lock()
	defer m.lockMu.Unlock()
	if m.lockDepth--; m.lockDepth == 0 && m.lockHeld != nil {
		m.lockHeld.Close()
		m.lockHeld = nil
	}
}

// runInProgress reports whether a scheduled run holds the run lock right
// now, without waiting and without creating anything. A run this *Machine
// holds doesn't count.
func (m *Machine) runInProgress() bool {
	m.lockMu.Lock()
	ours := m.runHeld != nil
	m.lockMu.Unlock()
	if ours {
		return false
	}
	f, err := os.OpenFile(m.runLockFile(), os.O_RDWR, 0)
	if err != nil {
		return false // no lock file: nothing has ever run
	}
	defer f.Close()
	return errors.Is(flock(f, false), errLocked)
}
