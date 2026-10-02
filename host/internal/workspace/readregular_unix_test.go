//go:build unix

package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// initWithin runs Init and fails the test if it does not return in time: a
// FIFO opened for reading blocks until someone writes to it.
func initWithin(t *testing.T, o Options) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := Init(o)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(20 * time.Second):
		t.Fatal("Init hangs")
		return nil
	}
}

// plant replaces path, after a first Init, with a FIFO or with a link to
// /dev/zero; it skips the test if this system does not allow it.
func plant(t *testing.T, path, kind string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	var err error
	switch kind {
	case "fifo":
		err = syscall.Mkfifo(path, 0o600)
	case "devzero":
		err = os.Symlink("/dev/zero", path)
	}
	if err != nil {
		t.Skipf("cannot plant a %s: %v", kind, err)
	}
}

// A FIFO, or a link to /dev/zero, where a settings file goes is an error that
// tells the user to fix it, not a read that hangs; and it is left as it was
// (V14).
func TestInitDoesNotHangOnSettingsThatAreNotARegularFile(t *testing.T) {
	for _, rel := range []string{filepath.Join(".claude", "settings.json"), filepath.Join(".gemini", "settings.json")} {
		for _, kind := range []string{"fifo", "devzero"} {
			t.Run(kind+"/"+rel, func(t *testing.T) {
				dir := t.TempDir()
				exe := fakeExe(t, "v1")
				if _, err := Init(Options{Dir: dir, Exe: exe}); err != nil {
					t.Fatal(err)
				}
				p := filepath.Join(dir, rel)
				plant(t, p, kind)
				if err := initWithin(t, Options{Dir: dir, Exe: exe}); err == nil || !strings.Contains(err.Error(), "not a regular file") {
					t.Fatalf("error %v, want one that says it is not a regular file", err)
				}
				if st, err := os.Lstat(p); err != nil || st.Mode().IsRegular() {
					t.Fatalf("what was planted was replaced: %v %v", st, err)
				}
			})
		}
	}
}

// A FIFO, or a link to /dev/zero, where the shim goes is not read: it is
// replaced by the shim, so serve and run still start (V14).
func TestInitReplacesAShimThatIsNotARegularFile(t *testing.T) {
	for _, kind := range []string{"fifo", "devzero"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			exe := fakeExe(t, "v1")
			if _, err := Init(Options{Dir: dir, Exe: exe}); err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(BinDir(dir), "adb")
			plant(t, p, kind)
			if err := initWithin(t, Options{Dir: dir, Exe: exe}); err != nil {
				t.Fatal(err)
			}
			if st, err := os.Lstat(p); err != nil || !st.Mode().IsRegular() || read(t, p) != "v1" {
				t.Fatalf("the shim was not put back: %v %v", st, err)
			}
		})
	}
}

// readRegular and hasUserMarker do not block on a FIFO even when it is the one
// opened (the Lstat before can be raced): the open does not block.
func TestReadRegularDoesNotHangOnAFifo(t *testing.T) {
	p := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Skipf("no fifo: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		// The open itself, as if the Lstat before it had been raced.
		f, err := os.OpenFile(p, openFlags, 0)
		if err != nil {
			done <- err
			return
		}
		f.Close()
		if _, err := readRegular(p); !errors.Is(err, errNotRegular) {
			done <- fmt.Errorf("readRegular: %v, want errNotRegular", err)
			return
		}
		if hasUserMarker(p) {
			done <- errors.New("a FIFO has the marker")
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
	if st, err := os.Lstat(p); err != nil || st.Mode()&fs.ModeNamedPipe == 0 {
		t.Fatalf("the FIFO changed: %v %v", st, err)
	}
}
