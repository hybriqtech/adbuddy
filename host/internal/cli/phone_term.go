package cli

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/session"
)

// The agent's terminal on the phone (spec M3 §4.1 and §4.3, protocol spec §7).
// Neither what the terminal shows nor what is typed goes to any log or error.

// Limits of the terminal messages (protocol spec §9) and of what the host
// sends.
const (
	maxTermInput = 64 << 10 // terminal.input.data
	termChunk    = 64 << 10 // terminal.output.data
	// maxTermSide is the most the protocol accepts for cols or rows of a
	// terminal.resize; the host takes less (termCols, termRows) so that a
	// terminal.snapshot fits in a message of the bridge (1 MiB).
	maxTermSide = 1000
	termCols    = 200
	termRows    = 100
	// maxTermPending is how much output waits for the next tick before the
	// screen is sent whole instead.
	maxTermPending = 1 << 20
)

var (
	// termInterval is how often the output is grouped and sent (50 ms).
	termInterval = 50 * time.Millisecond
	// termBacklogLimit is the queue of the app (256 messages) past which the
	// terminal stops sending: half of it (protocol spec §7).
	termBacklogLimit = 128
	// backlogOf is the app's queue, a variable so that a test can raise it.
	backlogOf = func(s *bridge.Server) int { return s.Backlog() }
)

// termState is the terminal subscription of the connected app: at most one.
// Two locks: mu guards w and the inbox of each watch, and is only held to
// copy; send serializes the sending and the state only that code touches, and
// is held while calling the Manager and the bridge server (mu never is).
type termState struct {
	mu    sync.Mutex
	w     *termWatch
	unsub func() // the Manager subscription

	send sync.Mutex
}

// termWatch is what one terminal.watch keeps. msg is the request it came on:
// SendTo takes the connection from it.
type termWatch struct {
	msg  bridge.Message
	done chan struct{}

	// Guarded by termState.mu.
	inbox      []session.Output
	inboxBytes int
	overflow   bool

	// Guarded by termState.send.
	epoch      string // the process the app has the screen of; "" when none
	next       int64  // the offset of the next byte the app is owed
	snapDue    bool   // a terminal.snapshot goes out as soon as it can
	buf        []byte // output accepted and not yet sent
	bufOff     int64
	cols, rows int
	pressure   bool // the queue was pressed since the last snapshot
}

// subscribeTerm takes the one subscription to the session's output (useSession
// gives the Manager). What it hears goes nowhere without a watch.
func (l *phoneLink) subscribeTerm(m *session.Manager) {
	var cancel func()
	if m != nil {
		cancel = m.Subscribe(l.onTermOutput)
	}
	l.term.mu.Lock()
	old := l.term.unsub
	l.term.unsub = cancel
	l.term.mu.Unlock()
	if old != nil {
		old()
	}
}

// onTermOutput is the Manager's callback: it only copies, under the mutex.
func (l *phoneLink) onTermOutput(o session.Output) {
	if o.End {
		return // nothing to show: the next process makes a snapshot
	}
	l.term.mu.Lock()
	defer l.term.mu.Unlock()
	w := l.term.w
	if w == nil || w.overflow {
		return
	}
	w.inboxBytes += len(o.Data)
	if w.inboxBytes > maxTermPending {
		w.inbox, w.overflow = nil, true
		return
	}
	w.inbox = append(w.inbox, o)
}

// stopTerm ends the subscription, if any (unwatch, a new connection or none).
func (l *phoneLink) stopTerm() {
	l.term.mu.Lock()
	defer l.term.mu.Unlock()
	l.endWatchLocked(nil)
}

// endWatchLocked ends w, or the current watch when w is nil.
func (l *phoneLink) endWatchLocked(w *termWatch) {
	if l.term.w == nil || w != nil && l.term.w != w {
		return
	}
	close(l.term.w.done)
	l.term.w = nil
}

func (l *phoneLink) watching(w *termWatch) bool {
	l.term.mu.Lock()
	defer l.term.mu.Unlock()
	return l.term.w == w
}

