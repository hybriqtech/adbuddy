//go:build unix

package workspace

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO where a skill file goes must be replaced, not read: reading it would
// hang Init.
func TestForceReplacesAFifoWhereASkillFileGoes(t *testing.T) {
	dir := t.TempDir()
	exe := fakeExe(t, "v1")
	if _, err := Init(Options{Dir: dir, Exe: exe}); err != nil {
		t.Fatal(err)
	}
	first := filepath.FromSlash(skillFiles(t)[0])
	target := filepath.Join(dir, ".claude", "skills", first)
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(target, 0o600); err != nil {
		t.Skipf("no fifo: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := Init(Options{Dir: dir, Exe: exe, Force: true})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Init hangs on a FIFO")
	}
	want, _ := fs.ReadFile(Skills(), filepath.ToSlash(first))
	if st, err := os.Lstat(target); err != nil || !st.Mode().IsRegular() || read(t, target) != string(want) {
		t.Fatalf("the FIFO was not replaced by the embedded file: %v %v", st, err)
	}
}
