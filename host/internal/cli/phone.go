package cli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/session"
)

// tunnelCheck is how often serve checks that the chat tunnel is still in
// place: adb drops the reverse when the phone reconnects.
var tunnelCheck = 15 * time.Second

// keptEntries is how many journal entries a snapshot carries.
const keptEntries = 50

// phoneLink connects serve with the phone app (spec §5): the bridge server,
// confirmations on the phone and the pause.
type phoneLink struct {
	srv   *bridge.Server // nil when the app is not installed
	conf  *bridge.Confirmer
	log   io.Writer
	agent string

	// verifier checks the app installed (spec M6a §6.3). recheck is how
	// Admit asks for a new connect() when an app that is not the verified
	// one has connected.
	verifier *phoneapp.Verifier
	recheck  chan struct{}

	// areas are what each milestone adds (phone_areas.go), and handlers
	// their message handlers by type; set before the server starts.
	areas    []area
	handlers map[string]func(bridge.Message)
	// ui is the helpers' channel and the protected apps (phone_ui.go).
	ui *uiLink

	mu      sync.Mutex
	paused  bool
	onPause func() // withdraws the question waiting for an answer (daemon.CancelConfirmations)
	entries []json.RawMessage
	noticed map[string]bool  // types told unsupported by notice on this connection
	sess    session.Info     // the agent's session, as shown on the phone
	manager *session.Manager // nil until serve starts the session
	chat    chatState        // phone_chat.go
	term    termState        // phone_term.go
	files   filesState       // phone_files.go
	adb     phoneapp.ADB     // the phone's adb, for screenshots
	ws      string           // the environment (useWorkspace)

	// termEvery is termInterval as it was when the link started: read here,
	// not from the variable, by the goroutines that outlive a test.
	termEvery time.Duration
	// backlog is backlogOf at the same moment: termLoop and its senders call it.
	backlog func(*bridge.Server) int
}

// maxNotices is how many unsupported types a connection is told of by notice:
// each notice is an event, kept in the buffer of the ones to resend.
const maxNotices = 16

// chatPort returns the configured port of the app channel, choosing and
// saving one on first use (spec §5.2: fixed, so the reverse never changes).
func chatPort(e *env) (int, error) {
	if e.cfg.ChatPort != 0 {
		return e.cfg.ChatPort, nil
	}
	for range 20 {
		n, err := rand.Int(rand.Reader, big.NewInt(40000))
		if err != nil {
			return 0, err
		}
		port := 20000 + int(n.Int64())
		l, err := bridge.Listen(port)
		if err != nil {
			continue // in use
		}
		l.Close()
		e.cfg.ChatPort = port
		if err := config.Save(e.dirs.ConfigFile(), e.cfg); err != nil {
			return 0, err
		}
		return port, nil
	}
	return 0, fmt.Errorf("no free port found for the app channel")
}

// newToken makes the token of each handover (a variable so a test can make
// it fail).
var newToken = bridge.NewToken