// onTermWatch subscribes the connection that asked: what it lacks since its
// epoch and offset, or the whole screen.
func (l *phoneLink) onTermWatch(m bridge.Message) {
	var tw bridge.TerminalWatch
	if json.Unmarshal(m.Payload, &tw) != nil || tw.Offset < 0 {
		return // it carries no rid: there is nobody to tell
	}
	// A watch read before a replacement can arrive after it.
	if l.srv == nil || !l.srv.Current(m) {
		return
	}
	l.term.send.Lock()
	defer l.term.send.Unlock()
	w := &termWatch{msg: m, done: make(chan struct{})}
	l.term.mu.Lock()
	l.endWatchLocked(nil)
	l.term.w = w // from here on the live output is kept in its inbox
	l.term.mu.Unlock()
	// The connection may have been replaced meanwhile (OnAttach ran before w
	// was installed, so nothing would end it).
	if !l.srv.Current(m) {
		l.term.mu.Lock()
		l.endWatchLocked(w)
		l.term.mu.Unlock()
		return
	}

	w.snapDue = true // unless the app only lacks what the tail has
	if mgr := l.session(); mgr != nil {
		if scr := mgr.Screen(); scr != nil {
			if data, ok := scr.Since(tw.Epoch, tw.Offset); ok {
				w.snapDue = false
				w.epoch, w.next = scr.Epoch(), tw.Offset+int64(len(data))
				w.cols, w.rows = scr.Size()
				w.buf, w.bufOff = data, tw.Offset
				l.flushTerm(w)
				if !w.snapDue {
					// It resumes without a snapshot, so it learns the size to
					// draw with: it may have changed while it was away.
					l.sendSize(w, w.next)
				}
			}
		}
	}
	// Without an agent nothing goes out: its snapshot does when it starts.
	if w.snapDue {
		l.snapshotTerm(w)
	}
	go l.termLoop(w)
}

func (l *phoneLink) termLoop(w *termWatch) {
	every := l.termEvery
	if every <= 0 {
		every = 50 * time.Millisecond
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-t.C:
			l.termTick(w)
		}
	}
}

// termTick takes what arrived since the last tick and sends it.
func (l *phoneLink) termTick(w *termWatch) {
	l.term.send.Lock()
	defer l.term.send.Unlock()
	if !l.watching(w) {
		return
	}
	// The Manager does not tell of size changes: look at each tick, before
	// the inbox, so that the offset is not past what the inbox may hold.
	var cols, rows int
	var off int64
	var epoch string
	if mgr := l.session(); mgr != nil {
		if scr := mgr.Screen(); scr != nil {
			cols, rows = scr.Size()
			off, epoch = scr.Offset(), scr.Epoch()
		}
	}
	l.takeTerm(w)
	if !w.snapDue {
		l.flushTerm(w)
	}
	if w.snapDue {
		l.snapshotTerm(w)
		return
	}
	// Only after the output before it went out.
	if cols != 0 && epoch == w.epoch && (cols != w.cols || rows != w.rows) {
		w.cols, w.rows = cols, rows
		l.sendSize(w, off)
		if w.snapDue {
			l.snapshotTerm(w)
		}
	}
}

// sendSize tells the app the size w holds, from byte offset on. With the
// app's queue pressed it asks for a snapshot instead, which carries the size.
func (l *phoneLink) sendSize(w *termWatch, offset int64) {
	if l.pressed(w) {
		w.snapDue, w.buf = true, nil
		return
	}
	l.sendTerm(w, bridge.TypeTerminalSize, bridge.TerminalSize{Epoch: w.epoch, Offset: offset, Cols: w.cols, Rows: w.rows})
}

// takeTerm moves the inbox into the pending output, as session.ServeAttach
// does: what is below next already reached the app; a gap or another process
// asks for the whole screen. Call it with send held.
func (l *phoneLink) takeTerm(w *termWatch) {
	l.term.mu.Lock()
	in, overflow := w.inbox, w.overflow
	w.inbox, w.inboxBytes, w.overflow = nil, 0, false
	l.term.mu.Unlock()
	if overflow {
		w.snapDue, w.buf = true, nil
	}
	for _, o := range in {
		if w.snapDue {
			break // the snapshot is taken after all of these
		}
		if o.Epoch != w.epoch {
			// Another process (its first piece has no data).
			w.snapDue, w.buf = true, nil
			break
		}
		data, at := o.Data, o.Offset
		if len(data) == 0 || at+int64(len(data)) <= w.next {
			continue
		}
		if at < w.next {
			data, at = data[w.next-at:], w.next
		}
		if at > w.next || len(w.buf)+len(data) > maxTermPending {
			w.snapDue, w.buf = true, nil
			break
		}
		if len(w.buf) == 0 {
			w.bufOff = at
		}
		w.buf = append(w.buf, data...)
		w.next = at + int64(len(data))
	}
}

// sendTerm sends a stream message to the connection that watches. That
// connection being gone ends the watch.
func (l *phoneLink) sendTerm(w *termWatch, typ string, payload any) bool {
	if err := l.srv.SendTo(w.msg, typ, payload); err != nil {
		if errors.Is(err, bridge.ErrNoApp) {
			l.term.mu.Lock()
			l.endWatchLocked(w)
			l.term.mu.Unlock()
		}
		return false
	}
	return true
}

