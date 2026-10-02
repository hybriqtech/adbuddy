package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// What is typed into the agent goes through a queue to one writer per process
// (V08): the PTY's Write blocks when the agent does not read its input, and
// whoever types (the phone channel's only reader among them) must never wait
// for it. The queue is never closed: a process that ends closes its stopped
// channel instead, and what is still queued is dropped.

// writeQueue is how many pieces of typing wait for the agent's terminal.
const writeQueue = 16

// writeWait is how long Paste waits in all, from its call.
var writeWait = 3 * time.Second

// pasteGap separates the pasted text from the Enter that sends it.
var pasteGap = 100 * time.Millisecond

// ErrBusy means the agent is not reading its input: what was to be typed
// was not typed.
var ErrBusy = errors.New("the agent is not reading its input")

// ErrDropped is the ErrBusy that starts an episode of typing dropped by a
// Typer: errors.Is(ErrDropped, ErrBusy). The ones after it in the same
// episode are plain ErrBusy, so a caller can tell the person once.
var ErrDropped = fmt.Errorf("%w: typing dropped", ErrBusy)

// BusyError is a Paste that its guard, or the agent's screen, stopped: Why
// says why (English, protocol text). errors.Is(err, ErrBusy) holds.
type BusyError struct{ Why string }

func (e *BusyError) Error() string        { return e.Why }
func (e *BusyError) Is(target error) bool { return target == ErrBusy }

// States of a writeJob.
const (
	jobPending   int32 = iota
	jobStarted         // the writer took it: it is being typed
	jobCancelled       // its Paste gave up: it is never typed
)

// writeJob is one piece of typing: bytes as they are, or a paste.
type writeJob struct {
	gen   uint64 // the process it is for
	data  []byte // Input and Wait
	paste bool
	text  string        // a paste's text, already cleaned
	guard func() string // a paste's: why its text or Enter must not go now
	state atomic.Int32
	// done gets a paste's result. Its capacity of 1 lets the writer leave
	// the result of a Paste that already returned (writeWait) and go on.
	done chan error
}

// enqueue puts j in the process's queue without waiting: ErrNoAgent once the
// process is stopping, ErrBusy when the queue is full. drains is the
// writer's count of emptied queues at that moment, read under qmu like its
// increments: a later count means the writer emptied the queue afterwards.
func (pr *proc) enqueue(j *writeJob) (drains uint64, err error) {
	select {
	case <-pr.stopped:
		return 0, ErrNoAgent
	default:
	}
	pr.qmu.Lock()
	defer pr.qmu.Unlock()
	// stopped is closed and the queue has room: the select would choose at
	// random, hence the check above. A stop between them may still let j in:
	// the writer is gone or going, and never types it.
	select {
	case <-pr.stopped:
		return pr.drains, ErrNoAgent
	case pr.q <- j:
		return pr.drains, nil
	default:
		return pr.drains, ErrBusy
	}
}

// stop closes stopped, once: the writer drops what is queued, and nothing
// more gets in.
func (pr *proc) stop() { pr.stopOnce.Do(func() { close(pr.stopped) }) }

// isStopped reports whether stopped is closed.
func (pr *proc) isStopped() bool {
	select {
	case <-pr.stopped:
		return true
	default:
		return false
	}
}

// writer is the only caller of the PTY's Write for this process. It holds
// no lock while it types (qmu only to count an emptied queue), and nothing
// waits for it to end: Stop and pump go on while it is stuck in a Write,
// which the process's Kill ends (pty.PTY.Kill).
func (pr *proc) writer() {
	defer close(pr.wdone)
	for {
		if pr.isStopped() {
			pr.dropQueued()
			return
		}
		var j *writeJob
		select {
		case <-pr.stopped:
			pr.dropQueued()
			return
		case j = <-pr.q:
		}
		if pr.isStopped() || j.gen != pr.gen {
			// A job and a stop together: the stop wins.
			pr.fail(j, ErrNoAgent)
			pr.dropQueued()
			return
		}
		if j.state.CompareAndSwap(jobPending, jobStarted) {
			var err error
			if j.paste {
				err = pr.typePaste(j)
			} else {
				_, err = pr.p.Write(j.data)
			}
			if j.done != nil {
				j.done <- err
			}
		} // else cancelled: its Paste gave up, it is never typed
		pr.qmu.Lock()
		if len(pr.q) == 0 {
			pr.drains++
		}
		pr.qmu.Unlock()
	}
}

// fail ends j untyped with err.
func (pr *proc) fail(j *writeJob, err error) {
	j.state.CompareAndSwap(jobPending, jobCancelled)
	if j.done != nil {
		select {
		case j.done <- err:
		default:
		}
	}
}

// dropQueued fails what is left in the queue with ErrNoAgent.
func (pr *proc) dropQueued() {
	for {
		select {
		case j := <-pr.q:
			pr.fail(j, ErrNoAgent)
		default:
			return
		}
	}
}

