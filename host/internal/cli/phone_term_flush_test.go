package cli

import (
	"errors"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// V41: when sending output fails for a reason other than the app being gone,
// the pending output is dropped and a snapshot is due, never kept to grow.
func TestFlushTermFailureAsksForASnapshot(t *testing.T) {
	r := termRig(t, 50*time.Millisecond)
	r.l.term.sendFn = func(bridge.Message, string, any) error { return errors.New("boom") }
	w := &termWatch{buf: []byte("pending output"), bufOff: 5}
	r.l.flushTerm(w)
	if !w.snapDue || w.buf != nil {
		t.Fatalf("snapDue %v, buf %q", w.snapDue, w.buf)
	}
}
