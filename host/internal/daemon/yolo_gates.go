package daemon

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
)

// noRecord is what the agent is told when the journal could not be written
// and the command ran anyway.
const noRecord = "the journal cannot be written: this ran without a record and cannot be undone"

// yoloNotes collects, for one request, what YOLO let through that would have
// been asked or refused. They are told to the agent on stderr right before the
// command runs (`⚠ YOLO: …`), so it knows what the person did not get to see.
type yoloNotes struct {
	mu    sync.Mutex
	lines []string
}

type yoloNotesKey struct{}

// withYoloNotes returns ctx with a collector for one request.
func withYoloNotes(ctx context.Context) (context.Context, *yoloNotes) {
	n := &yoloNotes{}
	return context.WithValue(ctx, yoloNotesKey{}, n), n
}

func (n *yoloNotes) add(why string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, l := range n.lines {
		if l == why {
			return
		}
	}
	n.lines = append(n.lines, why)
}

// tell writes the notes, one per line.
func (n *yoloNotes) tell(w io.Writer) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, l := range n.lines {
		fmt.Fprintln(w, "⚠ YOLO: "+l)
	}
}

// lift reports whether YOLO lets the request pass a door that would have
// refused or asked, for the reason why (English, for the agent), and notes it.
// Without YOLO it is false and nothing is noted. It is asked at the door, not
// once at the start: an off from the phone takes effect at the next door.
func (d *Daemon) lift(ctx context.Context, why string) bool {
	if !d.o.Yolo.Active() {
		return false
	}
	if n, _ := ctx.Value(yoloNotesKey{}).(*yoloNotes); n != nil {
		n.add(why)
	}
	return true
}

// liftQuestion is lift for the doors that a question waiting for an answer
// closes (waitingFor, admit): text is what they say (blocks). It is false for
// a question asked through AskUser (serve's own `adbuddy remote on`, and the 🔴 inverse of an undo), which is the
// person's, not the agent's command's: while it waits, an `input tap` could
// press Allow on the phone and approve it for them. YOLO lifts the doors for
// the agent's own 🔴 questions only.
func (d *Daemon) liftQuestion(ctx context.Context, text string) bool {
	if d.waitingOnOwn() {
		return false
	}
	return d.lift(ctx, "a question was waiting for your answer ("+classify.Printable(text)+")")
}

// waitingOnOwn reports whether the question waiting is serve's own (AskUser).
func (d *Daemon) waitingOnOwn() bool {
	d.askMu.Lock()
	defer d.askMu.Unlock()
	return d.asking != nil && d.asking.own
}

// flyAnyway counts a change in flight even if a question is waiting: YOLO
// runs the command anyway, and the question (if one is ever asked) waits for
// it to end, as for any change.
func (d *Daemon) flyAnyway(change bool, title string) (release func()) {
	if !change {
		return func() {}
	}
	d.askMu.Lock()
	defer d.askMu.Unlock()
	return d.flyLocked(title)
}
