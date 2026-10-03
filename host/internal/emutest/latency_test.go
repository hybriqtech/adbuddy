package emutest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// The device half of the latency harness (docs/superpowers/plans/2026-10-03-latencia.md):
// how long each of the agent's helpers and adb commands takes through the
// host's own daemon on the emulator, and where the time goes. The daemon runs
// in the test with an adb that times every call and a UI function that times
// every request to the real app, so the rest of the time (classifier, journal,
// guard logic) is what is left. It measures: it checks nothing and is off
// unless ADBUDDY_LATENCY=1:
//
//	ADBUDDY_LATENCY=1 ADBUDDY_LATENCY_OUT=<file.md> go test ./internal/emutest -run TestLatencyDevice -v
//
// (with ADBUDDY_EMULATOR_SERIAL, ADBUDDY_EMULATOR_ADB and ANDROID_ADB_SERVER_PORT
// as for any emutest). The emulator has no Wi-Fi between it and the PC: a real
// phone adds the round trip of each adb call (the plan counts them).

// stage is one timed call of a command.
type stage struct {
	label string
	d     time.Duration
}

// probe records the stages of the command running now.
type probe struct {
	mu     sync.Mutex
	stages []stage
}

func (p *probe) reset() {
	p.mu.Lock()
	p.stages = nil
	p.mu.Unlock()
}

func (p *probe) add(label string, d time.Duration) {
	p.mu.Lock()
	p.stages = append(p.stages, stage{label, d})
	p.mu.Unlock()
}

func (p *probe) take() []stage {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.stages
	p.stages = nil
	return s
}

// timedADB is the daemon's adb with every call timed.
type timedADB struct {
	adb.Runner
	p *probe
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func (a timedADB) Shell(ctx context.Context, command string) (string, int, error) {
	t0 := time.Now()
	out, code, err := a.Runner.Shell(ctx, command)
	a.p.add("adb shell "+clip(command, 40), time.Since(t0))
	return out, code, err
}

func (a timedADB) Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	t0 := time.Now()
	code, err := a.Runner.Run(ctx, dir, args, stdin, stdout, stderr)
	a.p.add("adb "+clip(strings.Join(args, " "), 40), time.Since(t0))
	return code, err
}

type noQuestions struct{ t *testing.T }

func (q noQuestions) Confirm(_ context.Context, r confirm.Request) confirm.Decision {
	q.t.Errorf("the latency harness was asked to confirm %q", r.Title)
	return confirm.Deny
}

// latencyHost is a daemon on the emulator with probes.
type latencyHost struct {
	d  *daemon.Daemon
	p  *probe
	ws string
	e  *emu
}

type uiFunc = func(context.Context, string, json.RawMessage) (json.RawMessage, error)

func newLatencyHost(t *testing.T, e *emu, ui uiFunc) *latencyHost {
	t.Helper()
	cls, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	p := &probe{}
	var timedUI uiFunc
	if ui != nil {
		timedUI = func(ctx context.Context, op string, args json.RawMessage) (json.RawMessage, error) {
			t0 := time.Now()
			raw, err := ui(ctx, op, args)
			p.add("app "+op, time.Since(t0))
			return raw, err
		}
	}
	return &latencyHost{p: p, ws: ws, e: e, d: daemon.New(daemon.Options{
		Classifier: cls, ADB: timedADB{e.adb, p}, Journal: journal.OpenSigned(filepath.Join(ws, "journal"), []byte("emutest")),
		Confirmer: noQuestions{t}, Serial: e.adb.Serial, Session: "emutest", Lang: "en", Workspace: ws, UI: timedUI,
	})}
}

func (h *latencyHost) exec(argv ...string) (ipc.Exit, int) {
	s := &guardStream{}
	exit := h.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindExec, Argv: argv, Cwd: h.ws, Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}, s)
	if exit.Error == "" {
		exit.Error = s.errOut.String()
	}
	return exit, s.out.Len() + len(s.data)
}

func (h *latencyHost) helper(op string, args any) (ipc.Exit, int) {
	raw, _ := json.Marshal(args)
	s := &guardStream{}
	exit := h.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUI, UIOp: op, UIArgs: raw, Argv: []string{"adbuddy", op}, Cwd: h.ws,
		Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}, s)
	return exit, s.out.Len() + len(s.data)
}

const daemonSelf = "(daemon itself: classify, journal, guard logic)"

