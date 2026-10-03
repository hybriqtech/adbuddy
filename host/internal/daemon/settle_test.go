package daemon

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/uidump"
)

// settleTimes sets the waits of settle for a test and restores them after.
func settleTimes(t *testing.T, first, gap, min, max time.Duration, reads int) {
	t.Helper()
	of, og, omin, omax, or := settleFirst, settleGap, settleMin, settleMax, settleReads
	settleFirst, settleGap, settleMin, settleMax, settleReads = first, gap, min, max, reads
	t.Cleanup(func() { settleFirst, settleGap, settleMin, settleMax, settleReads = of, og, omin, omax, or })
}

func dumpOf(pkg string, texts ...string) bridge.UIDump {
	d := bridge.UIDump{Package: pkg, Width: 1344, Height: 2992, Elements: []bridge.UIElement{}}
	for _, t := range texts {
		d.Elements = append(d.Elements, bridge.UIElement{Text: t, Class: "TextView", Bounds: [4]int{0, 0, 10, 10}, Enabled: true, Window: "app"})
	}
	return d
}

// settleApp is an app whose dump answers come from a function of the call
// number (1 for the first) and that records the sequence of operations.
type settleApp struct {
	mu      sync.Mutex
	ops     []string
	counts  map[string]int
	dumpFn  func(n int) bridge.UIDump
	dumpErr error
	// fg answers foreground; afterClick says whether the tap already happened.
	fg func(afterClick bool) bridge.UIForeground
	// failOnCancel makes a dump fail with the context's error once it ended.
	failOnCancel bool
	onOp         func(op string, n int) // called before answering; n counts that op
}

func (a *settleApp) ui(ctx context.Context, op string, _ json.RawMessage) (json.RawMessage, error) {
	a.mu.Lock()
	a.ops = append(a.ops, op)
	if a.counts == nil {
		a.counts = map[string]int{}
	}
	a.counts[op]++
	n := a.counts[op]
	clicked := a.counts[bridge.UIOpClick] > 0
	fn, onOp, fg, derr := a.dumpFn, a.onOp, a.fg, a.dumpErr
	a.mu.Unlock()
	if onOp != nil {
		onOp(op, n)
	}
	switch op {
	case bridge.UIOpForeground:
		if fg != nil {
			return json.Marshal(fg(clicked))
		}
		return json.Marshal(bridge.UIForeground{Package: "com.android.settings", Activity: "com.android.settings.Settings"})
	case bridge.UIOpDump:
		if derr != nil {
			return nil, derr
		}
		if a.failOnCancel && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return json.Marshal(fn(n))
	case bridge.UIOpClick:
		return json.Marshal(bridge.UIClicked{Clicked: true, Bounds: [4]int{0, 0, 10, 10}, How: "action"})
	case bridge.UIOpSetText:
		return json.Marshal(bridge.UITextSet{Set: true})
	}
	return nil, &UIError{Code: "nope", Text: op}
}

func (a *settleApp) count(op string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.counts[op]
}

func (a *settleApp) sequence() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string{}, a.ops...)
}

// afterClick is what the app was asked after the tap itself.
func (a *settleApp) afterClick() []string {
	seq := a.sequence()
	for i, op := range seq {
		if op == bridge.UIOpClick {
			return seq[i+1:]
		}
	}
	return nil
}

func (e *env) settleApp(protected ...string) *settleApp {
	a := &settleApp{dumpFn: func(int) bridge.UIDump { return dumpOf("com.android.settings", "same") }}
	e.d.o.UI = a.ui
	e.d.o.Protected = func(pkg string) bool {
		for _, p := range protected {
			if p == pkg {
				return true
			}
		}
		return false
	}
	return a
}

// helperScreen is helper with Request.Screen set and a context of the test's.
func (e *env) helperScreen(t *testing.T, ctx context.Context, op string, args any) (*stream, ipc.Exit) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	s := &stream{}
	exit := e.d.Handle(ctx, ipc.Request{Kind: ipc.KindUI, UIOp: op, UIArgs: raw, Screen: true,
		Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}, s)
	return s, exit
}

func (e *env) entriesAll(t *testing.T) []journal.Entry {
	t.Helper()
	all, _, err := e.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return all
}

func decodeClicked(t *testing.T, s *stream) ClickedScreen {
	t.Helper()
	if len(s.data) != 1 {
		t.Fatalf("data %q", s.data)
	}
	var c ClickedScreen
	if err := json.Unmarshal(s.data[0], &c); err != nil {
		t.Fatal(err)
	}
	return c
}