// backlogLen is the app's queue; a link made without startPhoneLink (a test's
// bare one) asks the server.
func (l *phoneLink) backlogLen() int {
	if l.backlog == nil {
		return l.srv.Backlog()
	}
	return l.backlog(l.srv)
}

// pressed reports whether the app's queue is past the limit, and remembers it
// in w: after a pressure the snapshot waits for the queue to ease to half the
// limit, so that it does not sit at the edge.
func (l *phoneLink) pressed(w *termWatch) bool {
	if l.backlogLen() > termBacklogLimit {
		w.pressure = true
		return true
	}
	return false
}

// maxSnapshotMessage is the most a terminal.snapshot message may weigh: the
// app takes 1 MiB, and a PC terminal can make the screen bigger than the one
// the phone asks for.
const maxSnapshotMessage = 900 << 10

// screenSnapshot is the screen as a terminal.snapshot, without styles when
// the styled one would not fit in a message.
func screenSnapshot(scr interface {
	Snapshot() ([]byte, int64, int, int)
	PlainSnapshot(int) ([]byte, int64, int, int)
	Epoch() string
}) bridge.TerminalSnapshot {
	data, off, cols, rows := scr.Snapshot()
	snap := bridge.TerminalSnapshot{Epoch: scr.Epoch(), Offset: off, Cols: cols, Rows: rows, Data: data}
	if raw, err := json.Marshal(snap); err == nil && len(raw) <= maxSnapshotMessage {
		return snap
	}
	// base64 takes 4 bytes for 3, and the envelope and fields some more.
	snap.Data, snap.Offset, snap.Cols, snap.Rows = scr.PlainSnapshot(maxSnapshotMessage/4*3 - 1024)
	return snap
}

// flushTerm sends the pending output in chunks. With the app's queue pressed
// it drops it and asks for a snapshot instead: the terminal never fills the
// queue whose overflow drops the app.
func (l *phoneLink) flushTerm(w *termWatch) {
	for len(w.buf) > 0 {
		if l.pressed(w) {
			w.snapDue, w.buf = true, nil
			return
		}
		n := min(len(w.buf), termChunk)
		if !l.sendTerm(w, bridge.TypeTerminalOutput, bridge.TerminalOutput{Epoch: w.epoch, Offset: w.bufOff, Data: w.buf[:n]}) {
			w.buf = nil
			return
		}
		w.buf, w.bufOff = w.buf[n:], w.bufOff+int64(n)
	}
	w.buf = nil
}

// snapshotTerm sends the screen when it can: there is an agent and the app's
// queue is not pressed (and, after a pressure, is down to half the limit).
// Otherwise it stays due.
func (l *phoneLink) snapshotTerm(w *termWatch) {
	mgr := l.session()
	if mgr == nil || l.pressed(w) {
		return
	}
	if w.pressure && l.backlogLen() > termBacklogLimit/2 {
		return // hysteresis
	}
	scr := mgr.Screen()
	if scr == nil {
		return
	}
	snap := screenSnapshot(scr)
	if !l.sendTerm(w, bridge.TypeTerminalSnapshot, snap) {
		return
	}
	w.snapDue, w.buf, w.pressure = false, nil, false
	w.epoch, w.next, w.cols, w.rows = snap.Epoch, snap.Offset, snap.Cols, snap.Rows
}

func (l *phoneLink) onTermUnwatch(m bridge.Message) {
	if l.srv != nil && l.srv.Current(m) {
		l.stopTerm()
	}
}

// onTermInput types what the person typed on the phone into the agent.
func (l *phoneLink) onTermInput(m bridge.Message) {
	var in bridge.TerminalInput
	if json.Unmarshal(m.Payload, &in) != nil || len(in.Data) == 0 || len(in.Data) > maxTermInput {
		return // no rid: dropped
	}
	if mgr := l.session(); mgr != nil {
		_ = mgr.Input(in.Data)
	}
}

// onTermResize gives the agent the size that fits the phone's view: only
// ruling while no PC terminal is attached (Manager.ResizePhone).
func (l *phoneLink) onTermResize(m bridge.Message) {
	var rs bridge.TerminalResize
	if json.Unmarshal(m.Payload, &rs) != nil || rs.Cols < 1 || rs.Cols > maxTermSide || rs.Rows < 1 || rs.Rows > maxTermSide {
		return
	}
	if mgr := l.session(); mgr != nil {
		mgr.ResizePhone(min(rs.Cols, termCols), min(rs.Rows, termRows))
	}
}
