// Package session runs the agent in a PTY inside serve (spec M3 §3): one
// session per host, its screen, its state for the phone and the size rules
// of the PC terminals and the phone.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

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
	// the session's goroutines, one call at a time, and must not call back
	// into the Manager while holding its own locks.
	OnInfo func(Info)
}

// Output is a piece of the PTY's output, already in its screen.
type Output struct {
	Epoch  string
	Offset int64
	Data   []byte
}

// Manager owns the agent's session. It is safe for concurrent use.
type Manager struct {
	o Options

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
	quiet   *time.Timer
	done    chan struct{}
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
	m.Stop()
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
	pr := &proc{p: p, agent: agent, resumed: len(args) > 0, started: time.Now(), done: make(chan struct{})}
	pr.screen = term.New(newID("e_"), cols, rows)
	m.mu.Lock()
	m.cur = pr
	m.info = Info{Agent: agent, State: StateIdle, Conversation: newID("c_"), Epoch: pr.screen.Epoch(), PCAttached: len(m.pcs) > 0}
	pr.quiet = time.AfterFunc(time.Hour, func() { m.quietFor(pr) })
	pr.quiet.Stop()
	info := m.info
	m.mu.Unlock()
	m.emitInfo(info)
	go m.pump(pr)
	return nil
}

// pump reads the PTY until the process ends.
func (m *Manager) pump(pr *proc) {
	buf := make([]byte, 32<<10)
	for {
		n, err := pr.p.Read(buf)
		if n > 0 {
			data := append([]byte(nil), buf[:n]...)
			off := pr.screen.Write(data)
			out := Output{Epoch: pr.screen.Epoch(), Offset: off, Data: data}
			m.notify.Lock()
			for _, f := range m.subscribers() {
				f(out)
			}
			m.notify.Unlock()
			m.active(pr)
		}
		if err != nil {
			break
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	code, _ := pr.p.Wait(ctx)
	cancel()
	pr.p.Kill()
	pr.quiet.Stop()
	m.mu.Lock()
	if m.cur != pr { // replaced or stopped on purpose
		m.mu.Unlock()
		close(pr.done)
		return
	}
	retry := pr.resumed && code != 0 && time.Since(pr.started) < m.o.Retry
	m.cur = nil
	m.info.State = StateStopped
	info := m.info
	m.mu.Unlock()
	close(pr.done)
	if retry {
		if m.Start(pr.agent, false) == nil {
			return
		}
	}
	m.emitInfo(info)
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
	info := m.info
	m.mu.Unlock()
	if changed {
		m.emitInfo(info)
	}
}

func (m *Manager) quietFor(pr *proc) {
	m.mu.Lock()
	if m.cur != pr || m.info.State != StateWorking {
		m.mu.Unlock()
		return
	}
	m.info.State = StateWaiting
	info := m.info
	m.mu.Unlock()
	m.emitInfo(info)
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

func (m *Manager) subscribers() []func(Output) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fs := make([]func(Output), 0, len(m.subs))
	for _, f := range m.subs {
		fs = append(fs, f)
	}
	return fs
}

func (m *Manager) emitInfo(i Info) {
	m.notify.Lock()
	defer m.notify.Unlock()
	m.o.OnInfo(i)
}

// Stop ends the agent's process, if any, and waits for it.
func (m *Manager) Stop() {
	m.mu.Lock()
	pr := m.cur
	m.cur = nil
	if pr != nil {
		m.info.State = StateStopped
	}
	info := m.info
	m.mu.Unlock()
	if pr == nil {
		return
	}
	pr.p.Kill()
	<-pr.done
	pr.screen.Close()
	m.emitInfo(info)
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
	_, err := pr.p.Write(p)
	return err
}

// pasteGap separates the pasted text from the Enter that sends it.
var pasteGap = 100 * time.Millisecond

// Paste sends text as one message (spec M3 §5.1): bracketed paste when the
// agent turned it on, else one line; then Enter. Control characters are
// dropped first, so the text cannot end the paste early (ESC[201~) and type
// the rest as keys.
func (m *Manager) Paste(text string) error {
	pr := m.current()
	if pr == nil {
		return ErrNoAgent
	}
	text = clean(text)
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

// trustPrompts are what Claude Code's folder trust dialog shows.
var trustPrompts = []string{"Do you trust the files in this folder", "Is this a project you created or one you trust"}

// Busy reports why a chat message must not be typed now: the agent shows its
// folder trust dialog, which an Enter would answer "No, exit" (spec M3 §3.3).
func (m *Manager) Busy() string {
	pr := m.current()
	if pr == nil {
		return ""
	}
	text := pr.screen.Text()
	for _, p := range trustPrompts {
		if strings.Contains(text, p) {
			return "Answer the question in the agent's terminal first."
		}
	}
	return ""
}