const notRepeat = "The action was done; do not repeat it."

func TestSettleStopsWhenTwoReadsMatch(t *testing.T) {
	settleTimes(t, time.Millisecond, time.Millisecond, 0, 2*time.Second, 8)
	e := newEnv(t)
	a := e.settleApp()
	a.dumpFn = func(n int) bridge.UIDump {
		if n == 1 {
			return dumpOf("com.android.settings", "A")
		}
		return dumpOf("com.android.settings", "B")
	}
	s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	c := decodeClicked(t, s)
	if !c.Clicked || c.Screen == nil || !c.Screen.Settled || len(c.Screen.Elements) != 1 || c.Screen.Elements[0].Text != "B" || c.Note != "" {
		t.Fatalf("result %s", s.data[0])
	}
	want := "foreground,dump,foreground,dump,foreground,dump"
	if got := strings.Join(a.afterClick(), ","); got != want {
		t.Fatalf("after the tap: %s, want %s", got, want)
	}
	all := e.entriesAll(t)
	if len(all) < 2 {
		t.Fatalf("journal %+v", all)
	}
	tap, rd := all[len(all)-2], all[len(all)-1]
	if tap.Rule != "helper.tap" || tap.Status != journal.StatusOK || rd.Rule != "helper.screen" || rd.Status != journal.StatusOK ||
		rd.Level != "green" || rd.Source != journal.SourceAgent || strings.Join(rd.Argv, " ") != "adbuddy screen" ||
		!strings.Contains(rd.Reason, "after click") || !strings.Contains(rd.Reason, "settled=true") {
		t.Fatalf("journal %+v / %+v", tap, rd)
	}
}

func TestSettleMinimumTime(t *testing.T) {
	// The A pairs happen far below settleMin, with room to spare on a slow machine.
	settleTimes(t, time.Millisecond, 5*time.Millisecond, time.Second, 10*time.Second, 1000)
	e := newEnv(t)
	a := e.settleApp()
	a.dumpFn = func(n int) bridge.UIDump {
		if n <= 3 {
			return dumpOf("com.android.settings", "A")
		}
		return dumpOf("com.android.settings", "B")
	}
	s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	c := decodeClicked(t, s)
	if c.Screen == nil || !c.Screen.Settled || c.Screen.Elements[0].Text != "B" {
		t.Fatalf("two equal reads before the minimum settled: %s", s.data[0])
	}
}

func TestSettleGivesUpAtTheCap(t *testing.T) {
	for name, times := range map[string]struct {
		max   time.Duration
		reads int
	}{"reads": {5 * time.Second, 4}, "time": {30 * time.Millisecond, 1000}} {
		t.Run(name, func(t *testing.T) {
			settleTimes(t, time.Millisecond, 5*time.Millisecond, 0, times.max, times.reads)
			e := newEnv(t)
			a := e.settleApp()
			a.dumpFn = func(n int) bridge.UIDump { return dumpOf("com.android.settings", "tick "+strconv.Itoa(n)) }
			s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
			if exit.Code != 0 {
				t.Fatalf("exit %+v", exit)
			}
			c := decodeClicked(t, s)
			if c.Screen == nil || c.Screen.Settled || c.Note != "" {
				t.Fatalf("result %s", s.data[0])
			}
			if name == "reads" && a.count(bridge.UIOpDump) != times.reads {
				t.Fatalf("%d reads, want %d", a.count(bridge.UIOpDump), times.reads)
			}
			if j := e.last(t); j.Status != journal.StatusOK || !strings.Contains(j.Reason, "settled=false") {
				t.Fatalf("journal %+v", j)
			}
		})
	}
}

