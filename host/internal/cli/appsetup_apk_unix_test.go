//go:build unix

package cli

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/paths"
)

// A FIFO with no writer blocks os.Open forever: --apk must refuse it
// without opening it (V16).
func TestChooseAPKRejectsAFIFOWithoutOpeningIt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.apk")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Skip("no FIFOs here:", err)
	}
	e := &env{dirs: paths.Dirs{Data: t.TempDir()}}
	done := make(chan error, 1)
	go func() {
		_, _, err := chooseAPK(e, p)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("err = %v, want one saying it is not a regular file", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("chooseAPK blocked: it opened the FIFO")
	}
}
