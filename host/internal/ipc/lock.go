package ipc

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// lockPath is the file a daemon keeps locked for as long as it serves
// (Lock, or Listen, to Server.Close). It stays on disk: removing a lock file
// opens a race.
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

// waitLock is tryLock retried every 10 ms until wait has passed.
func waitLock(path string, wait time.Duration) (*os.File, error) {
	deadline := time.Now().Add(wait)
	for {
		f, err := tryLock(path)
		if !errors.Is(err, errLocked) || time.Now().After(deadline) {
			return f, err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ServeLock is the data folder's daemon lock (lockPath), held from Lock to
// Close. serve takes it before anything else and hands it to ListenLocked.
type ServeLock struct {
	dir string
	mu  sync.Mutex
	f   *os.File // nil once released
}

// Lock takes the daemon lock of dataDir without waiting, creating the folder
// if needed: ErrAlreadyRunning when another daemon holds it (or another Lock
// of this process: the lock is per open file). serve takes it first, so that
// a second serve stops before it touches the workspace or starts watching
// the phone, instead of finding out at Listen.
func Lock(dataDir string) (*ServeLock, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	f, err := tryLock(lockPath(dataDir))
	if errors.Is(err, errLocked) {
		return nil, ErrAlreadyRunning
	}
	if err != nil {
		return nil, err
	}
	return &ServeLock{dir: dataDir, f: f}, nil
}

// Close releases the lock. Later calls, and a nil lock, do nothing: the
// Server that was given it closes it too.
func (l *ServeLock) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// held reports whether the lock has not been released yet.
func (l *ServeLock) held() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f != nil
}