func TestSettleEveryReadIsGuarded(t *testing.T) {
	settleTimes(t, time.Millisecond, time.Millisecond, 0, 2*time.Second, 8)
	frontAfter := func(fg bridge.UIForeground) func(bool) bridge.UIForeground {
		return func(after bool) bridge.UIForeground {
			if after {
				return fg
			}
			return bridge.UIForeground{Package: "com.android.settings"}
		}
	}
	cases := []struct {
		name     string
		setup    func(e *env, a *settleApp)
		blame    string // the journal reason holds it
		noDumps  bool   // no dump may be asked after the tap
		wantExit int
	}{
		{"protected package", func(e *env, a *settleApp) {
			a.fg = frontAfter(bridge.UIForeground{Package: "com.bank.app", Activity: "Main"})
		}, "com.bank.app is a protected app", true, ExitDenied},
		{"ADBuddy in front", func(e *env, a *settleApp) {
			a.fg = frontAfter(bridge.UIForeground{Package: uidump.AppPackage, Activity: "Main"})
		}, "ADBuddy itself is in front", true, ExitDenied},
		{"the shade open", func(e *env, a *settleApp) {
			a.onOp = func(op string, n int) {
				if op == bridge.UIOpClick {
					e.adb.mu.Lock()
					e.adb.shell[fixedFocusCmd] = focusShade
					e.adb.mu.Unlock()
				}
			}
		}, "the notification shade is open", true, ExitDenied},
		{"the app says protected", func(e *env, a *settleApp) {
			a.fg = frontAfter(bridge.UIForeground{Package: "com.android.settings", Protected: true})
		}, "something on the screen is protected", true, ExitDenied},
		{"the dump is protected", func(e *env, a *settleApp) {
			a.dumpErr = &UIError{Code: bridge.CodeProtected, Text: "ADBuddy does not read this screen."}
		}, "ADBuddy does not read this screen", false, ExitDenied},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			a := e.settleApp("com.bank.app")
			c.setup(e, a)
			s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
			if exit.Code != 0 {
				t.Fatalf("the act happened: exit %+v", exit)
			}
			cs := decodeClicked(t, s)
			if !cs.Clicked || cs.Screen != nil || !strings.HasPrefix(cs.Note, notRepeat) || !strings.Contains(cs.Note, "Blocked by ADBuddy: ") {
				t.Fatalf("result %s", s.data[0])
			}
			if n := a.count(bridge.UIOpClick); n != 1 {
				t.Fatalf("%d clicks", n)
			}
			if c.noDumps && a.count(bridge.UIOpDump) != 0 {
				t.Fatalf("a dump was asked after the block: %q", a.sequence())
			}
			all := e.entriesAll(t)
			tap, rd := all[len(all)-2], all[len(all)-1]
			if tap.Rule != "helper.tap" || tap.Status != journal.StatusOK {
				t.Fatalf("tap entry %+v", tap)
			}
			if rd.Rule != "helper.screen" || rd.Status != journal.StatusBlocked || !strings.Contains(rd.Reason, c.blame) || rd.ExitCode == nil || *rd.ExitCode != c.wantExit {
				t.Fatalf("read entry %+v", rd)
			}
		})
	}
}

