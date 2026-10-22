package backup

import (
	"errors"
	"os"
	"path/filepath"
)

// errLocked means another process holds this computer's backup lock: a
// scheduled run is in progress.
var errLocked = errors.New("another ctdev backup run holds the lock")

// The lock serializes every read-modify-write of folders.json and state.json
// across processes, and a scheduled run holds it for its whole run, so a
// second runner, or Configure, never interleaves with one. It's an advisory
// flock on ~/.config/ctdev/backup/lock, released when the process exits.
// Inside one process it's reentrant: RunScheduled holds it while Backup
// records its result.
func (m *Machine) lockFile() string { return filepath.Join(m.Dir, "lock") }

// lock takes the lock, waiting for it unless wait is false, in which case a
// lock held elsewhere is errLocked. The returned func releases it.
func (m *Machine) lock(wait bool) (func(), error) {
	m.lockMu.Lock()
	defer m.lockMu.Unlock()
	if m.lockDepth > 0 {
		m.lockDepth++
		return m.unlock, nil
	}
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(m.lockFile(), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := flock(f, wait); err != nil {
		f.Close()
		return nil, err
	}
	m.lockHeld, m.lockDepth = f, 1
	return m.unlock, nil
}

func (m *Machine) unlock() {
	m.lockMu.Lock()
	defer m.lockMu.Unlock()
	if m.lockDepth--; m.lockDepth == 0 && m.lockHeld != nil {
		m.lockHeld.Close() // closing the file releases the flock
		m.lockHeld = nil
	}
}

// runInProgress reports whether a scheduled run holds the lock right now,
// without waiting and without creating anything.
func (m *Machine) runInProgress() bool {
	m.lockMu.Lock()
	held := m.lockDepth > 0
	m.lockMu.Unlock()
	if held {
		return false // it's ours
	}
	f, err := os.OpenFile(m.lockFile(), os.O_RDWR, 0)
	if err != nil {
		return false // no lock file: nothing has ever run
	}
	defer f.Close()
	return errors.Is(flock(f, false), errLocked)
}
