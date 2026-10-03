package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/uidump"
)

// The guard reads the shade (and, without the app, the system windows) at
// the same time as it asks what is in front (plan 2026-10-04, guarda en
// paralelo). These tests use their own executor: fakeADB holds its lock for
// the whole call and ignores the context.

// gateReply is one answer of gateADB.
type gateReply struct {
	out  string
	code int
	err  error
}

// gateADB is an executor for the guard's own reads. Answers and hooks are set
// before the guard runs and only read after; calls are recorded under mu,
// and a hook runs outside it (that is where the tests meet, with channels).
// Every Shell looks at ctx after its hook: a cancelled read ends with ctx's
// error and is recorded in done.
type gateADB struct {
	mu      sync.Mutex
	replies map[string][]gateReply // by command, one per call; the last repeats
	hooks   map[string]func(ctx context.Context, n int)
	runs    []string
	done    []string // the commands whose Shell saw ctx cancelled
}

func newGateADB() *gateADB {
	return &gateADB{replies: map[string][]gateReply{}, hooks: map[string]func(context.Context, int){}}
}

// answer sets the replies to cmd, in order.
func (g *gateADB) answer(cmd string, rs ...gateReply) { g.replies[cmd] = rs }

func (g *gateADB) Shell(ctx context.Context, cmd string) (string, int, error) {
	g.mu.Lock()
	n := 0
	for _, r := range g.runs {
		if r == cmd {
			n++
		}
	}
	g.runs = append(g.runs, cmd)
	rs, hook := g.replies[cmd], g.hooks[cmd]
	g.mu.Unlock()
	if hook != nil {
		hook(ctx, n)
	}
	if err := ctx.Err(); err != nil {
		g.mu.Lock()
		g.done = append(g.done, cmd)
		g.mu.Unlock()
		return "", -1, err
	}
	if len(rs) == 0 {
		return "", 1, nil
	}
	r := rs[min(n, len(rs)-1)]
	return r.out, r.code, r.err
}

func (g *gateADB) Pull(context.Context, string, string) error { return errors.New("no pull") }

func (g *gateADB) Run(context.Context, string, []string, io.Reader, io.Writer, io.Writer) (int, error) {
	return 1, errors.New("no run in the guard's tests")
}

func (g *gateADB) ran() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.runs)
}

func (g *gateADB) cancelled() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.done)
}

// count is how many times cmd reached the executor.
func (g *gateADB) count(cmd string) int {
	n := 0
	for _, r := range g.ran() {
		if r == cmd {
			n++
		}
	}
	return n
}

// gateApp is the app's foreground for the guard: fg, or err, after hook
// (outside any lock). calls counts the questions.
type gateApp struct {
	fg   bridge.UIForeground
	err  error
	hook func(ctx context.Context)
	mu   sync.Mutex
	n    int
}

func (a *gateApp) ui(ctx context.Context, op string, _ json.RawMessage) (json.RawMessage, error) {
	a.mu.Lock()
	a.n++
	a.mu.Unlock()
	if op != bridge.UIOpForeground {
		return nil, errors.New("only the foreground in the guard's tests")
	}
	if a.hook != nil {
		a.hook(ctx)
	}
	if a.err != nil {
		return nil, a.err
	}
	return json.Marshal(a.fg)
}

func (a *gateApp) calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.n
}

// settingsApp is the app saying Settings, not protected, is in front.
func settingsApp() *gateApp {
	return &gateApp{fg: bridge.UIForeground{Package: "com.android.settings", Activity: "com.android.settings.Settings"}}
}

// gateDaemon is a daemon whose guard reads g and, when app is not nil, asks
// app; protected are the user's protected apps.
func gateDaemon(g *gateADB, app *gateApp, protected ...string) *Daemon {
	o := Options{ADB: g, Protected: func(pkg string) bool { return slices.Contains(protected, pkg) }}
	if app != nil {
		o.UI = app.ui
	}
	return New(o)
}