func TestSettlePausedAndQuestionStopTheReads(t *testing.T) {
	settleTimes(t, time.Millisecond, time.Millisecond, 0, 2*time.Second, 8)
	t.Run("paused", func(t *testing.T) {
		e := newEnv(t)
		a := e.settleApp()
		var mu sync.Mutex
		paused := false
		e.d.o.Paused = func() bool { mu.Lock(); defer mu.Unlock(); return paused }
		a.dumpFn = func(n int) bridge.UIDump { return dumpOf("com.android.settings", "tick "+strconv.Itoa(n)) }
		a.onOp = func(op string, n int) {
			if op == bridge.UIOpDump && n == 2 {
				mu.Lock()
				paused = true
				mu.Unlock()
			}
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		c := decodeClicked(t, s)
		if c.Screen != nil || !strings.HasPrefix(c.Note, notRepeat) || !strings.Contains(c.Note, PausedMessage) {
			t.Fatalf("result %s", s.data[0])
		}
		if n := a.count(bridge.UIOpDump); n != 2 {
			t.Fatalf("%d reads", n)
		}
		if j := e.last(t); j.Status != journal.StatusBlocked || j.Reason != PausedMessage {
			t.Fatalf("journal %+v", j)
		}
	})
	t.Run("question", func(t *testing.T) {
		e := newEnv(t)
		a := e.settleApp()
		a.dumpFn = func(n int) bridge.UIDump { return dumpOf("com.android.settings", "tick "+strconv.Itoa(n)) }
		a.onOp = func(op string, n int) {
			if op == bridge.UIOpDump && n == 2 {
				if busy := e.d.ask(&question{title: "Reboot the phone", withdraw: func(error) {}}); busy != "" {
					t.Error(busy)
				}
			}
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		c := decodeClicked(t, s)
		if c.Screen != nil || !strings.HasPrefix(c.Note, notRepeat) || !strings.Contains(c.Note, `waiting to ask about "Reboot the phone"`) {
			t.Fatalf("result %s", s.data[0])
		}
		if n := a.count(bridge.UIOpDump); n != 2 {
			t.Fatalf("%d reads", n)
		}
		if j := e.last(t); j.Status != journal.StatusBlocked || !strings.Contains(j.Reason, "waiting to ask") {
			t.Fatalf("journal %+v", j)
		}
	})
}

func TestSettleDumpWaitsAndBlocksMidway(t *testing.T) {
	settleTimes(t, time.Millisecond, time.Millisecond, 0, 2*time.Second, 8)
	e := newEnv(t)
	a := e.settleApp("com.bank.app")
	a.dumpFn = func(n int) bridge.UIDump { return dumpOf("com.android.settings", "tick "+strconv.Itoa(n)) }
	a.onOp = func(op string, n int) {}
	a.fg = func(bool) bridge.UIForeground {
		// The helper's own guard is the first foreground; the second read's guard is the second.
		if a.count(bridge.UIOpForeground) >= 2 {
			return bridge.UIForeground{Package: "com.bank.app"}
		}
		return bridge.UIForeground{Package: "com.android.settings"}
	}
	s, exit := e.helperScreen(t, context.Background(), bridge.UIOpDump, bridge.UIDumpArgs{})
	if exit.Code != 126 || exit.ErrorCode != "UI_BLOCKED" || !strings.HasPrefix(exit.Error, "Blocked by ADBuddy: com.bank.app is a protected app") ||
		strings.Count(exit.Error, "Blocked by ADBuddy:") != 1 {
		t.Fatalf("exit %+v", exit)
	}
	if n := a.count(bridge.UIOpDump); len(s.data) != 0 || n != 1 {
		t.Fatalf("data %q, %d dumps: the first read must not come out", s.data, n)
	}
	j := e.last(t)
	if j.Status != journal.StatusBlocked || strings.HasPrefix(j.Reason, "Blocked by ADBuddy:") || !strings.Contains(j.Reason, "com.bank.app is a protected app") {
		t.Fatalf("journal %+v", j)
	}
}

func TestSettleContextCancel(t *testing.T) {
	settleTimes(t, 50*time.Millisecond, 50*time.Millisecond, 0, 2*time.Second, 8)
	t.Run("screen", func(t *testing.T) {
		e := newEnv(t)
		a := e.settleApp()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		a.onOp = func(op string, n int) {
			if op == bridge.UIOpDump {
				cancel()
			}
		}
		s, exit := e.helperScreen(t, ctx, bridge.UIOpDump, bridge.UIDumpArgs{})
		if exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" || len(s.data) != 0 {
			t.Fatalf("exit %+v data %q", exit, s.data)
		}
		if j := e.last(t); j.Status != journal.StatusDenied || !strings.Contains(j.Reason, "abandoned by the agent") {
			t.Fatalf("journal %+v", j)
		}
	})
	t.Run("after a tap, serve stopping", func(t *testing.T) {
		e := newEnv(t)
		a := e.settleApp()
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		a.onOp = func(op string, n int) {
			if op == bridge.UIOpClick {
				cancel(ipc.ErrClosed)
			}
		}
		s, exit := e.helperScreen(t, ctx, bridge.UIOpClick, xy(1, 1))
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		c := decodeClicked(t, s)
		if !c.Clicked || c.Screen != nil || !strings.HasPrefix(c.Note, notRepeat) {
			t.Fatalf("result %s", s.data[0])
		}
		if n := a.count(bridge.UIOpDump); n != 0 {
			t.Fatalf("%d reads after the cancel", n)
		}
		if j := e.last(t); j.Status != journal.StatusDenied || !strings.Contains(j.Reason, "ADBuddy stopped") {
			t.Fatalf("journal %+v", j)
		}
	})
}

func TestSettleCancelInsideTheSecondRead(t *testing.T) {
	settleTimes(t, time.Millisecond, time.Millisecond, 0, 2*time.Second, 8)
	e := newEnv(t)
	a := e.settleApp()
	a.failOnCancel = true
	a.dumpFn = func(n int) bridge.UIDump { return dumpOf("com.android.settings", "tick "+strconv.Itoa(n)) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.onOp = func(op string, n int) {
		if op == bridge.UIOpDump && n == 2 {
			cancel()
		}
	}
	s, exit := e.helperScreen(t, ctx, bridge.UIOpClick, xy(1, 1))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	c := decodeClicked(t, s)
	if !c.Clicked || c.Screen != nil || !strings.HasPrefix(c.Note, notRepeat) {
		t.Fatalf("result %s", s.data[0])
	}
	if j := e.last(t); j.Status != journal.StatusDenied || !strings.Contains(j.Reason, "abandoned") || j.Rule != "helper.screen" {
		t.Fatalf("journal %+v", j)
	}
}

func TestSettleCapStopsNewReads(t *testing.T) {
	// The gap alone goes past the cap: after the first read no other one starts.
	settleTimes(t, time.Millisecond, 200*time.Millisecond, 0, 100*time.Millisecond, 1000)
	e := newEnv(t)
	a := e.settleApp()
	a.dumpFn = func(n int) bridge.UIDump { return dumpOf("com.android.settings", "tick "+strconv.Itoa(n)) }
	s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	c := decodeClicked(t, s)
	if c.Screen == nil || c.Screen.Settled || c.Screen.Elements[0].Text != "tick 1" || c.Note != "" {
		t.Fatalf("result %s", s.data[0])
	}
	if n := a.count(bridge.UIOpDump); n != 1 {
		t.Fatalf("%d reads: one started past the cap", n)
	}
	if j := e.last(t); j.Status != journal.StatusOK || !strings.Contains(j.Reason, "settled=false") {
		t.Fatalf("journal %+v", j)
	}
}

func TestSettleGuardBlocksMidwayInATap(t *testing.T) {
	settleTimes(t, time.Millisecond, time.Millisecond, 0, 2*time.Second, 8)
	e := newEnv(t)
	a := e.settleApp("com.bank.app")
	a.dumpFn = func(n int) bridge.UIDump { return dumpOf("com.android.settings", "first read "+strconv.Itoa(n)) }
	a.fg = func(bool) bridge.UIForeground {
		// 1: the tap's guard, 2: the first read's, 3: the second read's.
		if a.count(bridge.UIOpForeground) >= 3 {
			return bridge.UIForeground{Package: "com.bank.app"}
		}
		return bridge.UIForeground{Package: "com.android.settings"}
	}
	s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
	if exit.Code != 0 {
		t.Fatalf("the act happened: exit %+v", exit)
	}
	c := decodeClicked(t, s)
	if !c.Clicked || c.Screen != nil || !strings.HasPrefix(c.Note, notRepeat) || !strings.Contains(c.Note, "com.bank.app is a protected app") {
		t.Fatalf("result %s", s.data[0])
	}
	if strings.Contains(string(s.data[0]), "first read") {
		t.Fatalf("the first read came out: %s", s.data[0])
	}
	if n := a.count(bridge.UIOpDump); n != 1 {
		t.Fatalf("%d dumps", n)
	}
	if j := e.last(t); j.Status != journal.StatusBlocked || j.Rule != "helper.screen" || !strings.Contains(j.Reason, "com.bank.app is a protected app") {
		t.Fatalf("journal %+v", j)
	}
}

func TestTypeWithScreen(t *testing.T) {
	settleTimes(t, time.Millisecond, time.Millisecond, 0, 2*time.Second, 8)
	check := func(t *testing.T, e *env, s *stream, exit ipc.Exit) {
		t.Helper()
		if exit.Code != 0 || len(s.data) != 1 {
			t.Fatalf("exit %+v data %q", exit, s.data)
		}
		var out TextSetScreen
		if err := json.Unmarshal(s.data[0], &out); err != nil {
			t.Fatal(err)
		}
		if !out.Set || out.Screen == nil || !out.Screen.Settled || out.Note != "" || !strings.Contains(string(s.data[0]), `"set":true`) {
			t.Fatalf("result %s", s.data[0])
		}
		all := e.entriesAll(t)
		ty, rd := all[len(all)-2], all[len(all)-1]
		if ty.Rule != "helper.type" || ty.Status != journal.StatusOK || rd.Rule != "helper.screen" || rd.Status != journal.StatusOK ||
			!strings.Contains(rd.Reason, "after "+bridge.UIOpSetText) || !strings.Contains(rd.Reason, "settled=true") {
			t.Fatalf("journal %+v / %+v", ty, rd)
		}
	}
	t.Run("with the app", func(t *testing.T) {
		e := newEnv(t)
		e.settleApp()
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpSetText, bridge.UISetTextArgs{Text: "hi"})
		check(t, e, s, exit)
	})
	t.Run("without the app", func(t *testing.T) {
		xml, err := os.ReadFile("../uidump/testdata/settings.xml")
		if err != nil {
			t.Fatal(err)
		}
		e := newEnv(t)
		e.adb.shell["dumpsys accessibility"] = "User state[\n  Enabled services:{}\n"
		e.adb.out["exec-out uiautomator dump /dev/tty"] = string(xml)
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpSetText, bridge.UISetTextArgs{Text: "hi"})
		check(t, e, s, exit)
		found := false
		for _, r := range e.adb.ran() {
			found = found || r == "shell input text hi"
		}
		if !found {
			t.Fatalf("adb ran %q", e.adb.ran())
		}
	})
}