// startPhoneLink starts the app channel when the app is installed. Without
// the app, or when the channel cannot start, it returns a link that
// confirms with fallback and never pauses, and says why: no failure of the
// app's channel stops serve. Its only error is Interrupted (ctx ended).
func startPhoneLink(ctx context.Context, app *App, e *env, a phoneapp.ADB, session, agentName string, fallback confirm.Confirmer) (*phoneLink, error) {
	l := &phoneLink{log: app.Stderr, agent: agentName, adb: a, termEvery: termInterval, backlog: backlogOf}
	l.ui = newUILink(protectedAppsPath(e), app.Stderr)
	l.conf = bridge.NewConfirmer(noApp{}, fallback)
	if trust := phoneapp.LoadTrust(releaseCert, e.dirs.Data); trust.DevAPK != "" {
		fmt.Fprintf(app.Stderr, "⚠ ADBuddy trusts a development build of the phone app (SHA-256 %s, from `adbuddy setup --apk`): it gets the token like a release. Delete %s to stop.\n", trust.DevAPK, phoneapp.DevTrustPath(e.dirs.Data))
	}
	without := func(why string) (*phoneLink, error) {
		fmt.Fprintf(app.Stderr, "⚠ %s. 🔴 confirmations happen here, without the phone app.\n", why)
		return l, nil
	}
	version, err := phoneapp.InstalledVersion(ctx, a)
	if ctx.Err() != nil {
		return nil, Interrupted()
	}
	if err != nil {
		// Only checked here: the app stays off until serve starts again.
		fmt.Fprintf(app.Stderr, "⚠ Could not check whether the ADBuddy app is on the phone (%s). The phone app is off for the rest of this session: "+
			"🔴 confirmations happen here; restart `adbuddy serve` to use it.\n", classify.Printable(err.Error()))
		return l, nil
	}
	if version == "" {
		fmt.Fprintln(app.Stderr, "The ADBuddy app is not on the phone: 🔴 confirmations happen here. Run `adbuddy setup` to install it.")
		return l, nil
	}
	port, err := chatPort(e)
	if err != nil {
		return without("Could not choose a port for the phone app's channel (" + classify.Printable(err.Error()) + "); set chat_port in " + e.dirs.ConfigFile())
	}
	listener, err := bridge.Listen(port)
	if err != nil {
		return without("Port " + strconv.Itoa(port) + " for the phone app's channel is in use (" + classify.Printable(err.Error()) + "); change chat_port in " +
			e.dirs.ConfigFile() + " to a free port between 1024 and 65535 and start adbuddy serve again")
	}
	l.verifier = phoneapp.NewVerifier(func() phoneapp.Trust { return phoneapp.LoadTrust(releaseCert, e.dirs.Data) }, phoneapp.MinAppVersionCode)
	l.recheck = make(chan struct{}, 1)
	// No token until connect hands one over: nobody gets in before.
	l.srv = bridge.New(bridge.Options{
		Session: session, HostVersion: Version,
		OnMessage: l.onMessage,
		OnFrame:   l.onFrame,
		OnAttach:  l.onAttach,
		Snapshot:  l.snapshot,
		OnReject: func(h bridge.Hello) {
			fmt.Fprintf(app.Stderr, "⚠ The ADBuddy app on the phone (%s) is too old for this host: run `adbuddy setup` to update it.\n", classify.Printable(h.AppVersion))
		},
		Admit: func(ctx context.Context, _ bridge.Hello) bool {
			if err := l.verifier.Same(ctx, a); err != nil {
				fmt.Fprintf(app.Stderr, "⚠ An app with the token connected, but it is not the app ADBuddy checked (%s): refused, checking the app again.\n", classify.Printable(err.Error()))
				select {
				case l.recheck <- struct{}{}:
				default:
				}
				return false
			}
			return true
		},
		OnConnect: func(on bool) {
			if on {
				fmt.Fprintln(app.Stderr, "📱 The phone app is connected: 🔴 confirmations go to the phone.")
				return
			}
			fmt.Fprintln(app.Stderr, "📱 The phone app disconnected: 🔴 confirmations happen here.")
			if l.Paused() {
				fmt.Fprintln(app.Stderr, "⏸ Still paused: resume from the app when it reconnects, or restart adbuddy serve.")
			}
			// A question already on the phone would wait there for
			// nobody: it moves here too.
			l.conf.AppGone()
			l.onGone()
		},
	})
	l.conf = bridge.NewConfirmer(l.srv, fallback)
	l.setAreas(l.confirmArea(), l.chatArea(), l.agentArea(), l.uiArea())
	l.conf.Notify = func(title string, moved bool) {
		if moved {
			fmt.Fprintf(app.Stderr, "📱 The phone app disconnected before you answered “%s”: it no longer waits for the phone.\n", classify.Printable(title))
			// The app keeps its questions while away and when it comes
			// back to the same session: a snapshot without this one takes
			// it off the phone. It goes to the events kept for the app, so
			// the app gets it when it resumes, or at once if it is already
			// back (it may have got the question in its welcome's snapshot).
			l.srv.PublishSnapshot()
		} else {
			fmt.Fprintf(app.Stderr, "📱 The phone app is back: “%s” still waits for your answer on the phone.\n", classify.Printable(title))
		}
	}
	go l.srv.Serve(ctx, listener)
	host, _ := os.Hostname()
	host = phoneapp.HostName(host)
	// connect hands the channel over again with a new token (spec §5.2): the
	// server takes only that one and drops an app with the old one before
	// the tunnel is back, so a token captured meanwhile is worth nothing.
	// The token only goes to the app checked right before (spec M6a §6.3):
	// the verifier reads the APK again only after a reinstall or an update.
	connect := func() error {
		// A new token first: whatever app held the old one loses it, even
		// when this check then fails.
		token, err := newToken()
		if err != nil {
			return fmt.Errorf("could not make a token: %w", err)
		}
		l.srv.SetToken(token)
		v, err := l.verifier.Verify(ctx, a)
		if err != nil {
			return err
		}
		if err := phoneapp.Tunnel(ctx, a, port); err != nil {
			return err
		}
		return phoneapp.Handshake(ctx, a, v, token, port, host)
	}
	err = connect()
	if ctx.Err() != nil {
		return nil, Interrupted()
	}
	if err != nil {
		fmt.Fprintf(app.Stderr, "⚠ Could not reach the phone app (%s): 🔴 confirmations happen here until it connects.\n", classify.Printable(err.Error()))
	}
	// Read here, before the goroutine: a test that sets tunnelCheck for its
	// own link must not race with the goroutine of a previous one.
	every := tunnelCheck
	go func() {
		// connected: the last connect() worked. If not, it is tried again
		// on each tick; if so, only when adb lost the tunnel. A failure is
		// said once until it works again.
		connected, warned := err == nil, err != nil
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			case <-l.recheck:
				connected = false
			}
			if connected {
				if open, err := phoneapp.TunnelOpen(ctx, a, port); err != nil || open {
					continue
				}
			}
			err := connect()
			connected = err == nil
			if connected {
				warned = false
			} else if !warned && ctx.Err() == nil {
				warned = true
				fmt.Fprintf(app.Stderr, "⚠ Could not reopen the phone app's channel: %s\n", classify.Printable(err.Error()))
			}
		}
	}()
	return l, nil
}

