package cli

import (
	"os"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
)

// V19: a FIFO put in place of the file once Lstat found it regular (between
// Lstat and open) does not block open, and with it the handler and the
// shutdown of serve: the file is opened without waiting, and refused once
// open because it is not the regular file that was checked.
func TestSendFIFOSwappedInBeforeOpen(t *testing.T) {
	if !fifosExist {
		t.Skip("Windows has no FIFOs in the file system; its residual is the junction window between ReparseBelow and Lstat (see openInEnvironment)")
	}
	l, ws := bareLink(t)
	p := writeIn(t, ws, "a.txt", []byte("x"))
	swapped := false
	setVar(t, &beforeOpen, func() {
		if swapped {
			return
		}
		swapped = true
		if err := os.Remove(p); err != nil {
			t.Error(err)
			return
		}
		if err := makeFIFO(p); err != nil {
			t.Errorf("mkfifo: %v", err)
		}
	})
	done := make(chan ipc.Exit, 1)
	go func() {
		_, e := l.sendFile(ws, sendReq(p, ""))
		done <- e
	}()
	select {
	case e := <-done:
		wantSendError(t, e, "SEND_FAILED", "changed while it was being opened")
	case <-time.After(5 * time.Second):
		// A FIFO opened to read and write never waits: it lets the blocked
		// open go on, so that nothing is left running after the test.
		if w, err := os.OpenFile(p, os.O_RDWR, 0); err == nil {
			defer w.Close()
		}
		<-done
		t.Fatal("sendFile blocked on a FIFO put in place of the file")
	}
	if !swapped {
		t.Fatal("the file was never swapped")
	}
	if got := outboxNames(t, ws); len(got) != 0 {
		t.Fatalf("copied %v", got)
	}
}