// scenario times f n times (after warmup runs) and adds to rep the total and
// each stage's median per run, with how many times it ran per command. after
// runs outside the timing after each run (to put the screen back).
func scenario(t *testing.T, rep *testutil.Report, p *probe, name string, warmup, n int, f func() (ipc.Exit, int), after func()) {
	t.Helper()
	var totals testutil.Samples
	perStage := map[string][]time.Duration{} // label -> the sum of that label in each run
	counts := map[string]int{}
	bytesOut := 0
	for i := range warmup + n {
		p.reset()
		t0 := time.Now()
		exit, size := f()
		total := time.Since(t0)
		stages := p.take()
		if after != nil {
			after()
		}
		if exit.Code != 0 {
			t.Fatalf("%s: exit %d %s %s", name, exit.Code, exit.ErrorCode, exit.Error)
		}
		if i < warmup {
			continue
		}
		bytesOut = size
		totals = append(totals, total)
		sum := map[string]time.Duration{}
		cnt := map[string]int{}
		var timed time.Duration
		for _, s := range stages {
			sum[s.label] += s.d
			cnt[s.label]++
			timed += s.d
		}
		for l, d := range sum {
			perStage[l] = append(perStage[l], d)
			counts[l] = cnt[l]
		}
		perStage[daemonSelf] = append(perStage[daemonSelf], total-timed)
	}
	rep.Add(fmt.Sprintf("**%s** (%d B out)", name, bytesOut), totals)
	labels := make([]string, 0, len(perStage))
	for l := range perStage {
		labels = append(labels, l)
	}
	sort.Slice(labels, func(i, j int) bool {
		return testutil.Samples(perStage[labels[i]]).Median() > testutil.Samples(perStage[labels[j]]).Median()
	})
	for _, l := range labels {
		label := l
		if c := counts[l]; c > 1 {
			label = fmt.Sprintf("%s (x%d)", l, c)
		}
		rep.Add("&nbsp;&nbsp;↳ "+label, perStage[l])
	}
}

// appUI is the daemon's Options.UI over the link, as serve's uiLink does it.
func (l *link) appUI(t *testing.T) uiFunc {
	var n atomic.Int64
	return func(_ context.Context, op string, args json.RawMessage) (json.RawMessage, error) {
		id := fmt.Sprintf("lat_%d", n.Add(1))
		l.send(t, bridge.TypeUIRequest, bridge.UIRequest{ID: id, Op: op, Args: args, ExpiresAt: time.Now().Add(10 * time.Second).UTC()})
		for {
			r := payload[bridge.UIResponse](t, l.waitFor(t, bridge.TypeUIResponse, 15*time.Second))
			if r.ID != id {
				continue
			}
			if r.OK {
				return r.Result, nil
			}
			return nil, &daemon.UIError{Code: r.Error.Code, Text: r.Error.Text}
		}
	}
}