// quietPhone answers the guard's reads with Settings in front, an app in
// focus and no system window.
func quietPhone() *gateADB {
	g := newGateADB()
	g.answer("dumpsys activity activities", gateReply{out: settingsInFront})
	g.answer(fixedFocusCmd, gateReply{out: focusApp})
	g.answer(fixedWindowsCmd, gateReply{out: windowsQuiet})
	g.answer("dumpsys power", gateReply{out: powerAwake})
	return g
}

// meeting is a point where want goroutines meet: arrive returns false when the
// others did not arrive within 2 s.
type meeting struct {
	mu   sync.Mutex
	n    int
	want int
	all  chan struct{}
}

func newMeeting(want int) *meeting { return &meeting{want: want, all: make(chan struct{})} }

func (m *meeting) arrive() bool {
	m.mu.Lock()
	m.n++
	if m.n == m.want {
		close(m.all)
	}
	m.mu.Unlock()
	select {
	case <-m.all:
		return true
	case <-time.After(2 * time.Second):
		return false
	}
}

// series records that a meeting failed.
type series struct {
	mu  sync.Mutex
	who []string
}

func (s *series) add(who string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.who = append(s.who, who)
}

func (s *series) list() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.who)
}

var tapWords = []string{"input", "tap", "1", "1"}

func TestGuardReadsRunTogether(t *testing.T) {
	t.Run("with the app", func(t *testing.T) {
		g, app := quietPhone(), settingsApp()
		m, late := newMeeting(2), &series{}
		app.hook = func(context.Context) {
			if !m.arrive() {
				late.add("foreground")
			}
		}
		g.hooks[fixedFocusCmd] = func(context.Context, int) {
			if !m.arrive() {
				late.add("focus")
			}
		}
		if why := gateDaemon(g, app).guardForeground(context.Background(), tapWords); why != "" {
			t.Fatalf("blocked: %s", why)
		}
		if l := late.list(); len(l) != 0 {
			t.Fatalf("the foreground and the shade ran in series (%q waited alone)", l)
		}
		if r := g.ran(); !slices.Equal(r, []string{fixedFocusCmd}) {
			t.Fatalf("adb ran %q", r)
		}
	})
	t.Run("without the app", func(t *testing.T) {
		g := quietPhone()
		m, late := newMeeting(3), &series{}
		for _, cmd := range []string{"dumpsys activity activities", fixedFocusCmd, fixedWindowsCmd} {
			g.hooks[cmd] = func(context.Context, int) {
				if !m.arrive() {
					late.add(cmd)
				}
			}
		}
		if why := gateDaemon(g, nil).guardForeground(context.Background(), tapWords); why != "" {
			t.Fatalf("blocked: %s", why)
		}
		if l := late.list(); len(l) != 0 {
			t.Fatalf("the reads ran in series (%q waited alone)", l)
		}
		r := g.ran()
		slices.Sort(r)
		if want := []string{"dumpsys activity activities", fixedWindowsCmd, fixedFocusCmd}; !slices.Equal(r, want) {
			t.Fatalf("adb ran %q, want %q", r, want)
		}
	})
}

const (
	shadeUnknown   = "could not tell whether the notification shade is open; ADBuddy does not touch or read the screen without knowing"
	windowsUnknown = "could not tell whether a system window is on screen; ADBuddy does not touch or read the screen without knowing"
	frontUnknown   = "could not tell which app is in the foreground ("
	shadeOpen      = "the notification shade is open; close it with Back (input keyevent 4) or ask the user to do this part"
)

