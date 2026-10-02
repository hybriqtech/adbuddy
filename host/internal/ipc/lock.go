package ipc

import (
	"errors"
	"os"
	"path/filepath"
)

// lockPath is the file a daemon keeps locked for as long as it serves
// (Listen to Close). It stays on disk: removing a lock file opens a race.
func lockPath(dataDir string) string { return filepath.Join(dataDir, "adbuddy.lock") }

// errLocked means someone else holds the lock.
var errLocked = errors.New("the lock is held")

// tryLock opens path (creating it, readable only by the user) and takes an
// exclusive lock on it without waiting: errLocked when another process, or
// another open of the same file, holds it. Closing the file releases the
// lock, and so does the end of the process, however it ends.
func tryLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
