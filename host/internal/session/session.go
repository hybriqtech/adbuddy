// Package session runs the agent in a PTY inside serve (spec M3 §3): one
// session per host, its screen, its state for the phone and the size rules
// of the PC terminals and the phone.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/pty"
	"github.com/hybriqtech/adbuddy/host/internal/term"
)

// States of session.state (protocol spec §5.1).
const (
	StateIdle    = "idle"
	StateWorking = "working"
	StateWaiting = "waiting"
	StateStopped = "stopped"
)

// Default size without any terminal asking for one (v1 spec §5.1).
const (
	DefaultCols = 120
	DefaultRows = 40
)

// ErrNoAgent means there is no agent process to type into.
var ErrNoAgent = errors.New("the agent is not running")

// stopWait is how long Stop waits after the hangup before it kills the
// agent's process tree (spec M3 §3.2: 5 s). On Windows the hangup does
// nothing (pty.PTY.Hangup), so it waits less.
var stopWait = func() time.Duration {
	if runtime.GOOS == "windows" {
		return 2 * time.Second
	}
	return 5 * time.Second
}()

// Info is what the phone shows of the session.
type Info struct {
	Agent        string
	State        string
	Conversation string // changes with each process of the agent
	Epoch        string // the PTY process (protocol spec §7)
	PCAttached   bool
}

// Options configure a Manager. Look, Env and Start are required.
type Options struct {
	Dir string // the environment
	// Look finds the agent's program (exec.LookPath).
	Look func(name string) (string, error)
	// Env is the agent's environment.
	Env func(agent string) []string
	// Start starts a program in a PTY (pty.Start).
	Start func(pty.Options) (pty.PTY, error)
	// PreTrust marks the environment as trusted for the agent before it
	// starts (Claude Code, spec M3 §3.3). May be nil.
	PreTrust func(agent, dir string)
	// Quiet is how long without output turns working into waiting (4 s).
	Quiet time.Duration
	// Retry: a resumed agent that ends with an error before this starts
	// again without resuming (claude --continue with nothing to continue).
	Retry time.Duration
	// OnInfo gets the session's info each time it changes. It is called from
	// the session's goroutines, one call at a time, always with the info as
	// it is at the time of the call, and must not call back into the Manager
	// while holding its own locks.
	OnInfo func(Info)
	// Lang is the language of what the manager writes to a person's terminal
	// (the notice that the agent ended); English if empty.
	Lang i18n.Lang
}

// Output is a piece of the PTY's output, already in its screen. Two pieces
// carry no data: the first of each process (Data empty, a new Epoch: draw
// its screen) and its last (End: the process ended).
type Output struct {
	Epoch  string
	Offset int64
	Data   []byte
	End    bool
}

// Manager owns the agent's session. It is safe for concurrent use.
type Manager struct {
	o Options

	// life puts Start and Stop in a row, so there is never more than one
	// process of the agent. gen counts them: a retry from pump only starts
	// the agent when no Start or Stop came after the process that failed.
	life sync.Mutex
	gen  uint64

	mu      sync.Mutex
	cur     *proc
	info    Info
	pcs     []sized    // attached PC terminals, oldest first: the last one rules
	phone   *[2]int    // the phone's size, if it asked for one
	notify  sync.Mutex // OnInfo and the subscribers, one call at a time
	subs    map[int]func(Output)
	nextSub int
}

type sized struct {
	id   string
	size [2]int
}

type proc struct {
	p       pty.PTY
	screen  *term.Screen
	agent   string
	resumed bool
	started time.Time
	gen     uint64
	quiet   *time.Timer
	done    chan struct{}
	// grace is readGrace as it was when the process started: pump reads it
	// from its own goroutine, which a test's restore of the variable does
	// not wait for.
	grace time.Duration
	// wmu keeps a paste (text, pause, Enter) and other typing apart.
	wmu sync.Mutex
}

