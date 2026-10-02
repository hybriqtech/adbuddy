//go:build unix

package safeio

import (
	"errors"
	"fmt"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO in the folder is an error, never an open that waits for a writer:
// also when the open itself meets it (the Lstat before it can be raced).
func TestFifoInRootDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "2026-10.jsonl"), 0o600); err != nil {
		t.Skipf("no fifo: %v", err)
	}
	r, err := OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	done := make(chan error, 1)
	go func() {
		for _, flag := range []int{ReadFlags, AppendFlags} {
			f, err := r.OpenFile("2026-10.jsonl", flag, 0o600)
			if err != nil {
				done <- fmt.Errorf("open %#x: %v", flag, err)
				return
			}
			err = checkOpened(f, "2026-10.jsonl")
			f.Close()
			if !errors.Is(err, ErrNotRegular) {
				done <- fmt.Errorf("open %#x: %v, want ErrNotRegular", flag, err)
				return
			}
		}
		if _, err := OpenRegularIn(r, "2026-10.jsonl"); !errors.Is(err, ErrNotRegular) {
			done <- fmt.Errorf("OpenRegularIn: %v", err)
			return
		}
		if _, err := OpenAppendIn(r, "2026-10.jsonl", 0o600); !errors.Is(err, ErrNotRegular) {
			done <- fmt.Errorf("OpenAppendIn: %v", err)
			return
		}
		done <- nil
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("hangs on a FIFO")
	}
}
