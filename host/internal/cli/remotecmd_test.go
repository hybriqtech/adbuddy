package cli

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// askFake is a confirmer that records every question and answers with
// answer (or, when wait is set, when the context ends).
type askFake struct {
	mu     sync.Mutex
	reqs   []confirm.Request
	hadDL  []bool
	answer confirm.Decision
	wait   bool
}

func (f *askFake) Confirm(ctx context.Context, r confirm.Request) confirm.Decision {
	_, dl := ctx.Deadline()
	f.mu.Lock()
	f.reqs = append(f.reqs, r)
	f.hadDL = append(f.hadDL, dl)
	answer, wait := f.answer, f.wait
	f.mu.Unlock()
	if wait {
		<-ctx.Done()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return confirm.Timeout
		}
		return confirm.Deny
	}
	return answer
}

func (f *askFake) set(d confirm.Decision) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answer = d
}

func (f *askFake) asked() []confirm.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]confirm.Request{}, f.reqs...)
}

// remoteHost is a serve's remote mode with a fake phone and a fake confirmer,
// answering `adbuddy remote` on the data folder's socket.
type remoteHost struct {
	g    *remoteRig
	l    *phoneLink
	ask  *askFake
	data string
}

func newRemoteHost(t *testing.T, cfg config.Config, answer confirm.Decision) *remoteHost {
	t.Helper()
	data := isolate(t)
	g := newRemoteRig(t, cfg, "41234")
	ask := &askFake{answer: answer}
	l := &phoneLink{remote: g.r, conf: bridge.NewConfirmer(noApp{}, ask), log: g.log}
	l.useRemoteAsk("en", time.Minute)
	setVar(t, &remoteNow, g.clock.now)
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	d := remoteDaemon(t, l, nil)
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		if req.Kind != ipc.KindRemote {
			return ipc.Exit{Code: 1, ErrorCode: "UNKNOWN_REQUEST"}
		}
		return l.handleRemote(ctx, req, c, d, nil)
	})
	if err != nil {
		t.Skipf("no socket: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	return &remoteHost{g: g, l: l, ask: ask, data: data}
}

// call sends `adbuddy remote <arg>` to the host and returns its answer.
func (h *remoteHost) call(t *testing.T, arg string, env map[string]string) (ipc.Exit, remoteInfo) {
	t.Helper()
	var info remoteInfo
	exit, err := ipc.Call(context.Background(), h.data, ipc.Request{Kind: ipc.KindRemote, Argv: []string{arg}, Env: env}, nil, nopWriter{}, nopWriter{},
		func(b []byte) error { return json.Unmarshal(b, &info) })
	if err != nil {
		t.Fatal(err)
	}
	return exit, info
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

func localHM(ts time.Time) string { return ts.Local().Format("15:04") }

// From the agent's session the three subcommands fail before the socket.
func TestRemoteRefusedFromTheAgentSession(t *testing.T) {
	data := isolate(t)
	t.Setenv("ADBUDDY_SOURCE", "agent")
	var mu sync.Mutex
	calls := 0
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		mu.Lock()
		calls++
		mu.Unlock()
		return ipc.Exit{}
	})
	if err != nil {
		t.Skipf("no socket: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	for _, arg := range []string{"on", "off", "status"} {
		out, errOut, code := run(t, "remote", arg)
		if code != ExitFailure || !strings.Contains(errOut, "AGENT_SESSION") || out != "" {
			t.Errorf("%s: exit %d stdout %q stderr %q", arg, code, out, errOut)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 0 {
		t.Fatalf("the socket was called %d times", calls)
	}
}

// The server refuses a request that declares the agent's session, whatever
// it asks, and never touches the phone or asks the user.
func TestHandleRemoteRefusesTheAgent(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	for _, arg := range []string{"on", "off", "status"} {
		exit, _ := h.call(t, arg, map[string]string{"ADBUDDY_SOURCE": "agent"})
		if exit.Code != 1 || exit.ErrorCode != "AGENT_SESSION" {
			t.Errorf("%s: %+v", arg, exit)
		}
	}
	if len(h.g.phone.called()) != 0 || len(h.ask.asked()) != 0 {
		t.Fatalf("phone %v, questions %v", h.g.phone.called(), h.ask.asked())
	}
}

func TestRemoteUsageIsChecked(t *testing.T) {
	isolate(t)
	for _, args := range [][]string{{"remote"}, {"remote", "maybe"}, {"remote", "on", "off"}} {
		if _, errOut, code := run(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d stderr %q", args, code, errOut)
		}
	}
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	if exit, _ := h.call(t, "maybe", nil); exit.Code != 2 || exit.ErrorCode != "USAGE" {
		t.Fatalf("%+v", exit)
	}
}

func TestRemoteOnWithoutServe(t *testing.T) {
	isolate(t)
	if _, errOut, code := run(t, "remote", "on"); code != ExitFailure || !strings.Contains(errOut, "DAEMON_NOT_RUNNING") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}

func saveRemoteCmdConfig(t *testing.T, e setupEnv, until string, serial string) {
	t.Helper()
	cfg := config.Default()
	cfg.Serial, cfg.ADBPath, cfg.Workspace = serial, fakeADB(t), e.ws
	cfg.TailscaleIP, cfg.RemoteUntil = "100.64.0.5", until
	if err := config.Save(filepath.Join(e.cfgDir, "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteStatusWithoutServeReadsTheConfig(t *testing.T) {
	e := newSetupEnv(t, nil)
	future := time.Now().Add(3 * time.Hour).UTC()
	saveRemoteCmdConfig(t, e, future.Format(time.RFC3339), "")
	out, errOut, code := run(t, "remote", "status")
	if code != 0 || !strings.Contains(out, "serve is not running; remote mode: on until "+localHM(future)) {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
	saveRemoteCmdConfig(t, e, "", "")
	out, _, code = run(t, "remote", "status")
	if code != 0 || !strings.Contains(out, "serve is not running; remote mode: off") {
		t.Fatalf("exit %d stdout %q", code, out)
	}
	// An expiry that passed with nothing to close the port: not "off".
	past := time.Now().Add(-time.Hour).UTC()
	saveRemoteCmdConfig(t, e, past.Format(time.RFC3339), "")
	out, _, code = run(t, "remote", "status")
	if code != 0 || !strings.Contains(out, "expired") || !strings.Contains(out, "remote off") || strings.Contains(out, "remote mode: off") {
		t.Fatalf("expired: exit %d stdout %q", code, out)
	}
	out, _, code = run(t, "remote", "status", "--json")
	var env struct {
		OK   bool
		Data remoteInfo
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || code != 0 || !env.OK || !env.Data.Enabled || !env.Data.Closing || env.Data.Serve {
		t.Fatalf("json: exit %d %q (%v)", code, out, err)
	}
}

func TestRemoteOnThroughServe(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	out, errOut, code := run(t, "remote", "on")
	if code != 0 {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
	until := h.g.clock.now().Add(12 * time.Hour)
	want := "Remote mode is on until " + localHM(until) + " (about 12 h). Reach the phone at 100.101.102.103:5555. " +
		"It stays open on any network until then: turn it off with 'adbuddy remote off' when you are home."
	if strings.TrimSpace(out) != want {
		t.Fatalf("stdout %q\nwant   %q", out, want)
	}
	if n := h.g.phone.count("tcpip 5555"); n != 1 {
		t.Fatalf("tcpip ran %d times: %v", n, h.g.phone.called())
	}
	// serve's own store has the expiry: the CLI never wrote config.toml.
	if h.g.store.Until().IsZero() {
		t.Fatal("the store of serve has no expiry")
	}
	// The app is told, once, with the same state.
	var states []bridge.RemoteState
	for _, m := range h.g.srv.messages() {
		if s, ok := m.payload.(bridge.RemoteState); ok && m.kind == "event" && m.typ == bridge.TypeRemoteState {
			states = append(states, s)
		}
	}
	if len(states) != 1 || !states[0].Enabled || states[0].Address != "100.101.102.103:5555" {
		t.Fatalf("published %+v", states)
	}
	// --json: the usual envelope.
	out, _, code = run(t, "remote", "status", "--json")
	var env struct {
		OK   bool
		Data remoteInfo
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || code != 0 || !env.OK || !env.Data.Enabled || !env.Data.Serve || env.Data.Address != "100.101.102.103:5555" {
		t.Fatalf("json: exit %d %q (%v)", code, out, err)
	}
}

func TestRemoteOffThroughServe(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	if _, _, code := run(t, "remote", "on"); code != 0 {
		t.Fatal(code)
	}
	asked := len(h.ask.asked())
	out, errOut, code := run(t, "remote", "off")
	if code != 0 || !strings.Contains(out, "Remote mode is off") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
	if h.g.phone.count("usb") != 1 || !h.g.store.Until().IsZero() {
		t.Fatalf("calls %v until %v", h.g.phone.called(), h.g.store.Until())
	}
	if len(h.ask.asked()) != asked {
		t.Fatal("off asked the user")
	}
	last := h.g.srv.messages()
	if s, _ := last[len(last)-1].payload.(bridge.RemoteState); s.Enabled {
		t.Fatalf("last published %+v", last[len(last)-1])
	}
	// Failing `usb`: the mode stays on, and it says so.
	h.g.phone.setFail("usb", true)
	if _, _, code := run(t, "remote", "on"); code != 0 {
		t.Fatal(code)
	}
	errOut, _, code = run(t, "remote", "off", "--json")
	if code != ExitFailure || !strings.Contains(errOut, "REMOTE_FAILED") || h.g.store.Until().IsZero() {
		t.Fatalf("exit %d stderr %q until %v", code, errOut, h.g.store.Until())
	}
}

func TestRemoteOnWithoutTailscale(t *testing.T) {
	h := newRemoteHost(t, config.Default(), confirm.Allow)
	out, errOut, code := run(t, "remote", "on")
	if code != ExitFailure || out != "" || !strings.Contains(errOut, "REMOTE_UNAVAILABLE") || !strings.Contains(errOut, "Turn on Tailscale on the phone first") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
	if len(h.ask.asked()) != 0 || len(h.g.phone.called()) != 0 {
		t.Fatalf("asked %v, phone %v", h.ask.asked(), h.g.phone.called())
	}
}

// The question carries exactly what keeps "allow during the session" and
// every shortcut away from it.
func TestRemoteOnAsksTheUser(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	before := time.Now()
	exit, info := h.call(t, "on", nil)
	if exit.Code != 0 || !info.Enabled {
		t.Fatalf("%+v %+v", exit, info)
	}
	qs := h.ask.asked()
	if len(qs) != 1 {
		t.Fatalf("asked %d times", len(qs))
	}
	q := qs[0]
	if q.Key != "" || !q.Special || !strings.HasPrefix(q.ID, "r_") || len(q.ID) < 10 {
		t.Fatalf("request %+v", q)
	}
	if q.Title != "Open adb's network port 5555 for 12 hours" {
		t.Fatalf("title %q", q.Title)
	}
	if !h.ask.hadDL[0] || q.ExpiresAt.Before(before.Add(50*time.Second)) || q.ExpiresAt.After(time.Now().Add(70*time.Second)) {
		t.Fatalf("deadline %v, expires %v", h.ask.hadDL[0], q.ExpiresAt)
	}
	if h.g.phone.count("tcpip 5555") != 1 {
		t.Fatalf("calls %v", h.g.phone.called())
	}
}

func TestRemoteOnTitleIsInTheUsersLanguage(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Deny)
	h.l.useRemoteAsk("es", time.Minute)
	h.call(t, "on", nil)
	if qs := h.ask.asked(); len(qs) != 1 || qs[0].Title != "Abrir el puerto de red 5555 de adb durante 12 horas" {
		t.Fatalf("%+v", qs)
	}
}

func TestRemoteOnNotConfirmedDoesNothing(t *testing.T) {
	cases := map[string]struct {
		answer confirm.Decision
		wait   bool
		pause  bool
		why    string
	}{
		"denied":  {answer: confirm.Deny, why: "denied"},
		"nobody":  {answer: confirm.Nobody, why: "nobody"},
		"timeout": {answer: confirm.Timeout, why: "expired"},
		"expires": {wait: true, why: "expired"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newRemoteHost(t, withTailscale(), c.answer)
			if c.wait {
				h.ask.mu.Lock()
				h.ask.wait = true
				h.ask.mu.Unlock()
				h.l.useRemoteAsk("en", 50*time.Millisecond)
			}
			exit, _ := h.call(t, "on", nil)
			if exit.Code != 126 || exit.ErrorCode != "REMOTE_DENIED" || !strings.Contains(exit.Error, c.why) {
				t.Fatalf("%+v", exit)
			}
			if len(h.g.phone.called()) != 0 || !h.g.store.Until().IsZero() {
				t.Fatalf("phone %v until %v", h.g.phone.called(), h.g.store.Until())
			}
		})
	}
	t.Run("paused", func(t *testing.T) {
		h := newRemoteHost(t, withTailscale(), confirm.Allow)
		h.l.mu.Lock()
		h.l.paused = true
		h.l.mu.Unlock()
		exit, _ := h.call(t, "on", nil)
		if exit.Code != 126 || exit.ErrorCode != "REMOTE_DENIED" || !strings.Contains(exit.Error, "paused") {
			t.Fatalf("%+v", exit)
		}
		if len(h.ask.asked()) != 0 || len(h.g.phone.called()) != 0 {
			t.Fatalf("asked %v phone %v", h.ask.asked(), h.g.phone.called())
		}
	})
}

// An answer that arrives after a pause does not open the port.
func TestRemoteOnPausedWhileAskingDenies(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	h.l.conf = bridge.NewConfirmer(noApp{}, confirmerFunc(func(context.Context, confirm.Request) confirm.Decision {
		h.l.mu.Lock()
		h.l.paused = true
		h.l.mu.Unlock()
		return confirm.Allow
	}))
	exit, _ := h.call(t, "on", nil)
	if exit.Code != 126 || exit.ErrorCode != "REMOTE_DENIED" || len(h.g.phone.called()) != 0 {
		t.Fatalf("%+v %v", exit, h.g.phone.called())
	}
}

type confirmerFunc func(context.Context, confirm.Request) confirm.Decision

func (f confirmerFunc) Confirm(ctx context.Context, r confirm.Request) confirm.Decision {
	return f(ctx, r)
}

// An earlier yes is worth nothing for the next request: after the user closes
// the port, `on` asks again, and a no keeps it closed.
func TestRemoteOnAsksAgainEvenAfterAnAllow(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	if exit, _ := h.call(t, "on", nil); exit.Code != 0 {
		t.Fatalf("%+v", exit)
	}
	if exit, _ := h.call(t, "off", nil); exit.Code != 0 {
		t.Fatalf("%+v", exit)
	}
	h.ask.set(confirm.Deny)
	if exit, _ := h.call(t, "on", nil); exit.Code != 126 || exit.ErrorCode != "REMOTE_DENIED" {
		t.Fatalf("%+v", exit)
	}
	if qs := h.ask.asked(); len(qs) != 2 || qs[0].ID == qs[1].ID {
		t.Fatalf("questions %+v", qs)
	}
	if h.g.phone.count("tcpip 5555") != 1 || !h.g.store.Until().IsZero() {
		t.Fatalf("calls %v", h.g.phone.called())
	}
}

// Already on: nothing opens, so nothing is asked.
func TestRemoteOnWhenAlreadyOnDoesNotAsk(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	h.call(t, "on", nil)
	if exit, info := h.call(t, "on", nil); exit.Code != 0 || !info.Enabled {
		t.Fatalf("%+v %+v", exit, info)
	}
	if len(h.ask.asked()) != 1 || h.g.phone.count("tcpip 5555") != 1 {
		t.Fatalf("asked %d, calls %v", len(h.ask.asked()), h.g.phone.called())
	}
}

// An expired mode that is not closed yet shows as expired, not as off.
func TestRemoteStatusShowsClosingAsExpired(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	h.call(t, "on", nil)
	h.g.clock.advance(13 * time.Hour)
	out, _, code := run(t, "remote", "status")
	if code != 0 || !strings.Contains(out, "expired") || !strings.Contains(out, "not") || strings.Contains(out, "mode: off") {
		t.Fatalf("exit %d stdout %q", code, out)
	}
	_, info := h.call(t, "status", nil)
	if !info.Enabled || !info.Closing {
		t.Fatalf("%+v", info)
	}
}

// off asked while an `on` of the app still runs goes behind it: the last
// request wins and the port ends closed.
func TestRemoteOffQueuesBehindTheAppsOn(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	gate := make(chan struct{})
	h.g.phone.mu.Lock()
	h.g.phone.gate = gate
	h.g.phone.mu.Unlock()
	h.g.srv.current.Store(true)
	h.g.r.handleSet(setMsg("r1", `{"enabled":true}`))
	waitUntil(t, "the app's on to start", func() bool { return h.g.phone.count("tcpip 5555") == 1 })
	done := make(chan ipc.Exit, 1)
	go func() {
		exit, _ := h.call(t, "off", nil)
		done <- exit
	}()
	waitUntil(t, "off to be queued", func() bool {
		h.g.r.mu.Lock()
		defer h.g.r.mu.Unlock()
		return len(h.g.r.queue) == 1
	})
	close(gate)
	if exit := <-done; exit.Code != 0 {
		t.Fatalf("%+v", exit)
	}
	calls := h.g.phone.called()
	if len(calls) != 2 || calls[0] != "tcpip 5555" || calls[1] != "usb" || !h.g.store.Until().IsZero() {
		t.Fatalf("calls %v until %v", calls, h.g.store.Until())
	}
}

// off without serve closes the port directly and only then clears the expiry.
func TestRemoteOffWithoutServe(t *testing.T) {
	offRules := func(usb testutil.FakeRule) []testutil.FakeRule {
		return []testutil.FakeRule{
			{Match: "^start-server$"},
			{Match: "^devices$", Stdout: phoneOnline},
			{Match: `^shell getprop ro\.serialno$`, Stdout: "AAAA1111\n"},
			usb,
		}
	}
	until := time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339)
	t.Run("closes and clears", func(t *testing.T) {
		e := newSetupEnv(t, offRules(testutil.FakeRule{Match: "^usb$", Stdout: "restarting in USB mode\n"}))
		saveRemoteCmdConfig(t, e, until, phoneSerial)
		out, errOut, code := run(t, "remote", "off")
		if code != 0 || !strings.Contains(out, "Remote mode is off") {
			t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
		}
		if cfg := loadSetupConfig(t, e); cfg.RemoteUntil != "" {
			t.Fatalf("remote_until %q", cfg.RemoteUntil)
		}
		if !strings.Contains(strings.Join(loggedCalls(t, e.log), "|"), "usb") {
			t.Fatalf("calls %q", loggedCalls(t, e.log))
		}
	})
	// Final review, minor 3: already off, it says so and leaves adb alone
	// (`adb usb` would restart adbd for nothing).
	t.Run("already off", func(t *testing.T) {
		e := newSetupEnv(t, offRules(testutil.FakeRule{Match: "^usb$", Stderr: "error: closed\n", Exit: 1}))
		saveRemoteCmdConfig(t, e, "", phoneSerial)
		out, errOut, code := run(t, "remote", "off")
		if code != 0 || !strings.Contains(out, "remote mode is already off") || strings.Contains(errOut, "stays on") {
			t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
		}
		if calls := loggedCalls(t, e.log); len(calls) != 0 {
			t.Fatalf("adb was used: %q", calls)
		}
		out, _, code = run(t, "remote", "off", "--json")
		var env struct {
			OK   bool
			Data remoteInfo
		}
		if err := json.Unmarshal([]byte(out), &env); err != nil || code != 0 || !env.OK || env.Data.Enabled || env.Data.Serve {
			t.Fatalf("json: exit %d %q (%v)", code, out, err)
		}
	})
	t.Run("usb fails", func(t *testing.T) {
		e := newSetupEnv(t, offRules(testutil.FakeRule{Match: "^usb$", Stderr: "error: closed\n", Exit: 1}))
		saveRemoteCmdConfig(t, e, until, phoneSerial)
		out, errOut, code := run(t, "remote", "off")
		if code != ExitFailure || !strings.Contains(errOut, "REMOTE_OFF_FAILED") || strings.Contains(out, "is off") {
			t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
		}
		if cfg := loadSetupConfig(t, e); cfg.RemoteUntil != until {
			t.Fatalf("remote_until %q, want it kept", cfg.RemoteUntil)
		}
	})
}

// ---- Round 1 of the review ----

// serve's own wiring (not a call the test makes to useRemoteAsk) gives the
// question the user's language and confirm_timeout.
func TestServeHandlerWiresTheRemoteQuestion(t *testing.T) {
	data := isolate(t)
	g := newRemoteRig(t, withTailscale(), "41234")
	ask := &askFake{answer: confirm.Deny}
	l := &phoneLink{remote: g.r, conf: bridge.NewConfirmer(noApp{}, ask), log: g.log}
	cfg := withTailscale()
	cfg.ConfirmTimeout = config.Duration{Duration: 30 * time.Second}
	getenv := func(k string) string {
		if k == "LANG" {
			return "es_ES.UTF-8"
		}
		return ""
	}
	h := serveHandler(l, cfg, getenv, t.TempDir(), "claude", nil, remoteDaemon(t, l, nil))
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ipc.Listen(data, secret, h)
	if err != nil {
		t.Skipf("no socket: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	before := time.Now()
	exit, err := ipc.Call(context.Background(), data, ipc.Request{Kind: ipc.KindRemote, Argv: []string{"on"}}, nil, nopWriter{}, nopWriter{}, nil)
	if err != nil || exit.ErrorCode != "REMOTE_DENIED" {
		t.Fatalf("%v %+v", err, exit)
	}
	qs := ask.asked()
	if len(qs) != 1 || qs[0].Title != "Abrir el puerto de red 5555 de adb durante 12 horas" {
		t.Fatalf("questions %+v", qs)
	}
	if d := qs[0].ExpiresAt.Sub(before); d < 25*time.Second || d > 35*time.Second {
		t.Fatalf("the question expires in %v, want about 30s (confirm_timeout)", d)
	}
}

// A yes that comes after a later `off` (from the app or the terminal) is the
// answer to an old question: it does not open the port.
func TestRemoteOnAllowedAfterALaterOffDenies(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	h.l.conf = bridge.NewConfirmer(noApp{}, confirmerFunc(func(context.Context, confirm.Request) confirm.Decision {
		if !h.g.r.enqueue(remoteJob{on: false, timeout: time.Second, reply: make(chan remoteResult, 1)}) {
			t.Error("not queued")
		}
		return confirm.Allow
	}))
	exit, _ := h.call(t, "on", nil)
	if exit.Code != 126 || exit.ErrorCode != "REMOTE_DENIED" || !strings.Contains(exit.Error, "turned off while asking; run it again") {
		t.Fatalf("%+v", exit)
	}
	// The mode was off: the off only looks at the port (closed), without `adb usb`.
	waitUntil(t, "the off to run", func() bool { return h.g.phone.count("shell getprop service.adb.tcp.port") == 1 })
	if h.g.phone.count("tcpip 5555") != 0 || h.g.phone.count("usb") != 0 {
		t.Fatalf("calls %v", h.g.phone.called())
	}
}

// One question of `on` at a time.
func TestRemoteOnOneQuestionAtATime(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	entered, release := make(chan struct{}, 4), make(chan struct{})
	var mu sync.Mutex
	asks := 0
	h.l.conf = bridge.NewConfirmer(noApp{}, confirmerFunc(func(context.Context, confirm.Request) confirm.Decision {
		mu.Lock()
		asks++
		mu.Unlock()
		entered <- struct{}{}
		<-release
		return confirm.Allow
	}))
	first := make(chan ipc.Exit, 1)
	go func() {
		exit, _ := h.call(t, "on", nil)
		first <- exit
	}()
	<-entered
	exit, _ := h.call(t, "on", nil)
	if exit.Code != 1 || exit.ErrorCode != "REMOTE_BUSY" {
		t.Fatalf("second: %+v", exit)
	}
	mu.Lock()
	if asks != 1 {
		t.Fatalf("asked %d times", asks)
	}
	mu.Unlock()
	close(release)
	if e := <-first; e.Code != 0 {
		t.Fatalf("first: %+v", e)
	}
	// Answered: it can be asked again.
	if e, _ := h.call(t, "off", nil); e.Code != 0 {
		t.Fatalf("%+v", e)
	}
	if e, _ := h.call(t, "on", nil); e.Code != 0 {
		t.Fatalf("third: %+v", e)
	}
	mu.Lock()
	defer mu.Unlock()
	if asks != 2 {
		t.Fatalf("asked %d times, want 2", asks)
	}
}

// The client leaves while the question is open: denied, nothing runs, and the
// next `on` can ask again.
func TestRemoteOnClientGoneWhileAsking(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	entered, ended := make(chan struct{}), make(chan error, 1)
	var once sync.Once
	h.l.conf = bridge.NewConfirmer(noApp{}, confirmerFunc(func(ctx context.Context, _ confirm.Request) confirm.Decision {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		select {
		case ended <- ctx.Err():
		default:
		}
		return confirm.Deny
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	callDone := make(chan struct{})
	go func() {
		defer close(callDone)
		_, _ = ipc.Call(ctx, h.data, ipc.Request{Kind: ipc.KindRemote, Argv: []string{"on"}}, nil, nopWriter{}, nopWriter{}, nil)
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the question never reached the phone")
	}
	cancel()
	defer func() {
		select {
		case <-callDone:
		case <-time.After(10 * time.Second):
			t.Error("the client call did not return after cancelling")
		}
	}()
	select {
	case err := <-ended:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("the question ended with %v, want a cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the question stayed open after the client left")
	}
	waitUntil(t, "the question to be released", func() bool {
		h.l.mu.Lock()
		defer h.l.mu.Unlock()
		return !h.l.askBusy
	})
	if len(h.g.phone.called()) != 0 || !h.g.store.Until().IsZero() {
		t.Fatalf("phone %v", h.g.phone.called())
	}
}

// A pause from the phone withdraws the question of `on`.
func TestPauseWithdrawsTheRemoteQuestion(t *testing.T) {
	r := newFilesRig(t, nil)
	g := newRemoteRig(t, withTailscale(), "41234")
	r.l.remote = g.r
	r.l.useRemoteAsk("en", time.Minute)
	d := remoteDaemon(t, r.l, nil)
	r.l.OnPause(d.CancelConfirmations) // as serve wires it
	done := make(chan ipc.Exit, 1)
	go func() {
		done <- r.l.handleRemote(context.Background(), ipc.Request{Kind: ipc.KindRemote, Argv: []string{"on"}}, nil, d, nil)
	}()
	for {
		if m := readApp(t, r.c); m.Type == bridge.TypeConfirmReq {
			break
		}
	}
	r.l.onPauseMessage(bridge.Message{Type: bridge.TypePause})
	select {
	case exit := <-done:
		if exit.Code != 126 || exit.ErrorCode != "REMOTE_DENIED" {
			t.Fatalf("%+v", exit)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the pause did not withdraw the question")
	}
	if len(g.phone.called()) != 0 {
		t.Fatalf("calls %v", g.phone.called())
	}
}

// No phone connected is REMOTE_UNAVAILABLE (spec §8), after the question.
func TestRemoteOnWithoutPhoneIsUnavailable(t *testing.T) {
	for _, why := range []string{"adb tcpip: error: no devices/emulators found", "adb tcpip: error: device offline", "no phone connected"} {
		h := newRemoteHost(t, withTailscale(), confirm.Allow)
		h.g.phone.mu.Lock()
		h.g.phone.stderr = why
		h.g.phone.mu.Unlock()
		h.g.phone.setFail("tcpip 5555", true)
		h.g.phone.setFail("shell getprop service.adb.tcp.port", true) // no phone: nothing answers
		exit, _ := h.call(t, "on", nil)
		if exit.Code != 1 || exit.ErrorCode != "REMOTE_UNAVAILABLE" {
			t.Errorf("%q: %+v", why, exit)
		}
		// Final review, minor 2: the order never reached the phone, so the
		// mode is off, not "on" for 12 hours.
		if !h.g.store.Until().IsZero() || strings.Contains(exit.Error, "still on") {
			t.Errorf("%q: until %v, %q", why, h.g.store.Until(), exit.Error)
		}
		if _, info := h.call(t, "status", nil); info.Enabled {
			t.Errorf("%q: status %+v", why, info)
		}
	}
	// Any other failure stays REMOTE_FAILED; if the port cannot be read the
	// order may have arrived: the mode stays on, and it says so.
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	h.g.phone.setFail("tcpip 5555", true)
	h.g.phone.setFail("shell getprop service.adb.tcp.port", true)
	exit, _ := h.call(t, "on", nil)
	if exit.ErrorCode != "REMOTE_FAILED" || !strings.Contains(exit.Error, "(the remote mode is still on)") || h.g.store.Until().IsZero() {
		t.Fatalf("%+v until %v", exit, h.g.store.Until())
	}
}

// A no-phone answer while the mode stays on (the expiry could not be
// cleared) says it is still on.
func TestRemoteUnavailableSaysStillOn(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	if exit, _ := h.call(t, "on", nil); exit.Code != 0 {
		t.Fatalf("%+v", exit)
	}
	h.g.phone.mu.Lock()
	h.g.phone.stderr = "error: no devices/emulators found"
	h.g.phone.mu.Unlock()
	h.g.phone.setFail("usb", true)
	exit, _ := h.call(t, "off", nil)
	if exit.ErrorCode != "REMOTE_UNAVAILABLE" || !strings.Contains(exit.Error, "(the remote mode is still on)") {
		t.Fatalf("%+v", exit)
	}
}