// New returns a Manager with no session.
func New(o Options) *Manager {
	if o.Quiet == 0 {
		o.Quiet = 4 * time.Second
	}
	if o.Retry == 0 {
		o.Retry = 3 * time.Second
	}
	if o.OnInfo == nil {
		o.OnInfo = func(Info) {}
	}
	return &Manager{o: o, info: Info{State: StateStopped}, subs: map[int]func(Output){}}
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// Start ends the current process, if any, and starts the agent. resume asks
// it to continue its last conversation (claude --continue).
func (m *Manager) Start(agent string, resume bool) error {
	m.life.Lock()
	defer m.life.Unlock()
	return m.startLocked(agent, resume)
}

// startLocked is Start with life held.
func (m *Manager) startLocked(agent string, resume bool) error {
	m.stopLocked()
	path, err := m.o.Look(agent)
	if err != nil {
		return err
	}
	if m.o.PreTrust != nil {
		m.o.PreTrust(agent, m.o.Dir)
	}
	var args []string
	if resume && agent == "claude" {
		args = []string{"--continue"}
	}
	m.mu.Lock()
	cols, rows := m.sizeLocked()
	m.mu.Unlock()
	p, err := m.o.Start(pty.Options{Path: path, Args: args, Dir: m.o.Dir, Env: m.o.Env(agent), Cols: cols, Rows: rows})
	if err != nil {
		return err
	}
	pr := &proc{p: p, agent: agent, resumed: len(args) > 0, started: time.Now(), grace: readGrace, gen: m.gen, done: make(chan struct{})}
	pr.screen = term.New(newID("e_"), cols, rows)
	pr.quiet = time.AfterFunc(time.Hour, func() { m.quietFor(pr) })
	pr.quiet.Stop()
	m.mu.Lock()
	m.cur = pr
	m.info = Info{Agent: agent, State: StateIdle, Conversation: newID("c_"), Epoch: pr.screen.Epoch(), PCAttached: len(m.pcs) > 0}
	m.applyLocked() // a terminal may have changed the size meanwhile
	m.publish(Output{Epoch: pr.screen.Epoch()})
	m.emitInfo()
	go m.pump(pr)
	return nil
}

// readGrace is how long pump keeps reading after the process ended before it
// kills what is left: the PTY ends Read by itself (pty.PTY.Read), but on Unix
// a child of the agent that keeps the terminal open may hold it.
var readGrace = 5 * time.Second

// pump reads the PTY until the process ends.
func (m *Manager) pump(pr *proc) {
	readDone := make(chan struct{})
	waitCtx, stopWaiting := context.WithCancel(context.Background())
	defer stopWaiting()
	go func() {
		if _, err := pr.p.Wait(waitCtx); err != nil {
			return // pump is done
		}
		t := time.NewTimer(pr.grace)
		defer t.Stop()
		select {
		case <-readDone:
		case <-t.C:
			pr.p.Kill() // Read returns io.EOF
		}
	}()
	buf := make([]byte, 32<<10)
	for {
		n, err := pr.p.Read(buf)
		if n > 0 {
			data := append([]byte(nil), buf[:n]...)
			off := pr.screen.Write(data)
			m.publish(Output{Epoch: pr.screen.Epoch(), Offset: off, Data: data})
			m.active(pr)
		}
		if err != nil {
			break
		}
	}
	close(readDone)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	code, _ := pr.p.Wait(ctx)
	cancel()
	pr.p.Kill()
	pr.quiet.Stop()
	m.mu.Lock()
	mine := m.cur == pr // else replaced or stopped on purpose
	retry := false
	if mine {
		retry = pr.resumed && code != 0 && time.Since(pr.started) < m.o.Retry
		m.cur = nil
		m.info.State = StateStopped
	}
	m.mu.Unlock()
	m.publish(Output{Epoch: pr.screen.Epoch(), Offset: pr.screen.Offset(), End: true})
	pr.screen.Close()
	// Stop waits for done with life held: close it before taking life.
	close(pr.done)
	if !mine {
		return
	}
	if retry {
		m.life.Lock()
		restarted := m.gen == pr.gen && m.startLocked(pr.agent, false) == nil
		m.life.Unlock()
		if restarted {
			return
		}
	}
	m.emitInfo()
}

// active marks output: working until Quiet passes without more.
func (m *Manager) active(pr *proc) {
	m.mu.Lock()
	if m.cur != pr {
		m.mu.Unlock()
		return
	}
	pr.quiet.Reset(m.o.Quiet)
	changed := m.info.State != StateWorking
	m.info.State = StateWorking
	m.mu.Unlock()
	if changed {
		m.emitInfo()
	}
}

func (m *Manager) quietFor(pr *proc) {
	m.mu.Lock()
	if m.cur != pr || m.info.State != StateWorking {
		m.mu.Unlock()
		return
	}
	m.info.State = StateWaiting
	m.mu.Unlock()
	m.emitInfo()
}

// Subscribe calls f with each piece of output from now on, until the returned
// function is called. f must not block: it is called from the PTY's reader,
// one call at a time.
func (m *Manager) Subscribe(f func(Output)) (cancel func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextSub
	m.nextSub++
	m.subs[id] = f
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.subs, id)
	}
}