// noApp is a Publisher with no app behind it.
type noApp struct{}

func (noApp) Connected() bool           { return false }
func (noApp) Publish(string, any) error { return nil }

// Confirmer asks on the phone when the app is connected, else fallback.
func (l *phoneLink) Confirmer() confirm.Confirmer { return l.conf }

// OnPause sets what withdraws the question waiting for an answer when the
// person pauses from the phone (the daemon's CancelConfirmations): the
// daemon is created after the link.
func (l *phoneLink) OnPause(f func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onPause = f
}

// Paused reports whether the person paused the agent from the phone.
func (l *phoneLink) Paused() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.paused
}

// Entry sends a journal entry to the app's activity panel.
func (l *phoneLink) Entry(j journal.Entry) {
	raw, err := json.Marshal(j)
	if err != nil {
		return
	}
	l.mu.Lock()
	l.entries = append(l.entries, raw)
	if len(l.entries) > keptEntries {
		l.entries = l.entries[len(l.entries)-keptEntries:]
	}
	l.mu.Unlock()
	if l.srv != nil {
		_ = l.srv.Publish(bridge.TypeJournalEntry, json.RawMessage(raw))
	}
}

func (l *phoneLink) state() bridge.SessionState {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := bridge.SessionState{Agent: l.agent, State: session.StateStopped, Paused: l.paused, View: "terminal"}
	if i := l.sess; i.Agent != "" {
		st.Agent, st.State, st.Conversation, st.PCAttached = i.Agent, i.State, i.Conversation, i.PCAttached
	}
	return st
}

// sessionInfo takes the agent's session info (session.Options.OnInfo) and
// shows it on the phone.
func (l *phoneLink) sessionInfo(i session.Info) {
	l.mu.Lock()
	l.sess = i
	l.mu.Unlock()
	l.chat.conversation(i.Conversation)
	if l.srv != nil {
		_ = l.srv.Publish(bridge.TypeSessionState, l.state())
	}
}

// useSession gives the chat area the agent's session.
func (l *phoneLink) useSession(m *session.Manager) {
	l.mu.Lock()
	l.manager = m
	l.mu.Unlock()
	l.subscribeTerm(m) // phone_term.go
}

// snapshot is called with the server's lock held: neither it nor the parts
// each area adds may call Publish, PublishRID, PublishSnapshot, Reply, Send,
// Current or Connected, nor hold a lock while calling them.
func (l *phoneLink) snapshot() bridge.Snapshot {
	st := l.state()
	l.mu.Lock()
	entries := append([]json.RawMessage{}, l.entries...)
	l.mu.Unlock()
	s := bridge.Snapshot{State: st, Pending: l.conf.Pending(), Journal: entries}
	for _, a := range l.areas {
		if a.snapshot != nil {
			a.snapshot(&s)
		}
	}
	return s
}

// onMessage handles what the app sends. Everything it carries comes from the
// phone: only known types with checked payloads do anything.
func (l *phoneLink) onMessage(m bridge.Message) {
	if h, ok := l.handlers[m.Type]; ok {
		h(m)
		return
	}
	l.unsupported(m)
}

// unsupported answers a type this host does not handle (protocol spec §3):
// an error to a request with a rid, and otherwise a notice, which an M2 app
// understands. A notice is an event, kept to resend: only one per type and
// at most maxNotices per connection, so an app sending unknown types does not
// push real events out.
func (l *phoneLink) unsupported(m bridge.Message) {
	typ := m.Type
	if r := []rune(typ); len(r) > 64 {
		typ = string(r[:64]) + "…"
	}
	text := "This version of the ADBuddy host does not handle " + classify.Printable(typ) + " yet."
	if m.RID != "" {
		_ = l.srv.Reply(m, bridge.TypeError, bridge.ErrorReply{Code: bridge.CodeUnsupported, Text: text})
		return
	}
	l.mu.Lock()
	told := l.noticed[typ] || len(l.noticed) >= maxNotices
	if l.noticed == nil {
		l.noticed = map[string]bool{}
	}
	if !told {
		l.noticed[typ] = true
	}
	l.mu.Unlock()
	if !told {
		_ = l.srv.Publish(bridge.TypeNotice, bridge.Notice{Level: "info", Text: text})
	}
}

// onPauseMessage handles pause and resume from the phone.
func (l *phoneLink) onPauseMessage(m bridge.Message) {
	pause := m.Type == bridge.TypePause
	l.mu.Lock()
	l.paused = pause
	withdraw := l.onPause
	l.mu.Unlock()
	if pause {
		fmt.Fprintln(l.log, "⏸ Paused from the phone: every adb command of the agent is refused until you resume.")
		// After Paused reports true: the command whose question is
		// withdrawn then ends blocked as paused.
		if withdraw != nil {
			withdraw()
		}
	} else {
		fmt.Fprintln(l.log, "▶ Resumed from the phone.")
	}
	_ = l.srv.Publish(bridge.TypeSessionState, l.state())
}
