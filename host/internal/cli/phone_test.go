package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/paths"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// appPhone is a phone with the app installed, in the process.
type appPhone struct {
	mu      sync.Mutex
	calls   []string
	nonce   string
	tokens  []string // handed over in each handshake that got through
	reverse bool
	// dropReverse: adb loses the reverse right after a check finds it, as
	// when the phone reconnects.
	dropReverse bool
	// fail makes the commands containing a key fail that many times (-1:
	// always), with an escape sequence in adb's error.
	fail map[string]int
	// onCall runs first, outside the lock; an error is adb's.
	onCall func(joined string) error
}

func (f *appPhone) Run(ctx context.Context, _ string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	joined := strings.Join(args, " ")
	if f.onCall != nil {
		if err := f.onCall(joined); err != nil {
			return -1, err
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, joined)
	for k, n := range f.fail {
		if strings.Contains(joined, k) && n != 0 {
			if n > 0 {
				f.fail[k] = n - 1
			}
			io.WriteString(stderr, "broken \x1b[31mpipe")
			return 1, nil
		}
	}
	switch {
	case strings.Contains(joined, "dumpsys package"):
		io.WriteString(stdout, "Packages:\n    versionName=0.2.0\n")
	case strings.HasPrefix(joined, "reverse tcp:"):
		f.reverse = true
	case joined == "reverse --list":
		if f.reverse {
			f.reverse = !f.dropReverse
			io.WriteString(stdout, "dev1 "+strings.TrimPrefix(f.lastReverse(), "reverse ")+"\n")
		}
	case strings.Contains(joined, "content write"):
		var hs struct{ Token, Nonce string }
		b, _ := io.ReadAll(stdin)
		json.Unmarshal(b, &hs)
		f.nonce = hs.Nonce
		f.tokens = append(f.tokens, hs.Token)
	case strings.Contains(joined, "--method ping"):
		port := strings.TrimPrefix(strings.Fields(f.lastReverse())[1], "tcp:")
		io.WriteString(stdout, "Result: Bundle[{ok=true, port="+port+", nonce="+f.nonce+"}]\n")
	}
	return 0, nil
}

func (f *appPhone) lastReverse() string {
	for i := len(f.calls) - 1; i >= 0; i-- {
		if strings.HasPrefix(f.calls[i], "reverse tcp:") {
			return f.calls[i]
		}
	}
	return ""
}

func (f *appPhone) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (f *appPhone) log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.calls...)
}

func (f *appPhone) handedTokens() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.tokens...)
}

// linkEnv is an environment with its own config folder and a free chat port.
func linkEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	e := &env{dirs: paths.Dirs{Config: dir, Data: dir}, cfg: config.Default()}
	e.cfg.ChatPort = freeTCPPort(t)
	return e
}

// startLink runs startPhoneLink until ctx ends (the test's end).
func startLink(t *testing.T, e *env, f *appPhone, fallback confirm.Confirmer) (*phoneLink, *syncBuffer, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var errb syncBuffer
	app := &App{Stderr: &errb, Stdout: io.Discard, Getenv: func(string) string { return "" }}
	l, err := startPhoneLink(ctx, app, e, f, "s_1", "claude", fallback)
	return l, &errb, err
}

// dialApp connects to the channel as the app, with tok, and returns the
// first message (a welcome, or nothing when refused) and the close code.
func dialApp(t *testing.T, port int, tok string) (string, websocket.StatusCode) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws://127.0.0.1:"+strconv.Itoa(port)+"/v1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	raw, _ := json.Marshal(bridge.Hello{Protocol: bridge.ProtocolVersion, Token: tok, AppVersion: "0.2.0"})
	data, _ := json.Marshal(bridge.Message{V: bridge.ProtocolVersion, Type: bridge.TypeHello, TS: time.Now(), Payload: raw})
	if err := c.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}
	_, data, err = c.Read(ctx)
	if err != nil {
		return "", websocket.CloseStatus(err)
	}
	var m bridge.Message
	json.Unmarshal(data, &m)
	return m.Type, -1
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The chat port is chosen on first use, between 20000 and 59999, and saved:
// the next serve uses the same one.
func TestChatPortIsChosenOnceAndSaved(t *testing.T) {
	dir := t.TempDir()
	e := &env{dirs: paths.Dirs{Config: dir, Data: dir}, cfg: config.Default()}
	port, err := chatPort(e)
	if err != nil || port < 20000 || port > 59999 {
		t.Fatalf("%d %v", port, err)
	}
	saved, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil || saved.ChatPort != port {
		t.Fatalf("saved %+v %v", saved, err)
	}
	again, err := chatPort(&env{dirs: e.dirs, cfg: saved})
	if err != nil || again != port {
		t.Fatalf("second time %d %v", again, err)
	}
}

