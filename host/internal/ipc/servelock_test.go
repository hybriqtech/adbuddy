package ipc

import (
	"errors"
	"path/filepath"
	"testing"
)

// serve takes the data folder's lock before anything else (Lock) and hands it
// to the server (ListenLocked): while it is held, neither another Lock nor a
// Listen gets it, and the server's Close releases it. Closing it again does
// nothing.
func TestLockIsHeldUntilTheServerCloses(t *testing.T) {
	dir := shortDir(t)
	secret, err := EnsureSecret(dir)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err := Lock(dir); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Lock: %v", err)
	}
	if _, err := Listen(dir, secret, nil); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Listen with the lock taken: %v", err)
	}
	s, err := ListenLocked(lock, secret, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lock(dir); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Lock while serving: %v", err)
	}
	s.Close()
	again, err := Lock(dir)
	if err != nil {
		t.Fatalf("Lock after Close: %v", err)
	}
	again.Close()
	if err := lock.Close(); err != nil {
		t.Fatalf("closing a released lock: %v", err)
	}
}

// A ListenLocked that fails leaves the lock with its caller, and a lock
// already released is refused instead of serving without one.
func TestListenLockedThatFailsKeepsTheLock(t *testing.T) {
	dir := shortDir(t)
	lock, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ListenLocked(lock, "not a secret", nil); err == nil {
		t.Fatal("ListenLocked accepted a bad secret")
	}
	if _, err := Lock(dir); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("the failed ListenLocked released the lock: %v", err)
	}
	lock.Close()
	secret, err := EnsureSecret(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s, err := ListenLocked(lock, secret, nil); err == nil {
		s.Close()
		t.Fatal("ListenLocked served with a released lock")
	}
}

// Lock creates the data folder, as Listen does.
func TestLockCreatesTheDataFolder(t *testing.T) {
	lock, err := Lock(filepath.Join(shortDir(t), "data"))
	if err != nil {
		t.Fatal(err)
	}
	lock.Close()
}