// publish gives o to the subscribers. Call it without mu.
func (m *Manager) publish(o Output) {
	m.notify.Lock()
	defer m.notify.Unlock()
	m.mu.Lock()
	fs := make([]func(Output), 0, len(m.subs))
	for _, f := range m.subs {
		fs = append(fs, f)
	}
	m.mu.Unlock()
	for _, f := range fs {
		f(o)
	}
}

// emitInfo publishes the info as it is now: an older change can never be
// published after a newer one. Call it without mu.
func (m *Manager) emitInfo() {
	m.notify.Lock()
	defer m.notify.Unlock()
	m.mu.Lock()
	i := m.info
	m.mu.Unlock()
	m.o.OnInfo(i)
}

// Stop ends the agent's process, if any, and waits for it: a hangup first,
// then, after stopWait, its process tree is killed (spec M3 §3.2).
func (m *Manager) Stop() {
	m.life.Lock()
	defer m.life.Unlock()
	m.stopLocked()
}

// stopLocked is Stop with life held.
func (m *Manager) stopLocked() {
	m.gen++
	m.mu.Lock()
	pr := m.cur
	m.cur = nil
	if pr != nil {
		m.info.State = StateStopped
	}
	m.mu.Unlock()
	if pr == nil {
		return
	}
	_ = pr.p.Hangup()
	t := time.NewTimer(stopWait)
	select {
	case <-pr.done:
	case <-t.C:
		pr.p.Kill()
		<-pr.done
	}
	t.Stop()
	m.emitInfo()
}

// Info is the session's current info.
func (m *Manager) Info() Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.info
}

// Screen is the current process's screen, or nil without one.
func (m *Manager) Screen() *term.Screen {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cur == nil {
		return nil
	}
	return m.cur.screen
}

func (m *Manager) current() *proc {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cur
}

// Input types bytes into the agent's terminal, as they are.
func (m *Manager) Input(p []byte) error {
	pr := m.current()
	if pr == nil {
		return ErrNoAgent
	}
	pr.wmu.Lock()
	defer pr.wmu.Unlock()
	_, err := pr.p.Write(p)
	return err
}

// pasteGap separates the pasted text from the Enter that sends it.
var pasteGap = 100 * time.Millisecond

// Paste sends text as one message (spec M3 §5.1): bracketed paste when the
// agent turned it on, else one line; then Enter. Control characters are
// dropped first, so the text cannot end the paste early (ESC[201~) and type
// the rest as keys. Nothing else is typed between the text and its Enter.
func (m *Manager) Paste(text string) error {
	pr := m.current()
	if pr == nil {
		return ErrNoAgent
	}
	text = clean(text)
	pr.wmu.Lock()
	defer pr.wmu.Unlock()
	var msg string
	if pr.screen.BracketedPaste() {
		msg = "\x1b[200~" + text + "\x1b[201~"
	} else {
		msg = strings.Join(strings.Fields(strings.ReplaceAll(text, "\n", " ")), " ")
	}
	if _, err := pr.p.Write([]byte(msg)); err != nil {
		return err
	}
	time.Sleep(pasteGap)
	_, err := pr.p.Write([]byte("\r"))
	return err
}

// Clean is the cleaning Paste applies to its text: what is left of text when
// the control characters go.
func Clean(s string) string { return clean(s) }

// clean keeps text, new lines and tabs; any other control character goes.
func clean(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
}

// busyPrompts are questions of Claude Code that a blind Enter from the chat
// would answer: the folder trust dialog (its default exits, spec M3 §3.3)
// and the permission dialogs (their default approves).
var busyPrompts = []string{
	"Do you trust the files in this folder",
	"Is this a project you created or one you trust",
	"Do you want to proceed?",
	"Do you want to make this edit",
	"Do you want to create",
}

// Busy reports why a chat message must not be typed now: the agent shows a
// question that the message's Enter would answer.
func (m *Manager) Busy() string {
	pr := m.current()
	if pr == nil {
		return ""
	}
	text := pr.screen.Text()
	for _, p := range busyPrompts {
		if strings.Contains(text, p) {
			return "Answer the question in the agent's terminal first."
		}
	}
	return ""
}