func TestSettleWithoutTheApp(t *testing.T) {
	settleTimes(t, time.Millisecond, time.Millisecond, 0, 2*time.Second, 8)
	xml, err := os.ReadFile("../uidump/testdata/settings.xml")
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t) // no app
	e.adb.shell["dumpsys accessibility"] = "User state[\n  Enabled services:{}\n"
	e.adb.out["exec-out uiautomator dump /dev/tty"] = string(xml)
	s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(10, 20))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	c := decodeClicked(t, s)
	if c.Screen == nil || !c.Screen.Settled || c.Screen.Package != "com.android.settings" || len(c.Screen.Elements) == 0 {
		t.Fatalf("result %s", s.data[0])
	}
	if j := e.last(t); !strings.Contains(j.Reason, "via adb") {
		t.Fatalf("journal %+v", j)
	}

	// The accessibility service on and no app: it fails as screen does.
	e2 := newEnv(t)
	e2.adb.shell["dumpsys accessibility"] = "Enabled services:{{tech.hybriq.adbuddy/tech.hybriq.adbuddy.a11y.AgentAccessibilityService}}"
	s, exit = e2.helperScreen(t, context.Background(), bridge.UIOpClick, xy(10, 20))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	c = decodeClicked(t, s)
	if c.Screen != nil || !strings.HasPrefix(c.Note, notRepeat) || !strings.Contains(c.Note, "open the ADBuddy app") {
		t.Fatalf("result %s", s.data[0])
	}
	if j := e2.last(t); j.Status != journal.StatusError || j.Rule != "helper.screen" {
		t.Fatalf("journal %+v", j)
	}
	_, exit = e2.helperScreen(t, context.Background(), bridge.UIOpDump, bridge.UIDumpArgs{})
	if exit.Code != 75 || exit.ErrorCode != "UI_UNAVAILABLE" {
		t.Fatalf("screen --settle: %+v", exit)
	}
}

