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

// settleCfg are the waits of settle and of the read of the start.
type settleCfg struct {
	first, gap, quiet, idle, blockWait, max time.Duration
	reads                                   int
	baseline, slack                         time.Duration
}

// fastSettle is a test's waits: a screen that changed settles one read after
// its last change, one that never changes after 30 ms, a block that lasts
// gives up after 20 ms, and the guard is not asked again after the start.
func fastSettle() settleCfg {
	return settleCfg{first: time.Millisecond, gap: time.Millisecond, quiet: time.Nanosecond, idle: 30 * time.Millisecond,
		blockWait: 20 * time.Millisecond, max: 10 * time.Second, reads: 100000, baseline: time.Second, slack: 10 * time.Second}
}

// settleTimes sets the waits of settle for a test and restores them after.
// It runs before the test starts any goroutine.
func settleTimes(t *testing.T, c settleCfg) {
	t.Helper()
	old := settleCfg{settleFirst, settleGap, settleQuiet, settleIdle, settleBlockWait, settleMax, settleReads, baselineWait, baselineSlack}
	set := func(c settleCfg) {
		settleFirst, settleGap, settleQuiet, settleIdle, settleBlockWait, settleMax, settleReads, baselineWait, baselineSlack =
			c.first, c.gap, c.quiet, c.idle, c.blockWait, c.max, c.reads, c.baseline, c.slack
	}
	set(c)
	t.Cleanup(func() { set(old) })
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
	mu     sync.Mutex
	ops    []string
	counts map[string]int
	dumpFn func(n int) bridge.UIDump
	// dumpErr, when set, is the dump's answer (nil: the dump).
	dumpErr func(n int, afterClick bool) error
	// hang makes that dump wait for the end of its context and fail with its error.
	hang func(n int) bool
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
	fn, onOp, fg, derr, hang := a.dumpFn, a.onOp, a.fg, a.dumpErr, a.hang
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
		if hang != nil && hang(n) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		if derr != nil {
			if err := derr(n, clicked); err != nil {
				return nil, err
			}
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

// countAfter counts op in afterClick.
func (a *settleApp) countAfter(op string) int {
	n := 0
	for _, o := range a.afterClick() {
		if o == op {
			n++
		}
	}
	return n
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

func decodeSettled(t *testing.T, s *stream) ScreenSettled {
	t.Helper()
	if len(s.data) != 1 {
		t.Fatalf("data %q", s.data)
	}
	var d ScreenSettled
	if err := json.Unmarshal(s.data[0], &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// is reports whether c is known and equal to want.
func is(c *bool, want bool) bool { return c != nil && *c == want }

// textOf is the first element's text of a screen ("" with none).
func textOf(s *ScreenSettled) string {
	if s == nil || len(s.Elements) == 0 {
		return ""
	}
	return s.Elements[0].Text
}

const notRepeat = "The action was done; do not repeat it."

// protectedAfter answers foreground: Settings before the tap and, after it,
// com.bank.app (protected) while blocked says so.
func protectedAfter(blocked func() bool) func(bool) bridge.UIForeground {
	return func(after bool) bridge.UIForeground {
		if after && blocked() {
			return bridge.UIForeground{Package: "com.bank.app", Activity: "Main"}
		}
		return bridge.UIForeground{Package: "com.android.settings"}
	}
}

func TestSettleStopsWhenTwoReadsMatch(t *testing.T) {
	// A gap above the coarsest clock of Windows: each read is later than the one before.
	c := fastSettle()
	c.gap, c.idle = 20*time.Millisecond, 10*time.Second
	settleTimes(t, c)
	e := newEnv(t)
	a := e.settleApp()
	a.dumpFn = func(n int) bridge.UIDump {
		if n <= 2 { // the start, and the first read: the tap has not shown yet
			return dumpOf("com.android.settings", "A")
		}
		return dumpOf("com.android.settings", "B")
	}
	s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	cs := decodeClicked(t, s)
	if !cs.Clicked || cs.Screen == nil || !cs.Screen.Settled || !is(cs.Screen.Changed, true) || len(cs.Screen.Elements) != 1 || textOf(cs.Screen) != "B" || cs.Note != "" {
		t.Fatalf("result %s", s.data[0])
	}
	// A (as the start: not a change), B (the change), B (still): settled.
	want := "foreground,dump,foreground,dump,foreground,dump"
	if got := strings.Join(a.afterClick(), ","); got != want {
		t.Fatalf("after the tap: %s, want %s", got, want)
	}
	if got := strings.Join(a.sequence()[:3], ","); got != "foreground,dump,click" {
		t.Fatalf("before the tap: %q", a.sequence())
	}
	all := e.entriesAll(t)
	if len(all) < 2 {
		t.Fatalf("journal %+v", all)
	}
	tap, rd := all[len(all)-2], all[len(all)-1]
	if tap.Rule != "helper.tap" || tap.Status != journal.StatusOK || rd.Rule != "helper.screen" || rd.Status != journal.StatusOK ||
		rd.Level != "green" || rd.Source != journal.SourceAgent || strings.Join(rd.Argv, " ") != "adbuddy screen" ||
		!strings.Contains(rd.Reason, "after click") || !strings.Contains(rd.Reason, "settled=true") || !strings.Contains(rd.Reason, "changed=true") {
		t.Fatalf("journal %+v / %+v", tap, rd)
	}
	// The start is not journaled: the tap's entries and the read's, nothing else.
	for _, j := range all {
		if j.Rule == "helper.screen" && j.ID != rd.ID {
			t.Fatalf("the start was journaled: %+v", j)
		}
	}
}

func TestSettleNeedsAChange(t *testing.T) {
	t.Run("it never changes", func(t *testing.T) {
		c := fastSettle()
		c.idle = 150 * time.Millisecond
		settleTimes(t, c)
		e := newEnv(t)
		a := e.settleApp()
		a.dumpFn = func(int) bridge.UIDump { return dumpOf("com.android.settings", "A") }
		var clickedAt time.Time
		var mu sync.Mutex
		a.onOp = func(op string, n int) {
			if op == bridge.UIOpClick {
				mu.Lock()
				clickedAt = time.Now()
				mu.Unlock()
			}
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		mu.Lock()
		took := time.Since(clickedAt)
		mu.Unlock()
		cs := decodeClicked(t, s)
		if cs.Screen == nil || !cs.Screen.Settled || !is(cs.Screen.Changed, false) || textOf(cs.Screen) != "A" || cs.Note != "" {
			t.Fatalf("result %s", s.data[0])
		}
		if took < c.idle {
			t.Fatalf("settled %v after the tap, before the idle wait %v", took, c.idle)
		}
		if n := a.countAfter(bridge.UIOpDump); n < 2 {
			t.Fatalf("%d reads", n)
		}
		if j := e.last(t); j.Status != journal.StatusOK || !strings.Contains(j.Reason, "settled=true") || !strings.Contains(j.Reason, "changed=false") {
			t.Fatalf("journal %+v", j)
		}
	})
	t.Run("the old screen twice, then the new one", func(t *testing.T) {
		c := fastSettle()
		c.idle = 10 * time.Second
		settleTimes(t, c)
		e := newEnv(t)
		a := e.settleApp()
		a.dumpFn = func(n int) bridge.UIDump {
			if n <= 3 { // the start and two reads of the old screen
				return dumpOf("com.android.settings", "A")
			}
			return dumpOf("com.android.settings", "B")
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		cs := decodeClicked(t, s)
		if cs.Screen == nil || !cs.Screen.Settled || !is(cs.Screen.Changed, true) || textOf(cs.Screen) != "B" {
			t.Fatalf("the old screen came out as settled: %s", s.data[0])
		}
		if n := a.countAfter(bridge.UIOpDump); n < 4 {
			t.Fatalf("%d reads", n)
		}
	})
}

func TestSettleQuietAfterTheChange(t *testing.T) {
	c := fastSettle()
	c.gap, c.quiet, c.idle = 5*time.Millisecond, 80*time.Millisecond, 10*time.Second
	settleTimes(t, c)
	e := newEnv(t)
	a := e.settleApp()
	var mu sync.Mutex
	var firstC time.Time
	a.dumpFn = func(n int) bridge.UIDump {
		switch {
		case n == 1:
			return dumpOf("com.android.settings", "A")
		case n == 2:
			return dumpOf("com.android.settings", "B")
		}
		mu.Lock()
		if firstC.IsZero() {
			firstC = time.Now()
		}
		mu.Unlock()
		return dumpOf("com.android.settings", "C")
	}
	s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	mu.Lock()
	quietFor := time.Since(firstC)
	mu.Unlock()
	cs := decodeClicked(t, s)
	if cs.Screen == nil || !cs.Screen.Settled || !is(cs.Screen.Changed, true) || textOf(cs.Screen) != "C" {
		t.Fatalf("result %s", s.data[0])
	}
	if quietFor < c.quiet {
		t.Fatalf("settled %v after C first showed, before the quiet %v", quietFor, c.quiet)
	}
}

func TestSettleIgnoresFocusOnly(t *testing.T) {
	focused := func(text string) bridge.UIDump {
		d := dumpOf("com.android.settings", text)
		d.Elements[0].Focused = true
		return d
	}
	t.Run("sameScreen", func(t *testing.T) {
		a := dumpOf("com.android.settings", "A")
		if !sameScreen(a, focused("A")) {
			t.Error("the focus alone counts as a change")
		}
		empty, none := dumpOf("p"), dumpOf("p")
		none.Elements = nil
		if !sameScreen(empty, none) || !sameScreen(none, empty) {
			t.Error("no elements and nil elements differ")
		}
		on, off := true, false
		x, y := dumpOf("p", "Wi-Fi"), dumpOf("p", "Wi-Fi")
		x.Elements[0].Checked, y.Elements[0].Checked = &on, &off
		if sameScreen(x, y) {
			t.Error("a switch that changed is not a change")
		}
		for name, change := range map[string]func(*bridge.UIDump){
			"package":  func(d *bridge.UIDump) { d.Package = "q" },
			"activity": func(d *bridge.UIDump) { d.Activity = "Other" },
			"locked":   func(d *bridge.UIDump) { d.Locked = true },
			"text":     func(d *bridge.UIDump) { d.Elements[0].Text = "Wi-Fi 2" },
			"bounds":   func(d *bridge.UIDump) { d.Elements[0].Bounds[1] = 50 },
			"enabled":  func(d *bridge.UIDump) { d.Elements[0].Enabled = false },
			"one more": func(d *bridge.UIDump) { d.Elements = append(d.Elements, d.Elements[0]) },
		} {
			d := dumpOf("p", "Wi-Fi")
			change(&d)
			if sameScreen(dumpOf("p", "Wi-Fi"), d) {
				t.Errorf("%s: not a change", name)
			}
		}
		// sameScreen does not touch what it compares.
		f := focused("A")
		_ = sameScreen(a, f)
		if !f.Elements[0].Focused {
			t.Error("sameScreen changed its argument")
		}
	})
	t.Run("a tap that only moves the focus", func(t *testing.T) {
		c := fastSettle()
		c.idle = 10 * time.Second
		settleTimes(t, c)
		e := newEnv(t)
		a := e.settleApp()
		a.dumpFn = func(n int) bridge.UIDump {
			switch {
			case n == 1:
				return dumpOf("com.android.settings", "A")
			case n <= 5:
				return focused("A")
			}
			return dumpOf("com.android.settings", "B")
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		cs := decodeClicked(t, s)
		if cs.Screen == nil || !cs.Screen.Settled || !is(cs.Screen.Changed, true) || textOf(cs.Screen) != "B" {
			t.Fatalf("the focus counted as the change: %s", s.data[0])
		}
	})
}

func TestSettleWithoutBase(t *testing.T) {
	c := fastSettle()
	c.idle, c.baseline = 150*time.Millisecond, 20*time.Millisecond
	settleTimes(t, c)
	e := newEnv(t)
	a := e.settleApp()
	a.hang = func(n int) bool { return n == 1 } // the app does not answer the start in time
	a.dumpFn = func(int) bridge.UIDump { return dumpOf("com.android.settings", "A") }
	var mu sync.Mutex
	var clickedAt time.Time
	a.onOp = func(op string, n int) {
		if op == bridge.UIOpClick {
			mu.Lock()
			clickedAt = time.Now()
			mu.Unlock()
		}
	}
	s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	mu.Lock()
	took := time.Since(clickedAt)
	mu.Unlock()
	if n := a.count(bridge.UIOpClick); n != 1 {
		t.Fatalf("%d clicks", n)
	}
	cs := decodeClicked(t, s)
	if cs.Screen == nil || !cs.Screen.Settled || cs.Screen.Changed != nil || textOf(cs.Screen) != "A" || cs.Note != "" {
		t.Fatalf("result %s", s.data[0])
	}
	if strings.Contains(string(s.data[0]), `"changed"`) {
		t.Fatalf("an unknown change came out: %s", s.data[0])
	}
	if took < c.idle {
		t.Fatalf("equal reads closed the wait after %v, before the idle wait %v", took, c.idle)
	}
	if n := a.countAfter(bridge.UIOpDump); n < 2 {
		t.Fatalf("%d reads", n)
	}
	if j := e.last(t); j.Status != journal.StatusOK || !strings.Contains(j.Reason, "settled=true") || !strings.Contains(j.Reason, "changed=unknown") {
		t.Fatalf("journal %+v", j)
	}
	// Nothing went to adb for the start: no uiautomator when the app does not answer.
	for _, r := range e.adb.ran() {
		if strings.Contains(r, "uiautomator") || strings.Contains(r, "dumpsys accessibility") {
			t.Fatalf("adb ran %q", e.adb.ran())
		}
	}
}

func TestBaselineDeniedBlocksTheAction(t *testing.T) {
	settleTimes(t, fastSettle())
	for _, op := range []struct {
		name string
		op   string
		args any
		rule string
	}{{"tap", bridge.UIOpClick, xy(1, 1), "helper.tap"}, {"type", bridge.UIOpSetText, bridge.UISetTextArgs{Text: "hi"}, "helper.type"}} {
		t.Run(op.name, func(t *testing.T) {
			e := newEnv(t)
			a := e.settleApp()
			a.dumpErr = func(int, bool) error {
				return &UIError{Code: bridge.CodeProtected, Text: "ADBuddy does not read this screen."}
			}
			s, exit := e.helperScreen(t, context.Background(), op.op, op.args)
			if exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, "ADBuddy does not read this screen") || len(s.data) != 0 {
				t.Fatalf("exit %+v data %q", exit, s.data)
			}
			if a.count(bridge.UIOpClick) != 0 || a.count(bridge.UIOpSetText) != 0 {
				t.Fatalf("the act happened: %q", a.sequence())
			}
			all := e.entriesAll(t)
			if len(all) != 1 || all[0].Rule != op.rule || all[0].Status != journal.StatusBlocked {
				t.Fatalf("journal %+v", all)
			}
		})
	}
}

func TestBaselineKeepsTheDoors(t *testing.T) {
	t.Run("paused during the start", func(t *testing.T) {
		settleTimes(t, fastSettle())
		e := newEnv(t)
		a := e.settleApp()
		var mu sync.Mutex
		paused := false
		e.d.o.Paused = func() bool { mu.Lock(); defer mu.Unlock(); return paused }
		a.onOp = func(op string, n int) {
			if op == bridge.UIOpDump && n == 1 {
				mu.Lock()
				paused = true
				mu.Unlock()
			}
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, PausedMessage) || len(s.data) != 0 {
			t.Fatalf("exit %+v data %q", exit, s.data)
		}
		if a.count(bridge.UIOpClick) != 0 {
			t.Fatalf("the act happened: %q", a.sequence())
		}
		if j := e.last(t); j.Rule != "helper.tap" || j.Status != journal.StatusBlocked || j.Reason != PausedMessage {
			t.Fatalf("journal %+v", j)
		}
	})
	t.Run("the agent gone during the start", func(t *testing.T) {
		settleTimes(t, fastSettle())
		e := newEnv(t)
		a := e.settleApp()
		a.failOnCancel = true
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		a.onOp = func(op string, n int) {
			if op == bridge.UIOpDump && n == 1 {
				cancel()
			}
		}
		s, exit := e.helperScreen(t, ctx, bridge.UIOpClick, xy(1, 1))
		if exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" || exit.Error != "stopped" || len(s.data) != 0 {
			t.Fatalf("exit %+v data %q", exit, s.data)
		}
		if a.count(bridge.UIOpClick) != 0 {
			t.Fatalf("the act happened: %q", a.sequence())
		}
		all := e.entriesAll(t)
		if len(all) != 1 || all[0].Rule != "helper.tap" || all[0].Status != journal.StatusDenied || !strings.Contains(all[0].Reason, "abandoned by the agent") {
			t.Fatalf("journal %+v", all)
		}
	})
	t.Run("a slow start asks the guard again", func(t *testing.T) {
		c := fastSettle()
		c.slack = time.Millisecond
		settleTimes(t, c)
		e := newEnv(t)
		a := e.settleApp("com.bank.app")
		a.onOp = func(op string, n int) {
			if op == bridge.UIOpDump && n == 1 {
				time.Sleep(30 * time.Millisecond) // the start takes longer than the slack
			}
		}
		a.fg = func(bool) bridge.UIForeground {
			if a.count(bridge.UIOpForeground) >= 2 { // the second time the guard looks: a protected app opened
				return bridge.UIForeground{Package: "com.bank.app"}
			}
			return bridge.UIForeground{Package: "com.android.settings"}
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, "com.bank.app is a protected app") || len(s.data) != 0 {
			t.Fatalf("exit %+v data %q", exit, s.data)
		}
		if a.count(bridge.UIOpClick) != 0 {
			t.Fatalf("the act happened: %q", a.sequence())
		}
		if got := strings.Join(a.sequence(), ","); got != "foreground,dump,foreground" {
			t.Fatalf("sequence %s", got)
		}
		if j := e.last(t); j.Rule != "helper.tap" || j.Status != journal.StatusBlocked || !strings.Contains(j.Reason, "com.bank.app is a protected app") {
			t.Fatalf("journal %+v", j)
		}
	})
	t.Run("a slow start, the guard still says yes", func(t *testing.T) {
		c := fastSettle()
		c.slack = time.Millisecond
		settleTimes(t, c)
		e := newEnv(t)
		a := e.settleApp()
		a.onOp = func(op string, n int) {
			if op == bridge.UIOpDump && n == 1 {
				time.Sleep(30 * time.Millisecond)
			}
		}
		if _, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1)); exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		if got := strings.Join(a.sequence()[:4], ","); got != "foreground,dump,foreground,click" || a.count(bridge.UIOpClick) != 1 {
			t.Fatalf("sequence %q", a.sequence())
		}
	})
	t.Run("a fast start does not ask the guard again", func(t *testing.T) {
		settleTimes(t, fastSettle())
		e := newEnv(t)
		a := e.settleApp()
		if _, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1)); exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		if got := strings.Join(a.sequence()[:3], ","); got != "foreground,dump,click" {
			t.Fatalf("sequence %q", a.sequence())
		}
	})
	t.Run("paused while the guard looked again", func(t *testing.T) {
		c := fastSettle()
		c.slack = time.Millisecond
		settleTimes(t, c)
		e := newEnv(t)
		a := e.settleApp()
		var mu sync.Mutex
		paused := false
		e.d.o.Paused = func() bool { mu.Lock(); defer mu.Unlock(); return paused }
		a.onOp = func(op string, n int) {
			switch {
			case op == bridge.UIOpDump && n == 1:
				time.Sleep(30 * time.Millisecond)
			case op == bridge.UIOpForeground && n == 2:
				mu.Lock()
				paused = true
				mu.Unlock()
			}
		}
		_, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != ExitDenied || !strings.Contains(exit.Error, PausedMessage) || a.count(bridge.UIOpClick) != 0 {
			t.Fatalf("exit %+v, sequence %q", exit, a.sequence())
		}
	})
}

func TestSettleRetriesATransientBlock(t *testing.T) {
	t.Run("the guard, after a tap", func(t *testing.T) {
		c := fastSettle()
		c.blockWait = 10 * time.Second
		settleTimes(t, c)
		e := newEnv(t)
		a := e.settleApp("com.bank.app")
		a.dumpFn = func(n int) bridge.UIDump {
			if n == 1 {
				return dumpOf("com.android.settings", "A")
			}
			return dumpOf("com.android.settings", "B")
		}
		a.fg = protectedAfter(func() bool { return a.countAfter(bridge.UIOpForeground) <= 2 })
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		cs := decodeClicked(t, s)
		if cs.Screen == nil || !cs.Screen.Settled || !is(cs.Screen.Changed, true) || textOf(cs.Screen) != "B" || cs.Note != "" {
			t.Fatalf("result %s", s.data[0])
		}
		if got := strings.Join(a.afterClick()[:4], ","); got != "foreground,foreground,foreground,dump" {
			t.Fatalf("a dump during the block: %q", a.afterClick())
		}
		if j := e.last(t); j.Status != journal.StatusOK || j.Rule != "helper.screen" {
			t.Fatalf("journal %+v", j)
		}
	})
	t.Run("the dump's own check, after a tap", func(t *testing.T) {
		c := fastSettle()
		c.blockWait = 10 * time.Second
		settleTimes(t, c)
		e := newEnv(t)
		a := e.settleApp()
		a.dumpFn = func(n int) bridge.UIDump {
			if n == 1 {
				return dumpOf("com.android.settings", "A")
			}
			return dumpOf("com.android.settings", "B "+strconv.Itoa(min(n, 4)))
		}
		a.dumpErr = func(n int, after bool) error {
			if after && n <= 3 { // the first two reads after the tap
				return &UIError{Code: bridge.CodeProtected, Text: "ADBuddy does not read this screen."}
			}
			return nil
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		cs := decodeClicked(t, s)
		if cs.Screen == nil || !cs.Screen.Settled || !is(cs.Screen.Changed, true) || textOf(cs.Screen) != "B 4" || cs.Note != "" {
			t.Fatalf("result %s", s.data[0])
		}
		if n := a.count(bridge.UIOpClick); n != 1 {
			t.Fatalf("%d clicks", n)
		}
	})
	t.Run("screen --settle, the helper's own guard", func(t *testing.T) {
		c := fastSettle()
		c.blockWait = 10 * time.Second
		settleTimes(t, c)
		e := newEnv(t)
		a := e.settleApp("com.bank.app")
		a.fg = func(bool) bridge.UIForeground {
			if a.count(bridge.UIOpForeground) <= 2 {
				return bridge.UIForeground{Package: "com.bank.app"}
			}
			return bridge.UIForeground{Package: "com.android.settings"}
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpDump, bridge.UIDumpArgs{})
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		d := decodeSettled(t, s)
		if !d.Settled || !is(d.Changed, false) || textOf(&d) != "same" {
			t.Fatalf("result %s", s.data[0])
		}
		if got := strings.Join(a.sequence()[:4], ","); got != "foreground,foreground,foreground,dump" {
			t.Fatalf("a dump during the block: %q", a.sequence())
		}
		all := e.entriesAll(t)
		if len(all) != 1 || all[0].Status != journal.StatusOK || !strings.HasSuffix(all[0].Reason, ", settled=true") {
			t.Fatalf("journal %+v", all)
		}
	})
	t.Run("screen --settle, the helper's guard then a change", func(t *testing.T) {
		c := fastSettle()
		c.blockWait, c.idle = 10*time.Second, 10*time.Second
		settleTimes(t, c)
		e := newEnv(t)
		a := e.settleApp("com.bank.app")
		a.fg = func(bool) bridge.UIForeground {
			if a.count(bridge.UIOpForeground) == 1 {
				return bridge.UIForeground{Package: "com.bank.app"}
			}
			return bridge.UIForeground{Package: "com.android.settings"}
		}
		a.dumpFn = func(n int) bridge.UIDump {
			if n == 1 { // the first valid read is the start: it never comes out
				return dumpOf("com.android.settings", "old")
			}
			return dumpOf("com.android.settings", "new")
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpDump, bridge.UIDumpArgs{})
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		d := decodeSettled(t, s)
		if !d.Settled || !is(d.Changed, true) || textOf(&d) != "new" {
			t.Fatalf("result %s", s.data[0])
		}
	})
}

func TestSettleBlockThatLastsGivesNoScreen(t *testing.T) {
	always := func() bool { return true }
	t.Run("tap", func(t *testing.T) {
		c := fastSettle()
		c.blockWait = 60 * time.Millisecond
		settleTimes(t, c)
		e := newEnv(t)
		a := e.settleApp("com.bank.app")
		a.fg = protectedAfter(always)
		start := time.Now()
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		if took := time.Since(start); took < c.blockWait {
			t.Fatalf("gave up after %v, before %v", took, c.blockWait)
		}
		cs := decodeClicked(t, s)
		if !cs.Clicked || cs.Screen != nil || !strings.HasPrefix(cs.Note, notRepeat) || !strings.Contains(cs.Note, "com.bank.app is a protected app") {
			t.Fatalf("result %s", s.data[0])
		}
		if n := a.countAfter(bridge.UIOpDump); n != 0 {
			t.Fatalf("a dump after the block: %q", a.afterClick())
		}
		if n := a.countAfter(bridge.UIOpForeground); n < 2 {
			t.Fatalf("the block was not tried again: %q", a.afterClick())
		}
	})
	t.Run("screen --settle, blocked from the start", func(t *testing.T) {
		settleTimes(t, fastSettle())
		e := newEnv(t)
		a := e.settleApp("com.bank.app")
		a.fg = func(bool) bridge.UIForeground { return bridge.UIForeground{Package: "com.bank.app"} }
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpDump, bridge.UIDumpArgs{})
		if exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" || !strings.HasPrefix(exit.Error, "Blocked by ADBuddy: com.bank.app is a protected app") || len(s.data) != 0 {
			t.Fatalf("exit %+v data %q", exit, s.data)
		}
		if n := a.count(bridge.UIOpDump); n != 0 || a.count(bridge.UIOpForeground) < 2 {
			t.Fatalf("sequence %q", a.sequence())
		}
		all := e.entriesAll(t)
		if len(all) != 1 || all[0].Status != journal.StatusBlocked || !strings.Contains(all[0].Reason, "com.bank.app is a protected app") {
			t.Fatalf("journal %+v", all)
		}
	})
	// With a read made before the block, the block is what comes out: at the
	// block wait, at the time cap and at the cap of attempts.
	for name, change := range map[string]func(*settleCfg){
		"block wait": func(c *settleCfg) {},
		"time cap":   func(c *settleCfg) { c.blockWait, c.max = 10*time.Second, 60*time.Millisecond },
		"tries cap":  func(c *settleCfg) { c.blockWait, c.reads = 10*time.Second, 3 },
	} {
		t.Run("screen --settle, a read and then the block, "+name, func(t *testing.T) {
			c := fastSettle()
			c.idle = 10 * time.Second
			change(&c)
			settleTimes(t, c)
			e := newEnv(t)
			a := e.settleApp("com.bank.app")
			a.dumpFn = func(n int) bridge.UIDump { return dumpOf("com.android.settings", "read "+strconv.Itoa(n)) }
			a.fg = func(bool) bridge.UIForeground {
				// The helper's own guard says yes; every later one, no.
				if a.count(bridge.UIOpForeground) >= 2 {
					return bridge.UIForeground{Package: "com.bank.app"}
				}
				return bridge.UIForeground{Package: "com.android.settings"}
			}
			s, exit := e.helperScreen(t, context.Background(), bridge.UIOpDump, bridge.UIDumpArgs{})
			if exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" || !strings.HasPrefix(exit.Error, "Blocked by ADBuddy: com.bank.app is a protected app") ||
				strings.Count(exit.Error, "Blocked by ADBuddy:") != 1 {
				t.Fatalf("exit %+v", exit)
			}
			if n := a.count(bridge.UIOpDump); len(s.data) != 0 || n != 1 {
				t.Fatalf("data %q, %d dumps: the read before the block must not come out", s.data, n)
			}
			if name == "tries cap" && a.count(bridge.UIOpForeground) != 1+c.reads {
				t.Fatalf("%d guards, want %d", a.count(bridge.UIOpForeground), 1+c.reads)
			}
			j := e.last(t)
			if j.Status != journal.StatusBlocked || strings.HasPrefix(j.Reason, "Blocked by ADBuddy:") || !strings.Contains(j.Reason, "com.bank.app is a protected app") {
				t.Fatalf("journal %+v", j)
			}
		})
		t.Run("tap, a read and then the block, "+name, func(t *testing.T) {
			c := fastSettle()
			c.idle = 10 * time.Second
			change(&c)
			settleTimes(t, c)
			e := newEnv(t)
			a := e.settleApp("com.bank.app")
			a.dumpFn = func(n int) bridge.UIDump { return dumpOf("com.android.settings", "read "+strconv.Itoa(n)) }
			a.fg = protectedAfter(func() bool { return a.countAfter(bridge.UIOpForeground) >= 2 })
			s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
			if exit.Code != 0 {
				t.Fatalf("exit %+v", exit)
			}
			cs := decodeClicked(t, s)
			if cs.Screen != nil || !strings.HasPrefix(cs.Note, notRepeat) || !strings.Contains(cs.Note, "com.bank.app is a protected app") || strings.Contains(string(s.data[0]), `"read `) {
				t.Fatalf("result %s", s.data[0])
			}
			if n := a.countAfter(bridge.UIOpDump); n != 1 {
				t.Fatalf("%d dumps after the tap", n)
			}
		})
	}
}

func TestSettleGivesUpAtTheCap(t *testing.T) {
	for name, times := range map[string]struct {
		max   time.Duration
		reads int
	}{"reads": {5 * time.Second, 4}, "time": {30 * time.Millisecond, 100000}} {
		t.Run(name, func(t *testing.T) {
			c := fastSettle()
			c.gap, c.quiet, c.idle, c.max, c.reads = 5*time.Millisecond, 10*time.Second, 10*time.Second, times.max, times.reads
			settleTimes(t, c)
			e := newEnv(t)
			a := e.settleApp()
			a.dumpFn = func(n int) bridge.UIDump { return dumpOf("com.android.settings", "tick "+strconv.Itoa(n)) }
			s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
			if exit.Code != 0 {
				t.Fatalf("exit %+v", exit)
			}
			cs := decodeClicked(t, s)
			if cs.Screen == nil || cs.Screen.Settled || !is(cs.Screen.Changed, true) || cs.Note != "" {
				t.Fatalf("result %s", s.data[0])
			}
			if name == "reads" && a.countAfter(bridge.UIOpDump) != times.reads {
				t.Fatalf("%d reads, want %d", a.countAfter(bridge.UIOpDump), times.reads)
			}
			if j := e.last(t); j.Status != journal.StatusOK || !strings.Contains(j.Reason, "settled=false") || !strings.Contains(j.Reason, "changed=true") {
				t.Fatalf("journal %+v", j)
			}
		})
	}
}

func TestSettleEveryReadIsGuarded(t *testing.T) {
	settleTimes(t, fastSettle())
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
			a.dumpErr = func(_ int, after bool) error {
				if after {
					return &UIError{Code: bridge.CodeProtected, Text: "ADBuddy does not read this screen."}
				}
				return nil
			}
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
			if c.noDumps && a.countAfter(bridge.UIOpDump) != 0 {
				t.Fatalf("a dump was asked after the block: %q", a.afterClick())
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
	c := fastSettle()
	c.quiet, c.idle = 10*time.Second, 10*time.Second
	settleTimes(t, c)
	t.Run("paused", func(t *testing.T) {
		e := newEnv(t)
		a := e.settleApp()
		var mu sync.Mutex
		paused := false
		e.d.o.Paused = func() bool { mu.Lock(); defer mu.Unlock(); return paused }
		a.dumpFn = func(n int) bridge.UIDump { return dumpOf("com.android.settings", "tick "+strconv.Itoa(n)) }
		a.onOp = func(op string, n int) {
			if op == bridge.UIOpDump && n == 3 { // the second read after the tap
				mu.Lock()
				paused = true
				mu.Unlock()
			}
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		cs := decodeClicked(t, s)
		if cs.Screen != nil || !strings.HasPrefix(cs.Note, notRepeat) || !strings.Contains(cs.Note, PausedMessage) {
			t.Fatalf("result %s", s.data[0])
		}
		if n := a.countAfter(bridge.UIOpDump); n != 2 {
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
			if op == bridge.UIOpDump && n == 3 {
				if busy := e.d.ask(&question{title: "Reboot the phone", withdraw: func(error) {}}); busy != "" {
					t.Error(busy)
				}
			}
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		cs := decodeClicked(t, s)
		if cs.Screen != nil || !strings.HasPrefix(cs.Note, notRepeat) || !strings.Contains(cs.Note, `waiting to ask about "Reboot the phone"`) {
			t.Fatalf("result %s", s.data[0])
		}
		if n := a.countAfter(bridge.UIOpDump); n != 2 {
			t.Fatalf("%d reads", n)
		}
		if j := e.last(t); j.Status != journal.StatusBlocked || !strings.Contains(j.Reason, "waiting to ask") {
			t.Fatalf("journal %+v", j)
		}
	})
}

// A pause or a 🔴 question waiting ends the wait at once: they are not a
// transition, so they are not tried again until settleBlockWait.
func TestSettlePausedAndQuestionAreNotRetried(t *testing.T) {
	c := fastSettle()
	c.quiet, c.idle, c.blockWait, c.max = 10*time.Second, 10*time.Second, 10*time.Second, 20*time.Second
	settleTimes(t, c)
	const fast = 3 * time.Second // far below the block wait
	t.Run("paused after a block", func(t *testing.T) {
		e := newEnv(t)
		a := e.settleApp("com.bank.app")
		var mu sync.Mutex
		paused := false
		e.d.o.Paused = func() bool { mu.Lock(); defer mu.Unlock(); return paused }
		a.fg = protectedAfter(func() bool { return true })
		a.onOp = func(op string, n int) {
			if op == bridge.UIOpForeground && a.countAfter(bridge.UIOpForeground) == 1 {
				mu.Lock()
				paused = true
				mu.Unlock()
			}
		}
		start := time.Now()
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if took := time.Since(start); exit.Code != 0 || took > fast {
			t.Fatalf("exit %+v after %v", exit, took)
		}
		cs := decodeClicked(t, s)
		if cs.Screen != nil || !strings.Contains(cs.Note, PausedMessage) || a.countAfter(bridge.UIOpForeground) != 1 {
			t.Fatalf("result %s, after the tap %q", s.data[0], a.afterClick())
		}
	})
	t.Run("question after a read", func(t *testing.T) {
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
		start := time.Now()
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if took := time.Since(start); exit.Code != 0 || took > fast {
			t.Fatalf("exit %+v after %v", exit, took)
		}
		cs := decodeClicked(t, s)
		if cs.Screen != nil || !strings.Contains(cs.Note, "waiting to ask") || a.countAfter(bridge.UIOpDump) != 1 {
			t.Fatalf("result %s, after the tap %q", s.data[0], a.afterClick())
		}
	})
	t.Run("screen --settle, paused", func(t *testing.T) {
		e := newEnv(t)
		a := e.settleApp()
		var mu sync.Mutex
		paused := false
		e.d.o.Paused = func() bool { mu.Lock(); defer mu.Unlock(); return paused }
		a.onOp = func(op string, n int) {
			if op == bridge.UIOpDump && n == 1 {
				mu.Lock()
				paused = true
				mu.Unlock()
			}
		}
		start := time.Now()
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpDump, bridge.UIDumpArgs{})
		if took := time.Since(start); exit.Code != ExitDenied || !strings.Contains(exit.Error, PausedMessage) || len(s.data) != 0 || took > fast {
			t.Fatalf("exit %+v data %q after %v", exit, s.data, took)
		}
	})
}

func TestSettleDumpWaitsAndBlocksMidway(t *testing.T) {
	settleTimes(t, fastSettle())
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
	c := fastSettle()
	c.first, c.gap = 50*time.Millisecond, 50*time.Millisecond
	settleTimes(t, c)
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
	t.Run("screen, blocked first", func(t *testing.T) {
		e := newEnv(t)
		a := e.settleApp("com.bank.app")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		a.fg = func(bool) bridge.UIForeground {
			cancel()
			return bridge.UIForeground{Package: "com.bank.app"}
		}
		s, exit := e.helperScreen(t, ctx, bridge.UIOpDump, bridge.UIDumpArgs{})
		if exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" || exit.Error != "stopped" || len(s.data) != 0 || a.count(bridge.UIOpDump) != 0 {
			t.Fatalf("exit %+v data %q, %q", exit, s.data, a.sequence())
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
		cs := decodeClicked(t, s)
		if !cs.Clicked || cs.Screen != nil || !strings.HasPrefix(cs.Note, notRepeat) {
			t.Fatalf("result %s", s.data[0])
		}
		if n := a.countAfter(bridge.UIOpDump); n != 0 {
			t.Fatalf("%d reads after the cancel", n)
		}
		if j := e.last(t); j.Status != journal.StatusDenied || !strings.Contains(j.Reason, "ADBuddy stopped") {
			t.Fatalf("journal %+v", j)
		}
	})
}

func TestSettleCancelInsideTheSecondRead(t *testing.T) {
	c := fastSettle()
	c.quiet, c.idle = 10*time.Second, 10*time.Second
	settleTimes(t, c)
	e := newEnv(t)
	a := e.settleApp()
	a.failOnCancel = true
	a.dumpFn = func(n int) bridge.UIDump { return dumpOf("com.android.settings", "tick "+strconv.Itoa(n)) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.onOp = func(op string, n int) {
		if op == bridge.UIOpDump && n == 3 { // the second read after the tap
			cancel()
		}
	}
	s, exit := e.helperScreen(t, ctx, bridge.UIOpClick, xy(1, 1))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	cs := decodeClicked(t, s)
	if !cs.Clicked || cs.Screen != nil || !strings.HasPrefix(cs.Note, notRepeat) {
		t.Fatalf("result %s", s.data[0])
	}
	if j := e.last(t); j.Status != journal.StatusDenied || !strings.Contains(j.Reason, "abandoned") || j.Rule != "helper.screen" {
		t.Fatalf("journal %+v", j)
	}
}

func TestSettleCapStopsNewReads(t *testing.T) {
	// The gap alone goes past the cap: after the first read no other one starts.
	c := fastSettle()
	c.gap, c.max = 200*time.Millisecond, 100*time.Millisecond
	settleTimes(t, c)
	e := newEnv(t)
	a := e.settleApp()
	a.dumpFn = func(n int) bridge.UIDump { return dumpOf("com.android.settings", "tick "+strconv.Itoa(n)) }
	s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	cs := decodeClicked(t, s)
	// tick 1 is the start: it never comes out.
	if cs.Screen == nil || cs.Screen.Settled || textOf(cs.Screen) != "tick 2" || !is(cs.Screen.Changed, true) || cs.Note != "" {
		t.Fatalf("result %s", s.data[0])
	}
	if n := a.countAfter(bridge.UIOpDump); n != 1 {
		t.Fatalf("%d reads: one started past the cap", n)
	}
	if j := e.last(t); j.Status != journal.StatusOK || !strings.Contains(j.Reason, "settled=false") {
		t.Fatalf("journal %+v", j)
	}
}

func TestSettleGuardBlocksMidwayInATap(t *testing.T) {
	settleTimes(t, fastSettle())
	e := newEnv(t)
	a := e.settleApp("com.bank.app")
	a.dumpFn = func(n int) bridge.UIDump { return dumpOf("com.android.settings", "first read "+strconv.Itoa(n)) }
	// After the tap: 1, the first read's guard (yes); 2 and later, no.
	a.fg = protectedAfter(func() bool { return a.countAfter(bridge.UIOpForeground) >= 2 })
	s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
	if exit.Code != 0 {
		t.Fatalf("the act happened: exit %+v", exit)
	}
	cs := decodeClicked(t, s)
	if !cs.Clicked || cs.Screen != nil || !strings.HasPrefix(cs.Note, notRepeat) || !strings.Contains(cs.Note, "com.bank.app is a protected app") {
		t.Fatalf("result %s", s.data[0])
	}
	if strings.Contains(string(s.data[0]), "first read") {
		t.Fatalf("the first read came out: %s", s.data[0])
	}
	if n := a.countAfter(bridge.UIOpDump); n != 1 {
		t.Fatalf("%d dumps", n)
	}
	if j := e.last(t); j.Status != journal.StatusBlocked || j.Rule != "helper.screen" || !strings.Contains(j.Reason, "com.bank.app is a protected app") {
		t.Fatalf("journal %+v", j)
	}
}

func TestTypeWithScreen(t *testing.T) {
	settleTimes(t, fastSettle())
	check := func(t *testing.T, e *env, s *stream, exit ipc.Exit, changed string) {
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
			!strings.Contains(rd.Reason, "after "+bridge.UIOpSetText) || !strings.Contains(rd.Reason, "settled=true") || !strings.Contains(rd.Reason, "changed="+changed) {
			t.Fatalf("journal %+v / %+v", ty, rd)
		}
	}
	t.Run("with the app", func(t *testing.T) {
		e := newEnv(t)
		a := e.settleApp()
		a.dumpFn = func(n int) bridge.UIDump {
			if n == 1 {
				return dumpOf("com.android.settings", "")
			}
			return dumpOf("com.android.settings", "hi")
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpSetText, bridge.UISetTextArgs{Text: "hi"})
		check(t, e, s, exit, "true")
		if got := strings.Join(a.sequence()[:3], ","); got != "foreground,dump,setText" {
			t.Fatalf("sequence %q", a.sequence())
		}
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
		check(t, e, s, exit, "unknown")
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
	settleTimes(t, fastSettle())
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
	cs := decodeClicked(t, s)
	if cs.Screen == nil || !cs.Screen.Settled || cs.Screen.Changed != nil || cs.Screen.Package != "com.android.settings" || len(cs.Screen.Elements) == 0 {
		t.Fatalf("result %s", s.data[0])
	}
	if j := e.last(t); !strings.Contains(j.Reason, "via adb") || !strings.Contains(j.Reason, "changed=unknown") {
		t.Fatalf("journal %+v", j)
	}
	// The start is only asked of the app: no uiautomator before the tap (it
	// takes about 2 s, more than the start may wait).
	for _, r := range e.adb.ran() {
		if r == "shell input tap 10 20" {
			break
		}
		if strings.Contains(r, "uiautomator") || strings.Contains(r, "dumpsys accessibility") {
			t.Fatalf("adb read the screen before the tap: %q", e.adb.ran())
		}
	}

	// The accessibility service on and no app: it fails as screen does.
	e2 := newEnv(t)
	e2.adb.shell["dumpsys accessibility"] = "Enabled services:{{tech.hybriq.adbuddy/tech.hybriq.adbuddy.a11y.AgentAccessibilityService}}"
	s, exit = e2.helperScreen(t, context.Background(), bridge.UIOpClick, xy(10, 20))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	cs = decodeClicked(t, s)
	if cs.Screen != nil || !strings.HasPrefix(cs.Note, notRepeat) || !strings.Contains(cs.Note, "open the ADBuddy app") {
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
	if n := a.count(bridge.UIOpDump); n != 0 {
		t.Fatalf("a tap without --screen read the start: %d dumps", n)
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
	if n := a.count(bridge.UIOpDump); n != 1 {
		t.Fatalf("a type without --screen read the start: %d dumps", n)
	}
}

func TestScreenSettleJournalSaysSettled(t *testing.T) {
	settleTimes(t, fastSettle())
	e := newEnv(t)
	e.settleApp()
	s, exit := e.helperScreen(t, context.Background(), bridge.UIOpDump, bridge.UIDumpArgs{})
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	if d := decodeSettled(t, s); !d.Settled || !is(d.Changed, false) {
		t.Fatalf("result %s", s.data[0])
	}
	if j := e.last(t); j.Status != journal.StatusOK || !strings.HasSuffix(j.Reason, ", settled=true") {
		t.Fatalf("journal %+v", j)
	}
}

// AGENTS.md tells the agent what to look for in a note: the literal it quotes
// must be how notDone begins.
func TestAgentsTemplateQuotesNotDone(t *testing.T) {
	b, err := os.ReadFile("../workspace/templates/AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	const quoted = "The action was done; do not repeat it"
	if !strings.Contains(string(b), "`"+quoted+"`") {
		t.Errorf("AGENTS.md does not quote %q", quoted)
	}
	if !strings.HasPrefix(notDone, quoted) {
		t.Errorf("notDone %q does not begin with %q", notDone, quoted)
	}
}

// adbDumpReady makes the fake adb answer a uiautomator read (no ADBuddy
// accessibility service, Settings on screen), for an app that stops answering.
func adbDumpReady(t *testing.T, e *env) {
	t.Helper()
	xml, err := os.ReadFile("../uidump/testdata/settings.xml")
	if err != nil {
		t.Fatal(err)
	}
	e.adb.mu.Lock()
	e.adb.shell["dumpsys accessibility"] = "User state[\n  Enabled services:{}\n"
	e.adb.out["exec-out uiautomator dump /dev/tty"] = string(xml)
	e.adb.mu.Unlock()
}

// A read that came another way than the start (uiautomator instead of the
// app) is not compared with it: a difference of format is not a change. It
// does not end the wait as a change (only settleIdle does) and Changed is
// not known.
func TestSettleMixedRoutes(t *testing.T) {
	unavailable := &UIError{Code: bridge.CodeUnavailable, Text: "the app is not connected"}
	t.Run("tap, the start by the app and every read by adb", func(t *testing.T) {
		c := fastSettle()
		c.gap, c.idle = 5*time.Millisecond, 200*time.Millisecond
		settleTimes(t, c)
		e := newEnv(t)
		adbDumpReady(t, e)
		a := e.settleApp()
		a.dumpFn = func(int) bridge.UIDump { return dumpOf("com.android.settings", "A") }
		var clickedAt time.Time
		var mu sync.Mutex
		a.onOp = func(op string, n int) {
			if op == bridge.UIOpClick {
				mu.Lock()
				clickedAt = time.Now()
				mu.Unlock()
			}
		}
		a.dumpErr = func(_ int, after bool) error {
			if after {
				return unavailable
			}
			return nil
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		mu.Lock()
		took := time.Since(clickedAt)
		mu.Unlock()
		cs := decodeClicked(t, s)
		if cs.Screen == nil || !cs.Screen.Settled || cs.Screen.Changed != nil || cs.Note != "" || len(cs.Screen.Elements) == 0 {
			t.Fatalf("result %s", s.data[0])
		}
		if strings.Contains(string(s.data[0]), `"changed"`) {
			t.Fatalf("changed is not known and came out: %s", s.data[0])
		}
		if took < c.idle {
			t.Fatalf("settled %v after the tap, before the idle wait %v: a difference of format counted as a change", took, c.idle)
		}
		if j := e.last(t); !strings.Contains(j.Reason, "via adb") || !strings.Contains(j.Reason, "changed=unknown") {
			t.Fatalf("journal %+v", j)
		}
	})
	t.Run("tap, a change by the app and then adb", func(t *testing.T) {
		c := fastSettle()
		c.gap, c.idle = 5*time.Millisecond, 10*time.Second
		settleTimes(t, c)
		e := newEnv(t)
		adbDumpReady(t, e)
		a := e.settleApp()
		a.dumpFn = func(n int) bridge.UIDump {
			if n == 1 {
				return dumpOf("com.android.settings", "A")
			}
			return dumpOf("com.android.settings", "B")
		}
		a.dumpErr = func(n int, _ bool) error {
			if n >= 3 { // the start and one read by the app, then adb
				return unavailable
			}
			return nil
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		cs := decodeClicked(t, s)
		if cs.Screen == nil || !cs.Screen.Settled || cs.Screen.Changed != nil || cs.Screen.Package != "com.android.settings" {
			t.Fatalf("result %s", s.data[0])
		}
		if j := e.last(t); !strings.Contains(j.Reason, "via adb") || !strings.Contains(j.Reason, "changed=unknown") {
			t.Fatalf("journal %+v", j)
		}
	})
	t.Run("screen --settle, the first read by the app and the rest by adb", func(t *testing.T) {
		c := fastSettle()
		c.gap, c.idle = 5*time.Millisecond, 200*time.Millisecond
		settleTimes(t, c)
		e := newEnv(t)
		adbDumpReady(t, e)
		a := e.settleApp()
		a.dumpErr = func(n int, _ bool) error {
			if n >= 2 {
				return unavailable
			}
			return nil
		}
		start := time.Now()
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpDump, bridge.UIDumpArgs{})
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		if took := time.Since(start); took < c.idle {
			t.Fatalf("settled after %v, before the idle wait %v", took, c.idle)
		}
		d := decodeSettled(t, s)
		if !d.Settled || d.Changed != nil || len(d.Elements) == 0 {
			t.Fatalf("result %s", s.data[0])
		}
	})
}

// Changed is about the screen that comes out: one that changed and came back
// to the start is the start (and the CLI says the tap did nothing visible).
func TestSettleChangedIsAboutTheScreenThatComesOut(t *testing.T) {
	c := fastSettle()
	c.gap, c.idle = 20*time.Millisecond, 10*time.Second
	settleTimes(t, c)
	e := newEnv(t)
	a := e.settleApp()
	a.dumpFn = func(n int) bridge.UIDump {
		if n == 2 { // the start is A; then B for a moment and A again
			return dumpOf("com.android.settings", "B")
		}
		return dumpOf("com.android.settings", "A")
	}
	s, exit := e.helperScreen(t, context.Background(), bridge.UIOpClick, xy(1, 1))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	cs := decodeClicked(t, s)
	if cs.Screen == nil || !cs.Screen.Settled || textOf(cs.Screen) != "A" || !is(cs.Screen.Changed, false) {
		t.Fatalf("result %s", s.data[0])
	}
	if n := a.countAfter(bridge.UIOpDump); n < 3 {
		t.Fatalf("%d reads: it settled before the change", n)
	}
	if j := e.last(t); !strings.Contains(j.Reason, "settled=true") || !strings.Contains(j.Reason, "changed=false") {
		t.Fatalf("journal %+v", j)
	}
}

// screen --settle whose first read the app refuses (UI_PROTECTED: the window
// with no content of a transition) waits like one the guard blocked: the
// first valid read is the start; a refusal that lasts is a block, with no data.
func TestScreenSettleFirstReadProtected(t *testing.T) {
	protected := &UIError{Code: bridge.CodeProtected, Text: "ADBuddy does not read this screen."}
	t.Run("for a moment", func(t *testing.T) {
		c := fastSettle()
		c.blockWait = 10 * time.Second
		settleTimes(t, c)
		e := newEnv(t)
		a := e.settleApp()
		a.dumpErr = func(n int, _ bool) error {
			if n <= 2 {
				return protected
			}
			return nil
		}
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpDump, bridge.UIDumpArgs{})
		if exit.Code != 0 {
			t.Fatalf("exit %+v", exit)
		}
		d := decodeSettled(t, s)
		if !d.Settled || !is(d.Changed, false) || textOf(&d) != "same" {
			t.Fatalf("result %s", s.data[0])
		}
		if n := a.count(bridge.UIOpDump); n < 4 { // two refused, the start, a read
			t.Fatalf("%d dumps", n)
		}
		all := e.entriesAll(t)
		if len(all) != 1 || all[0].Status != journal.StatusOK || !strings.HasSuffix(all[0].Reason, ", settled=true") {
			t.Fatalf("journal %+v", all)
		}
	})
	t.Run("it lasts", func(t *testing.T) {
		c := fastSettle()
		c.blockWait = 60 * time.Millisecond
		settleTimes(t, c)
		e := newEnv(t)
		a := e.settleApp()
		a.dumpErr = func(int, bool) error { return protected }
		start := time.Now()
		s, exit := e.helperScreen(t, context.Background(), bridge.UIOpDump, bridge.UIDumpArgs{})
		if took := time.Since(start); took < c.blockWait {
			t.Fatalf("gave up after %v, before %v", took, c.blockWait)
		}
		if exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, "ADBuddy does not read this screen") || len(s.data) != 0 {
			t.Fatalf("exit %+v data %q", exit, s.data)
		}
		if n := a.count(bridge.UIOpDump); n < 2 {
			t.Fatalf("not tried again: %d dumps", n)
		}
		all := e.entriesAll(t)
		if len(all) != 1 || all[0].Status != journal.StatusBlocked || !strings.Contains(all[0].Reason, "ADBuddy does not read this screen") {
			t.Fatalf("journal %+v", all)
		}
	})
}
