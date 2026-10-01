//go:build unix

package cli

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO blocks open, and with it the shutdown of serve: it is refused
// before opening.
func TestSendFIFO(t *testing.T) {
	l, ws := bareLink(t)
	p := filepath.Join(ws, "tuberia")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, e := l.sendFile(ws, sendReq(p, ""))
		wantSendError(t, e, "NOT_A_FILE", "not a regular file")
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("sendFile blocked on the FIFO")
	}
}