// serve checks the tunnel every tunnelCheck and, when adb lost it, opens it
// again and hands a token over again.
func TestTheTunnelIsReopened(t *testing.T) {
	setVar(t, &tunnelCheck, 50*time.Millisecond)
	e := linkEnv(t)
	f := &appPhone{dropReverse: true}
	_, errb, err := startLink(t, e, f, confirm.DenyAll{})
	if err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the tunnel reopened", func() bool { return f.count("reverse tcp:") >= 2 && f.count("shell content write") >= 2 })
	if strings.Contains(errb.String(), "Could not") {
		t.Fatalf("stderr %q", errb.String())
	}
	want := "reverse tcp:" + strconv.Itoa(e.cfg.ChatPort) + " tcp:" + strconv.Itoa(e.cfg.ChatPort)
	if f.count(want) < 2 {
		t.Fatalf("calls %q", f.log())
	}
}

// T5 review, I2: each connect() hands over a new token, and only the last
// one gets in: a token captured earlier (while the tunnel was down, say) is
// worth nothing.
func TestEachConnectHandsOverANewToken(t *testing.T) {
	setVar(t, &tunnelCheck, 50*time.Millisecond)
	e := linkEnv(t)
	f := &appPhone{dropReverse: true}
	if _, _, err := startLink(t, e, f, confirm.DenyAll{}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "two handshakes", func() bool { return len(f.handedTokens()) >= 2 })
	tokens := f.handedTokens()
	if tokens[0] == tokens[1] || len(tokens[0]) != 64 {
		t.Fatalf("tokens %q", tokens)
	}
	// Stop the reconnections, then only the last token gets in.
	f.mu.Lock()
	f.dropReverse, f.reverse = false, true
	f.mu.Unlock()
	time.Sleep(150 * time.Millisecond)
	tokens = f.handedTokens()
	last := tokens[len(tokens)-1]
	if _, code := dialApp(t, e.cfg.ChatPort, tokens[0]); code != websocket.StatusPolicyViolation {
		t.Fatalf("the first token: close %v, want 1008", code)
	}
	if typ, _ := dialApp(t, e.cfg.ChatPort, last); typ != bridge.TypeWelcome {
		t.Fatalf("the last token: %q", typ)
	}
}

// T5 review, I1: a handshake that failed with the tunnel in place is tried
// again on the next tick, and the app gets in without restarting serve.
// Minor 4: adb's error is escaped.
func TestAFailedHandshakeIsRetried(t *testing.T) {
	setVar(t, &tunnelCheck, 50*time.Millisecond)
	e := linkEnv(t)
	f := &appPhone{fail: map[string]int{"content write": 1}}
	_, errb, err := startLink(t, e, f, confirm.DenyAll{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errb.String(), "Could not reach the phone app") || strings.Contains(errb.String(), "\x1b") {
		t.Fatalf("stderr %q", errb.String())
	}
	waitUntil(t, "a second handshake", func() bool { return len(f.handedTokens()) >= 1 })
	tokens := f.handedTokens()
	if typ, _ := dialApp(t, e.cfg.ChatPort, tokens[len(tokens)-1]); typ != bridge.TypeWelcome {
		t.Fatalf("after the retry: %q", typ)
	}
	// Once it worked, a tunnel in place is not handed over again.
	n := f.count("shell content write")
	time.Sleep(200 * time.Millisecond)
	if f.count("shell content write") != n {
		t.Fatalf("handed over again with the tunnel in place: %q", f.log())
	}
}

// recording is a fallback that says it was asked.
type recording struct{ asked int }

func (r *recording) Confirm(context.Context, confirm.Request) confirm.Decision {
	r.asked++
	return confirm.Allow
}

// T5 review, doubt 4 and I3: no failure of the app's channel stops serve.
// It goes on without the app (confirmations here) and says what failed.
func TestChannelFailuresDoNotStopServe(t *testing.T) {
	cases := map[string]struct {
		prepare func(t *testing.T, e *env, f *appPhone)
		want    []string
	}{
		"installed version unreadable": {
			prepare: func(t *testing.T, e *env, f *appPhone) { f.fail = map[string]int{"dumpsys package": -1} },
			want: []string{"Could not check whether the ADBuddy app is on the phone",
				"off for the rest of this session", "restart `adbuddy serve` to use it"},
		},
		"chat port not saved": {
			prepare: func(t *testing.T, e *env, f *appPhone) {
				e.cfg.ChatPort = 0
				file := filepath.Join(t.TempDir(), "file")
				os.WriteFile(file, nil, 0o600)
				e.dirs.Config = filepath.Join(file, "cfg") // cannot be created
			},
			want: []string{"Could not choose a port for the phone app's channel", "chat_port"},
		},
		"token": {
			prepare: func(t *testing.T, e *env, f *appPhone) {
				setVar(t, &newToken, func() (string, error) { return "", errors.New("no randomness") })
			},
			want: []string{"Could not reach the phone app", "no randomness"},
		},
		"port in use": {
			prepare: func(t *testing.T, e *env, f *appPhone) {
				l, err := bridge.Listen(e.cfg.ChatPort)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { l.Close() })
			},
			want: []string{"is in use", "change chat_port in"},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := linkEnv(t)
			f := &appPhone{}
			c.prepare(t, e, f)
			fb := &recording{}
			l, errb, err := startLink(t, e, f, fb)
			if err != nil || l == nil {
				t.Fatalf("serve stops: %v", err)
			}
			for _, w := range c.want {
				if !strings.Contains(errb.String(), w) {
					t.Fatalf("stderr %q lacks %q", errb.String(), w)
				}
			}
			if strings.Contains(errb.String(), "\x1b") {
				t.Fatalf("unescaped: %q", errb.String())
			}
			if d := l.Confirmer().Confirm(context.Background(), confirm.Request{ID: "j_1", ExpiresAt: time.Now().Add(time.Minute)}); d != confirm.Allow || fb.asked != 1 {
				t.Fatalf("not confirmed here: %v %d", d, fb.asked)
			}
			if l.Paused() {
				t.Fatal("paused")
			}
		})
	}
}