func TestLatencyDevice(t *testing.T) {
	if !testutil.LatencyOn() {
		t.Skip("set ADBUDDY_LATENCY=1")
	}
	n := 25
	l := openLink(t)
	e := l.e
	rep := testutil.NewReport("Device side on the emulator (no Wi-Fi hop): the agent's helpers and adb commands through the host's daemon")
	defer rep.Flush(t.Logf)

	e.restoreAtCleanup(t, "secure", "enabled_accessibility_services", e.shell(t, "settings get secure enabled_accessibility_services"))
	e.restoreAtCleanup(t, "secure", "accessibility_enabled", e.shell(t, "settings get secure accessibility_enabled"))
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")

	// Floors: what adb costs by itself, called as the daemon calls it.
	probeFloor := func(name string, args ...string) {
		rep.Add(name, testutil.Measure(3, n, func() {
			ctx, cancel := context.WithTimeout(e.ctx, time.Minute)
			defer cancel()
			if _, _, code, err := e.adb.Output(ctx, args...); err != nil || code != 0 {
				t.Fatalf("%v: %d %v", args, code, err)
			}
		}))
	}
	e.openSettings(t)
	probeFloor("floor: adb shell true", "shell", "true")
	probeFloor("floor: adb shell dumpsys window | grep mCurrentFocus (guard)", "shell", guardReads["focus"])
	probeFloor("floor: adb shell dumpsys window windows (guard, no app)", "shell", guardReads["windows"])
	probeFloor("floor: adb shell dumpsys activity activities (guard, no app)", "shell", guardReads["activities"])
	probeFloor("floor: adb shell dumpsys power", "shell", guardReads["power"])
	probeFloor("floor: adb exec-out screencap -p (raw PNG, full size)", "exec-out", "screencap", "-p")
	probeFloor("floor: adb exec-out uiautomator dump /dev/tty", "exec-out", "uiautomator", "dump", "/dev/tty")

	// With the app: its accessibility service on, as setup leaves it.
	if _, err := phoneapp.EnableAccessibility(e.ctx, e.adb, l.v); err != nil {
		t.Fatalf("accessibility: %v", err)
	}
	var a11y string
	if !poll(20*time.Second, func() bool { a11y = e.read(t, "", "accessibility"); return a11yBound(a11y) }) {
		t.Fatalf("ADBuddy's service is not bound 20 s after enabling it:\n%s", a11y)
	}
	e.openSettings(t)
	time.Sleep(2 * time.Second)

	back := func() {
		e.shell(t, "input keyevent KEYCODE_BACK")
		time.Sleep(700 * time.Millisecond)
	}
	tapNet := bridge.UIClickArgs{Text: "Network & internet"}

	h := newLatencyHost(t, e, l.appUI(t))
	run := func(name string, f func() (ipc.Exit, int), after func()) {
		scenario(t, rep, h.p, "app on: "+name, 3, n, f, after)
	}
	run("adbuddy foreground", func() (ipc.Exit, int) { return h.helper(bridge.UIOpForeground, bridge.Empty{}) }, nil)
	run("adbuddy screen", func() (ipc.Exit, int) { return h.helper(bridge.UIOpDump, bridge.UIDumpArgs{}) }, nil)
	run("adbuddy screen --all", func() (ipc.Exit, int) { return h.helper(bridge.UIOpDump, bridge.UIDumpArgs{All: true}) }, nil)
	run("adbuddy screenshot (scale 0.5, png)", func() (ipc.Exit, int) {
		return h.helper(daemon.OpScreenshot, daemon.ScreenshotArgs{Scale: 0.5, Format: "png"})
	}, nil)
	run("adbuddy screenshot (scale 0.5, jpeg)", func() (ipc.Exit, int) {
		return h.helper(daemon.OpScreenshot, daemon.ScreenshotArgs{Scale: 0.5, Format: "jpeg"})
	}, nil)
	run("adbuddy tap by text (then Back, untimed)", func() (ipc.Exit, int) { return h.helper(bridge.UIOpClick, tapNet) }, back)
	run("adb shell input keyevent 61 (guarded raw input)", func() (ipc.Exit, int) { return h.exec("shell", "input", "keyevent", "61") }, nil)
	run("adb shell getprop ro.build.version.sdk (green, no guard)", func() (ipc.Exit, int) {
		return h.exec("shell", "getprop", "ro.build.version.sdk")
	}, nil)
	run("adb shell dumpsys battery (green, no guard)", func() (ipc.Exit, int) { return h.exec("shell", "dumpsys", "battery") }, nil)

	// Without the app: dumpsys and uiautomator, the fallback.
	e.shell(t, "settings put secure enabled_accessibility_services ''")
	e.shell(t, "settings put secure accessibility_enabled 0")
	poll(15*time.Second, func() bool { return !a11yBound(e.read(t, "", "accessibility")) })
	e.openSettings(t)
	time.Sleep(2 * time.Second)
	h2 := newLatencyHost(t, e, nil)
	run2 := func(name string, f func() (ipc.Exit, int), after func()) {
		scenario(t, rep, h2.p, "no app: "+name, 3, n, f, after)
	}
	run2("adbuddy foreground", func() (ipc.Exit, int) { return h2.helper(bridge.UIOpForeground, bridge.Empty{}) }, nil)
	run2("adbuddy screen", func() (ipc.Exit, int) { return h2.helper(bridge.UIOpDump, bridge.UIDumpArgs{}) }, nil)
	run2("adbuddy screenshot (scale 0.5, png)", func() (ipc.Exit, int) {
		return h2.helper(daemon.OpScreenshot, daemon.ScreenshotArgs{Scale: 0.5, Format: "png"})
	}, nil)
	run2("adbuddy tap by text (then Back, untimed)", func() (ipc.Exit, int) { return h2.helper(bridge.UIOpClick, tapNet) }, back)
	run2("adb shell input keyevent 61 (guarded raw input)", func() (ipc.Exit, int) { return h2.exec("shell", "input", "keyevent", "61") }, nil)
}
