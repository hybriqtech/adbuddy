package cli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
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
	// lang is the language of what serve tells the phone app in a notice
	// (startPhoneLink: App.Lang); English when empty.
	lang i18n.Lang

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
	// remote keeps the Tailscale address the app reports (phone_remote.go);
	// always set by startPhoneLink, with no server when the app is off.
	remote *remoteLink
	// yolo is the YOLO mode's state (phone_yolo.go); serve gives it with useYolo, under mu.
	yolo *daemon.Yolo

	mu       sync.Mutex
	paused   bool
	onPause  func() // withdraws the question waiting for an answer (daemon.CancelConfirmations)
	entries  []json.RawMessage
	noticed  map[string]bool  // types told unsupported by notice on this connection
	sess     session.Info     // the agent's session, as shown on the phone
	activity string           // the agent's last adb command as the journal describes it (session.state.activity)
	manager  *session.Manager // nil until serve starts the session
	typer    *session.Typer   // the phone's typing into manager (onTermInput, onInterrupt)
	chat     chatState        // phone_chat.go
	term     termState        // phone_term.go
	files    filesState       // phone_files.go
	adapter  adapter          // phone_agent.go
	adb      phoneapp.ADB     // the phone's adb, for screenshots
	ws       string           // the environment (useWorkspace)
	// askLang and askTimeout are how `adbuddy remote on` asks the user
	// (useRemoteAsk); set before the IPC server starts.
	askLang    string
	askTimeout time.Duration
	// askBusy: a question of `remote on` is open; askCancel withdraws it
	// (a pause from the phone). Under mu.
	askBusy   bool
	askCancel context.CancelFunc

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
func chatPort(store *cfgStore) (int, error) {
	if p := store.Config().ChatPort; p != 0 {
		return p, nil
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
		if err := store.Update(func(c *config.Config) { c.ChatPort = port }); err != nil {
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
func startPhoneLink(ctx context.Context, app *App, e *env, store *cfgStore, ctl *remote.Controller, a phoneapp.ADB, session, agentName string, fallback confirm.Confirmer) (*phoneLink, error) {
	l := &phoneLink{log: app.Log(), agent: agentName, adb: a, termEvery: termInterval, backlog: backlogOf, lang: app.Lang}
	l.ui = newUILink(protectedAppsPath(e), app.Log())
	l.remote = newRemoteLink(ctx, store, ctl, connOf(a), app.Log())
	l.ui.deviceInfo = l.remote.onDeviceInfo
	l.conf = bridge.NewConfirmer(noApp{}, fallback)
	l.conf.StaleText = l.lang.T("notice.stale_reply")
	l.conf.RTLNote = l.lang.T("confirm.rtl_escaped")
	if trust := phoneapp.LoadTrust(releaseCert, e.dirs.Data); trust.DevAPK != "" {
		fmt.Fprintln(app.Stderr, app.T("phone.adbuddy_trusts_a_development_build", trust.DevAPK, phoneapp.DevTrustPath(e.dirs.Data)))
	}
	without := func(why i18n.Msg) (*phoneLink, error) {
		fmt.Fprintln(app.Stderr, app.T("phone.confirmations_happen_here_without_the", why))
		return l, nil
	}
	version, err := phoneapp.InstalledVersion(ctx, a)
	if ctx.Err() != nil {
		return nil, Interrupted()
	}
	if err != nil {
		// Only checked here: the app stays off until serve starts again.
		fmt.Fprintln(app.Stderr, app.T("phone.could_not_check_whether_the", classify.Printable(err.Error())))
		return l, nil
	}
	if version == "" {
		fmt.Fprintln(app.Stderr, app.T("phone.the_adbuddy_app_is_not"))
		return l, nil
	}
	port, err := chatPort(store)
	if err != nil {
		return without(i18n.M("phone.chat_port_unavailable", classify.Printable(err.Error()), e.dirs.ConfigFile()))
	}
	listener, err := bridge.Listen(port)
	if err != nil {
		return without(i18n.M("phone.chat_port_in_use", port, classify.Printable(err.Error()), e.dirs.ConfigFile()))
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
			fmt.Fprintln(app.Stderr, app.T("phone.the_adbuddy_app_on_the", classify.Printable(h.AppVersion)))
		},
		Admit: func(ctx context.Context, _ bridge.Hello) bool {
			if err := l.verifier.Same(ctx, a); err != nil {
				fmt.Fprintln(app.Stderr, app.T("phone.an_app_with_the_token", classify.Printable(err.Error())))
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
				fmt.Fprintln(app.Stderr, app.T("phone.the_phone_app_is_connected"))
				return
			}
			fmt.Fprintln(app.Stderr, app.T("phone.the_phone_app_disconnected_confirmations"))
			if l.Paused() {
				fmt.Fprintln(app.Stderr, app.T("phone.still_paused_resume_from_the"))
			}
			// A question already on the phone would wait there for
			// nobody: it goes to the fallback confirmer of --confirm too.
			l.conf.AppGone()
			l.onGone()
		},
	})
	l.conf = bridge.NewConfirmer(l.srv, fallback)
	l.conf.StaleText = l.lang.T("notice.stale_reply")
	l.conf.RTLNote = l.lang.T("confirm.rtl_escaped")
	l.remote.setServer(l.srv)
	l.setAreas(l.confirmArea(), l.chatArea(), l.agentArea(), l.uiArea(), l.remote.area(), l.yoloArea())
	l.conf.Notify = func(title string, moved bool) {
		if moved {
			fmt.Fprintln(app.Stderr, app.T("phone.the_phone_app_disconnected_before", classify.Printable(title)))
			// The app keeps its questions while away and when it comes
			// back to the same session: a snapshot without this one takes
			// it off the phone. It goes to the events kept for the app, so
			// the app gets it when it resumes, or at once if it is already
			// back (it may have got the question in its welcome's snapshot).
			l.srv.PublishSnapshot()
		} else {
			fmt.Fprintln(app.Stderr, app.T("phone.the_phone_app_is_back", classify.Printable(title)))
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
		fmt.Fprintln(app.Stderr, app.T("phone.could_not_reach_the_phone", classify.Printable(err.Error())))
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
				fmt.Fprintln(app.Stderr, app.T("phone.could_not_reopen_the_phone", classify.Printable(err.Error())))
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
	if j.Source == journal.SourceAgent {
		l.noteActivity(j)
	}
}

// maxActivity is the longest activity, in runes: it travels in every
// session.state.
const maxActivity = 120

// noteActivity records what the agent last did on the phone and publishes the
// state when it changes: a repeated description publishes nothing. Commands
// that did not run (denied, blocked, timed out) are not what the agent is doing.
func (l *phoneLink) noteActivity(j journal.Entry) {
	switch j.Status {
	case journal.StatusDenied, journal.StatusBlocked, journal.StatusTimeout:
		return
	}
	desc := strings.TrimSpace(j.Description)
	if desc == "" {
		return
	}
	if r := []rune(desc); len(r) > maxActivity {
		desc = string(r[:maxActivity-1]) + "…"
	}
	l.mu.Lock()
	changed := l.activity != desc
	l.activity = desc
	l.mu.Unlock()
	if changed && l.srv != nil {
		_ = l.srv.Publish(bridge.TypeSessionState, l.state())
	}
}

func (l *phoneLink) state() bridge.SessionState {
	view, hooked := l.adapterState() // before l.mu: the adapter's lock is never taken under it
	l.mu.Lock()
	defer l.mu.Unlock()
	st := bridge.SessionState{Agent: l.agent, State: session.StateStopped, Activity: l.activity, Paused: l.paused, View: "terminal"}
	if i := l.sess; i.Agent != "" {
		st.Agent, st.State, st.Conversation, st.PCAttached = i.Agent, i.State, i.Conversation, i.PCAttached
	}
	if view != "" && st.State != session.StateStopped {
		st.View = view
		if hooked != "" {
			st.State = hooked
		}
	}
	if st.State == session.StateStopped {
		st.Activity = ""
	}
	return st
}

// sessionInfo takes the agent's session info (session.Options.OnInfo) and
// shows it on the phone.
func (l *phoneLink) sessionInfo(i session.Info) {
	l.mu.Lock()
	if i.Conversation != l.sess.Conversation || i.State == session.StateStopped {
		l.activity = ""
	}
	l.sess = i
	l.mu.Unlock()
	l.chat.conversation(i.Conversation)
	l.adapterConversation(i.Conversation, i.State == session.StateStopped)
	if l.srv != nil {
		_ = l.srv.Publish(bridge.TypeSessionState, l.state())
	}
}

// useSession gives the chat area the agent's session.
func (l *phoneLink) useSession(m *session.Manager) {
	l.mu.Lock()
	l.manager, l.typer = m, m.NewTyper()
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
	if m.RID != "" {
		// The error's text is English by protocol (§4.1): the app shows its own by code.
		text := i18n.EN.T("notice.unsupported", classify.Printable(typ))
		_ = l.srv.Reply(m, bridge.TypeError, bridge.ErrorReply{Code: bridge.CodeUnsupported, Text: text})
		return
	}
	text := l.lang.T("notice.unsupported", classify.Printable(typ))
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
		fmt.Fprintln(l.log, l.lang.T("phone.paused_from_the_phone_every"))
		// The pause stops YOLO for good (spec): off before the question is withdrawn.
		if y := l.yoloOf(); y != nil {
			y.Off(daemon.YoloOffPause)
		}
		// After Paused reports true: the command whose question is
		// withdrawn then ends blocked as paused.
		if withdraw != nil {
			withdraw()
		}
		l.withdrawRemoteAsk()
	} else {
		fmt.Fprintln(l.log, l.lang.T("phone.resumed_from_the_phone"))
	}
	_ = l.srv.Publish(bridge.TypeSessionState, l.state())
}