// Minor 2: Ctrl+C while the channel starts ends with INTERRUPTED, without
// the warnings of a channel that failed.
func TestInterruptedWhileStartingTheLink(t *testing.T) {
	for _, at := range []string{"dumpsys package", "content write"} {
		t.Run(at, func(t *testing.T) {
			e := linkEnv(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := &appPhone{onCall: func(joined string) error {
				if strings.Contains(joined, at) {
					cancel()
					return ctx.Err()
				}
				return nil
			}}
			var errb syncBuffer
			app := &App{Stderr: &errb, Stdout: io.Discard, Getenv: func(string) string { return "" }}
			_, err := startPhoneLink(ctx, app, e, f, "s_1", "claude", confirm.DenyAll{})
			if !hasCode(err, "INTERRUPTED") {
				t.Fatalf("err %v", err)
			}
			if errb.String() != "" {
				t.Fatalf("stderr %q", errb.String())
			}
		})
	}
}

// Minor 2, through serve: Ctrl+C during the handshake exits with 130 and
// serve never says it is serving.
func TestServeInterruptedDuringTheHandshake(t *testing.T) {
	shortWaits(t)
	cfg := config.Default()
	cfg.Serial, cfg.Address = phoneSerial, tsAddr
	cfg.ChatPort = freeTCPPort(t)
	rules := append([]testutil.FakeRule{
		{Match: `^shell dumpsys package tech\.hybriq\.adbuddy$`, Stdout: "Packages:\n    versionName=0.2.0\n"},
		{Match: `^shell content write `, SleepMs: 20000},
	}, remoteRules("AAAA1111")...)
	isolate(t)
	if err := config.Save(filepath.Join(os.Getenv("ADBUDDY_CONFIG_DIR"), "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
	_, log := invariantRunner(t, rules...)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for ctx.Err() == nil {
			if strings.Contains(strings.Join(loggedCalls(t, log), "|"), "content write") {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	var out, errb syncBuffer
	done := make(chan int, 1)
	go func() {
		done <- mainContext(ctx, []string{"serve", "--adb", fakeADB(t), "--workspace", filepath.Join(t.TempDir(), "ws"), "--confirm", "deny"}, strings.NewReader(""), &out, &errb)
	}()
	select {
	case code := <-done:
		if code != ExitInterrupt || !strings.Contains(errb.String(), "INTERRUPTED") || strings.Contains(errb.String(), "ADBuddy is serving") || strings.Contains(errb.String(), "Could not reach") {
			t.Fatalf("exit %d stderr %q", code, errb.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not stop")
	}
}

// I3 through serve: with the chat port taken, serve still serves.
func TestServeWithTheChatPortTaken(t *testing.T) {
	shortWaits(t)
	cfg := config.Default()
	cfg.Serial, cfg.Address = phoneSerial, tsAddr
	cfg.ChatPort = freeTCPPort(t)
	l, err := bridge.Listen(cfg.ChatPort)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	rules := append([]testutil.FakeRule{
		{Match: `^shell dumpsys package tech\.hybriq\.adbuddy$`, Stdout: "Packages:\n    versionName=0.2.0\n"},
	}, remoteRules("AAAA1111")...)
	code, errOut, _, _ := serveOnce(t, rules, cfg)
	if code != 0 || !strings.Contains(errOut, "ADBuddy is serving") || !strings.Contains(errOut, "change chat_port in") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}

// freeTCPPort returns a loopback port that was free a moment ago.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