func TestGuardBothAnswersAreRequired(t *testing.T) {
	broken := gateReply{err: errors.New("adb: device offline")}
	for _, c := range []struct {
		name    string
		focus   gateReply
		appErr  bool // the app does not say what is in front
		dumpsys bool // dumpsys activity fails too
		want    string
	}{
		{name: "focus fails", focus: broken, want: shadeUnknown},
		{name: "focus exits non-zero", focus: gateReply{out: focusApp, code: 1}, want: shadeUnknown},
		{name: "focus cannot be read", focus: gateReply{out: "Unknown command\n"}, want: shadeUnknown},
		{name: "focus fine, the app fails", focus: gateReply{out: focusApp}, appErr: true, want: ""},
		{name: "focus fine, nothing says what is in front", focus: gateReply{out: focusApp}, appErr: true, dumpsys: true, want: frontUnknown},
		{name: "both fail", focus: broken, appErr: true, dumpsys: true, want: frontUnknown},
	} {
		t.Run(c.name, func(t *testing.T) {
			g, app := quietPhone(), settingsApp()
			g.answer(fixedFocusCmd, c.focus)
			if c.appErr {
				app.err = errors.New("the app is not linked")
			}
			if c.dumpsys {
				g.answer("dumpsys activity activities", broken)
			}
			why := gateDaemon(g, app).guardForeground(context.Background(), tapWords)
			if c.want == frontUnknown && !strings.HasPrefix(why, frontUnknown) || c.want != frontUnknown && why != c.want {
				t.Fatalf("got %q, want %q", why, c.want)
			}
		})
	}
	for name, windows := range map[string]gateReply{
		"fails":          broken,
		"exits non-zero": {out: windowsQuiet, code: 1},
		"cannot be read": {out: "Unknown command\n"},
	} {
		t.Run("without the app, the system windows "+name, func(t *testing.T) {
			g := quietPhone()
			g.answer(fixedWindowsCmd, windows)
			if why := gateDaemon(g, nil).guardForeground(context.Background(), tapWords); why != windowsUnknown {
				t.Fatalf("got %q", why)
			}
		})
	}
}

func TestGuardOrderOfReasons(t *testing.T) {
	broken := gateReply{err: errors.New("adb: device offline")}
	t.Run("a protected app before an unreadable shade", func(t *testing.T) {
		g := quietPhone()
		g.answer(fixedFocusCmd, broken)
		app := &gateApp{fg: bridge.UIForeground{Package: "com.bank", Activity: "com.bank.Main"}}
		if why := gateDaemon(g, app, "com.bank").guardForeground(context.Background(), tapWords); why != protectedReason("com.bank") {
			t.Fatalf("got %q", why)
		}
		if r := g.ran(); !slices.Equal(r, []string{fixedFocusCmd}) {
			t.Fatalf("adb ran %q, want only the shade", r)
		}
	})
	t.Run("ADBuddy in front before the open shade", func(t *testing.T) {
		g := quietPhone()
		g.answer(fixedFocusCmd, gateReply{out: focusShade})
		app := &gateApp{fg: bridge.UIForeground{Package: uidump.AppPackage, Activity: uidump.AppPackage + ".MainActivity"}}
		if why := gateDaemon(g, app).guardForeground(context.Background(), tapWords); why != adbuddyInFrontReason(true) {
			t.Fatalf("got %q", why)
		}
	})
	t.Run("the open shade alone", func(t *testing.T) {
		g := quietPhone()
		g.answer(fixedFocusCmd, gateReply{out: focusShade})
		if why := gateDaemon(g, settingsApp()).guardForeground(context.Background(), tapWords); why != shadeOpen {
			t.Fatalf("got %q", why)
		}
	})
	t.Run("a protected screen before an unreadable shade", func(t *testing.T) {
		g := quietPhone()
		g.answer(fixedFocusCmd, broken)
		app := settingsApp()
		app.fg.Protected = true
		if why := gateDaemon(g, app).guardForeground(context.Background(), tapWords); why != screenProtectedReason("com.android.settings") {
			t.Fatalf("got %q", why)
		}
	})
}

// blockUntilCancelled is a hook that holds a read until its context ends, and
// a little longer, so a guard that did not wait for it would return first.
func blockUntilCancelled(ctx context.Context, _ int) {
	<-ctx.Done()
	time.Sleep(30 * time.Millisecond)
}