func TestNoScreenWithoutTheFlag(t *testing.T) {
	e := newEnv(t)
	a := e.settleApp()
	a.dumpFn = func(int) bridge.UIDump { return dumpOf("com.android.settings", "x") }
	s, exit := e.helper(t, bridge.UIOpClick, xy(1, 1))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	want, _ := json.Marshal(bridge.UIClicked{Clicked: true, Bounds: [4]int{0, 0, 10, 10}, How: "action"})
	if string(s.data[0]) != string(want) || strings.Contains(string(s.data[0]), "screen") {
		t.Fatalf("data %s, want %s", s.data[0], want)
	}
	s, exit = e.helper(t, bridge.UIOpDump, bridge.UIDumpArgs{})
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	if n := a.count(bridge.UIOpDump); strings.Contains(string(s.data[0]), "settled") || n != 1 {
		t.Fatalf("data %s, %d dumps", s.data[0], n)
	}
	s, exit = e.helper(t, bridge.UIOpSetText, bridge.UISetTextArgs{Text: "hi"})
	if exit.Code != 0 || string(s.data[0]) != `{"set":true}` {
		t.Fatalf("type: %+v %s", exit, s.data)
	}
}

func TestScreenSettleJournalSaysSettled(t *testing.T) {
	settleTimes(t, time.Millisecond, time.Millisecond, 0, 2*time.Second, 8)
	e := newEnv(t)
	e.settleApp()
	if _, exit := e.helperScreen(t, context.Background(), bridge.UIOpDump, bridge.UIDumpArgs{}); exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	if j := e.last(t); j.Status != journal.StatusOK || !strings.HasSuffix(j.Reason, ", settled=true") {
		t.Fatalf("journal %+v", j)
	}
}