// typePaste types a paste: its text, a pause, its Enter. The guard and the
// screen are asked before the text and again before the Enter; the choice of
// bracketed paste or one line is made now, not when it was queued.
func (pr *proc) typePaste(j *writeJob) error {
	if why := pr.busyFor(j.guard); why != "" {
		return &BusyError{Why: why}
	}
	var msg string
	if pr.screen.BracketedPaste() {
		msg = "\x1b[200~" + j.text + "\x1b[201~"
	} else {
		msg = strings.Join(strings.Fields(strings.ReplaceAll(j.text, "\n", " ")), " ")
	}
	if _, err := pr.p.Write([]byte(msg)); err != nil {
		return err
	}
	t := time.NewTimer(pr.gap)
	select {
	case <-t.C:
	case <-pr.stopped:
		t.Stop()
		return ErrNoAgent
	}
	if pr.isStopped() {
		return ErrNoAgent
	}
	// A dialog that came while the text was typed: its Enter would answer
	// it. The text stays in the agent's input, not sent (fail safe).
	if why := pr.busyFor(j.guard); why != "" {
		return &BusyError{Why: why}
	}
	_, err := pr.p.Write([]byte("\r"))
	return err
}

// busyFor is why a paste must not be typed now: its guard's reason, else a
// question on this process's screen.
func (pr *proc) busyFor(guard func() string) string {
	if guard != nil {
		if why := guard(); why != "" {
			return why
		}
	}
	return busyOn(pr.screen)
}

// Paste sends text as one message (spec M3 §5.1): bracketed paste when the
// agent turned it on, else one line; then Enter. Control characters are
// dropped first, so the text cannot end the paste early (ESC[201~) and type
// the rest as keys. It is one job of the writer: nothing else is typed
// between the text and its Enter.
//
// guard (may be nil) is asked by the writer before the text and before the
// Enter, together with the agent's screen (Busy); when either says busy the
// paste stops with a *BusyError and its Enter is not typed. If that happens
// after the text, the text stays in the agent's input box, not sent.
//
// Paste waits writeWait at most in all. A paste still queued then is never
// typed and Paste returns ErrBusy; ErrBusy at once when the queue is full.
// One the writer already started cannot be undone: Paste returns nil without
// waiting for it to end (if its guard then stops the Enter, the text stays
// in the agent's input box and goes with the next message).
func (m *Manager) Paste(text string, guard func() string) error {
	wait := writeWait
	pr := m.current()
	if pr == nil {
		return ErrNoAgent
	}
	j := &writeJob{gen: pr.gen, paste: true, text: clean(text), guard: guard, done: make(chan error, 1)}
	if _, err := pr.enqueue(j); err != nil {
		return err
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	stopped := pr.stopped
	for {
		select {
		case err := <-j.done:
			return err
		case <-stopped:
			if j.state.CompareAndSwap(jobPending, jobCancelled) {
				return ErrNoAgent
			}
			// Started: the writer sees the stop before the Enter, or is
			// stuck in a Write that the Kill ends. Its result, or the time.
			stopped = nil
		case <-t.C:
			select {
			case err := <-j.done:
				return err
			default:
			}
			if j.state.CompareAndSwap(jobPending, jobCancelled) {
				return ErrBusy
			}
			if pr.isStopped() {
				return ErrNoAgent
			}
			// Started and not done: the text is being typed and cannot be
			// taken back. Not waiting for it is what keeps the caller free.
			return nil
		}
	}
}

// Typer types for one source of input (the phone, a PC terminal). Each
// source has its own: after Input drops a piece of typing because the agent
// is not reading, the same Typer drops all its Input until the writer has
// emptied the queue, so that the rest of a key sequence (ESC lost, then
// "[A") or a lone Enter is not typed on its own. A new process of the agent
// lifts it. Safe for concurrent use.
type Typer struct {
	m  *Manager
	mu sync.Mutex
	// dropped is the process where Input last dropped, until its writer
	// empties the queue: drains is its count of emptied queues then.
	dropped *proc
	drains  uint64
}

// NewTyper returns a Typer for one source of input.
func (m *Manager) NewTyper() *Typer { return &Typer{m: m} }

// Input types p into the agent's terminal, as it is, without waiting: p is
// queued whole or dropped whole. ErrNoAgent without an agent; ErrDropped
// when it drops p and starts an episode of drops; ErrBusy when it drops p
// within one (errors.Is(err, ErrBusy) for both).
func (t *Typer) Input(p []byte) error {
	pr := t.m.current()
	if pr == nil {
		return ErrNoAgent
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dropped == pr {
		pr.qmu.Lock()
		emptied := pr.drains != t.drains
		pr.qmu.Unlock()
		if !emptied && !pr.isStopped() {
			return ErrBusy
		}
	}
	t.dropped = nil
	drains, err := pr.enqueue(&writeJob{gen: pr.gen, data: append([]byte(nil), p...)})
	if errors.Is(err, ErrBusy) {
		t.dropped, t.drains = pr, drains
		return ErrDropped
	}
	return err
}

// Wait types p into the agent's terminal, as it is, waiting for room in the
// queue if needed: what a PC terminal types, which the person in front of
// the PC sends and which is never dropped. It returns once p is queued, or
// with ErrNoAgent when there is no agent or it stops, or ctx's error.
func (t *Typer) Wait(ctx context.Context, p []byte) error {
	pr := t.m.current()
	if pr == nil || pr.isStopped() {
		return ErrNoAgent
	}
	j := &writeJob{gen: pr.gen, data: append([]byte(nil), p...)}
	select {
	case <-pr.stopped:
		return ErrNoAgent
	case <-ctx.Done():
		return ctx.Err()
	case pr.q <- j:
		return nil
	}
}