func TestGuardStopsAndWaitsForItsReads(t *testing.T) {
	check := func(t *testing.T, g *gateADB, d *Daemon, ctx context.Context, wantWhy string, held ...string) {
		t.Helper()
		why := d.guardForeground(ctx, tapWords)
		if !strings.HasPrefix(why, wantWhy) {
			t.Fatalf("got %q, want %q", why, wantWhy)
		}
		got := g.cancelled()
		slices.Sort(got)
		slices.Sort(held)
		if !slices.Equal(got, held) {
			t.Fatalf("the guard returned before its reads %q saw the cancel (saw: %q)", held, got)
		}
		n := len(g.ran())
		time.Sleep(50 * time.Millisecond)
		if r := g.ran(); len(r) != n {
			t.Fatalf("adb ran %q after the guard returned", r[n:])
		}
	}
	t.Run("a protected app in front", func(t *testing.T) {
		g := quietPhone()
		g.hooks[fixedFocusCmd] = blockUntilCancelled
		app := &gateApp{fg: bridge.UIForeground{Package: "com.bank"}}
		check(t, g, gateDaemon(g, app, "com.bank"), context.Background(), protectedReason("com.bank"), fixedFocusCmd)
	})
	t.Run("nothing says what is in front, with the app", func(t *testing.T) {
		g := quietPhone()
		g.answer("dumpsys activity activities", gateReply{code: 1})
		g.hooks[fixedFocusCmd] = blockUntilCancelled
		app := settingsApp()
		app.err = errors.New("the app is not linked")
		check(t, g, gateDaemon(g, app), context.Background(), frontUnknown, fixedFocusCmd)
	})
	t.Run("nothing says what is in front, without the app", func(t *testing.T) {
		g := quietPhone()
		g.answer("dumpsys activity activities", gateReply{code: 1})
		g.hooks[fixedFocusCmd] = blockUntilCancelled
		g.hooks[fixedWindowsCmd] = blockUntilCancelled
		check(t, g, gateDaemon(g, nil), context.Background(), frontUnknown, fixedFocusCmd, fixedWindowsCmd)
	})
	t.Run("the context ends while the app is asked", func(t *testing.T) {
		g := quietPhone()
		g.hooks[fixedFocusCmd] = blockUntilCancelled
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		app := settingsApp()
		app.hook = func(context.Context) { cancel() }
		app.err = context.Canceled
		check(t, g, gateDaemon(g, app), ctx, frontUnknown+"context canceled)", fixedFocusCmd)
	})
	t.Run("the context is over already", func(t *testing.T) {
		for _, app := range []*gateApp{settingsApp(), nil} {
			g := quietPhone()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			d := gateDaemon(g, app)
			if why := d.guardForeground(ctx, tapWords); !strings.HasPrefix(why, frontUnknown+"context canceled)") {
				t.Fatalf("app=%v: got %q", app != nil, why)
			}
			if r := g.ran(); len(r) != 0 {
				t.Fatalf("app=%v: adb ran %q", app != nil, r)
			}
			if app != nil && app.calls() != 0 {
				t.Fatalf("the app was asked %d times", app.calls())
			}
		}
	})
}

// shortSlack sets focusSlack to slack for the test.
func shortSlack(t *testing.T, slack time.Duration) {
	t.Helper()
	old := focusSlack
	focusSlack = slack
	t.Cleanup(func() { focusSlack = old })
}

