//go:build unix

package claudecode

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO without writer must not stop the owner: neither Poll nor the open
// that follows the Lstat waits for one.
func TestTailerDoesNotBlockOnAFifo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	write(t, path, os.O_TRUNC, textLine("u1", "m1", "uno"))
	tl := tailerAt(path, false)
	if ev, _ := tl.Poll(); len(ev) != 1 {
		t.Fatal(ev)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skip("no FIFOs:", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if ev, err := tl.Poll(); err == nil || len(ev) != 0 {
			t.Errorf("a FIFO was read: %v %v", ev, err)
		}
		// The swap between Lstat and open: the open itself must return.
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Error(err)
			return
		}
		defer root.Close()
		f, err := openTail(root, "s.jsonl")
		if err != nil {
			t.Error(err)
			return
		}
		defer f.Close()
		if fi, err := f.Stat(); err != nil || fi.Mode().IsRegular() {
			t.Errorf("%v %v", fi, err)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("blocked on a FIFO without writer")
	}
}