func TestGuardRereadsStaleReads(t *testing.T) {
	const slack = 20 * time.Millisecond
	// meet closes the channel on the first call of a read (hook of the
	// executor); wait blocks until every one of them is closed, and then
	// lets more than focusSlack go by: the first reads are surely stale
	// when the slow one ends, whatever the load of the machine.
	meet := func() (chan struct{}, func(context.Context, int)) {
		ch := make(chan struct{})
		return ch, func(_ context.Context, n int) {
			if n == 0 {
				close(ch)
			}
		}
	}
	wait := func(t *testing.T, chs ...chan struct{}) {
		t.Helper()
		for _, ch := range chs {
			select {
			case <-ch:
			case <-time.After(5 * time.Second):
				t.Error("the first read never happened")
				return
			}
		}
		time.Sleep(slack * 5)
	}
	t.Run("the shade, behind a slow app", func(t *testing.T) {
		shortSlack(t, slack)
		g, app := quietPhone(), settingsApp()
		read, hook := meet()
		g.hooks[fixedFocusCmd] = hook
		app.hook = func(context.Context) { wait(t, read) }
		g.answer(fixedFocusCmd, gateReply{out: focusApp}, gateReply{out: focusShade})
		if why := gateDaemon(g, app).guardForeground(context.Background(), tapWords); why != shadeOpen {
			t.Fatalf("got %q", why)
		}
		if n := g.count(fixedFocusCmd); n != 2 {
			t.Fatalf("the shade was read %d times, want 2", n)
		}
	})
	t.Run("the shade, behind a quick app", func(t *testing.T) {
		shortSlack(t, time.Minute)
		g := quietPhone()
		g.answer(fixedFocusCmd, gateReply{out: focusApp}, gateReply{out: focusShade})
		if why := gateDaemon(g, settingsApp()).guardForeground(context.Background(), tapWords); why != "" {
			t.Fatalf("got %q", why)
		}
		if n := g.count(fixedFocusCmd); n != 1 {
			t.Fatalf("the shade was read %d times, want 1", n)
		}
	})
	t.Run("a heads-up that comes while dumpsys activity is slow", func(t *testing.T) {
		shortSlack(t, slack)
		g := quietPhone()
		readF, hookF := meet()
		readW, hookW := meet()
		g.hooks[fixedFocusCmd] = hookF
		g.hooks[fixedWindowsCmd] = hookW
		g.hooks["dumpsys activity activities"] = func(context.Context, int) { wait(t, readF, readW) }
		g.answer(fixedWindowsCmd, gateReply{out: windowsQuiet}, gateReply{out: windowsHeadsUp})
		why := gateDaemon(g, nil).guardForeground(context.Background(), tapWords)
		if !strings.HasPrefix(why, "a system window is on screen (") {
			t.Fatalf("got %q", why)
		}
		if n := g.count(fixedWindowsCmd); n != 2 {
			t.Fatalf("the system windows were read %d times, want 2", n)
		}
		if n := g.count(fixedFocusCmd); n != 2 {
			t.Fatalf("the shade was read %d times, want 2", n)
		}
	})
	t.Run("the system windows, behind a quick dumpsys", func(t *testing.T) {
		shortSlack(t, time.Minute)
		g := quietPhone()
		g.answer(fixedWindowsCmd, gateReply{out: windowsQuiet}, gateReply{out: windowsHeadsUp})
		if why := gateDaemon(g, nil).guardForeground(context.Background(), tapWords); why != "" {
			t.Fatalf("got %q", why)
		}
		if n := g.count(fixedWindowsCmd); n != 1 {
			t.Fatalf("the system windows were read %d times, want 1", n)
		}
	})
}

func TestGuardWithoutTheAppIgnoresTheWindowsWhereItDidBefore(t *testing.T) {
	broken := gateReply{err: errors.New("adb: device offline")}
	t.Run("a key that closes the shade", func(t *testing.T) {
		g := quietPhone()
		g.answer(fixedWindowsCmd, broken)
		if why := gateDaemon(g, nil).guardForeground(context.Background(), []string{"input", "keyevent", "4"}); why != "" {
			t.Fatalf("got %q", why)
		}
	})
	t.Run("the wake key with the screen off", func(t *testing.T) {
		g := quietPhone()
		g.answer("dumpsys activity activities", gateReply{out: "ACTIVITY MANAGER ACTIVITIES (dumpsys activity activities)\n"})
		g.answer("dumpsys power", gateReply{out: powerAsleep})
		g.answer(fixedWindowsCmd, broken)
		if why := gateDaemon(g, nil).guardForeground(context.Background(), []string{"input", "keyevent", "224"}); why != "" {
			t.Fatalf("got %q", why)
		}
		// Another key is judged as with the screen on.
		if why := gateDaemon(g, nil).guardForeground(context.Background(), tapWords); why != windowsUnknown {
			t.Fatalf("tap: got %q", why)
		}
	})
}
