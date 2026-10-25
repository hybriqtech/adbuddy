package emutest

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// The notice of the overlay when the host reads the screen (M5 spec §6
// «Cuándo», plan 2026-10-11-screen-overlay-frame): `adbuddy screenshot` asks
// the app for the notice (op notice) before screencap, and `adbuddy screen`
// (the app's dump) shows it too. Paused, neither shows it and both are refused.

// appGuard is the host's daemon with the app on the emulator behind its UI,
// as `adbuddy serve` wires it, and a pause the test sets.
type appGuard struct {
	d      *daemon.Daemon
	e      *emu
	ws     string
	ctx    context.Context
	paused *atomic.Bool

	mu  sync.Mutex
	ops []string // the notice and hideNotice ops the app got (noticeOps)
}

// appUI is daemon.Options.UI over the link: the app's answer, or a UIError
// with its code (what uiLink.Call returns).
func (l *link) appUI(t *testing.T) func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return func(_ context.Context, op string, args json.RawMessage) (json.RawMessage, error) {
		r := l.uiCall(t, op, args)
		if r.OK {
			return r.Result, nil
		}
		if r.Error == nil {
			return nil, &daemon.UIError{Code: bridge.CodeFailed, Text: "the app gave no reason"}
		}
		return nil, &daemon.UIError{Code: r.Error.Code, Text: r.Error.Text}
	}
}

func newAppGuard(t *testing.T, l *link) *appGuard {
	t.Helper()
	cls, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	g := &appGuard{e: l.e, ws: ws, ctx: l.e.ctx, paused: new(atomic.Bool)}
	ui := l.appUI(t)
	g.d = daemon.New(daemon.Options{
		Classifier: cls, ADB: l.e.adb, Journal: journal.OpenSigned(filepath.Join(ws, "journal"), []byte("emutest")),
		Confirmer: &questions{t: t}, Serial: l.e.adb.Serial, Session: "emutest", Lang: "en", Workspace: ws,
		UI: func(ctx context.Context, op string, args json.RawMessage) (json.RawMessage, error) {
			if op == bridge.UIOpNotice || op == bridge.UIOpHideNotice {
				g.mu.Lock()
				g.ops = append(g.ops, op)
				g.mu.Unlock()
			}
			return ui(ctx, op, args)
		},
		Paused: g.paused.Load,
	})
	return g
}

// noticeOps returns the notice and hideNotice ops the app got, in order, and forgets them.
func (g *appGuard) noticeOps() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	ops := g.ops
	g.ops = nil
	return ops
}

// helper runs one of the agent's helpers and returns its exit, its data and
// what it wrote on stdout (the image of a screenshot).
func (g *appGuard) helper(op string, args any) (ipc.Exit, string, []byte) {
	raw, _ := json.Marshal(args)
	s := &guardStream{}
	exit := g.d.Handle(g.ctx, ipc.Request{Kind: ipc.KindUI, UIOp: op, UIArgs: raw, Argv: []string{"adbuddy", op}, Cwd: g.ws,
		Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}, s)
	return exit, string(s.data), s.out.Bytes()
}

// read is helper for a read that must go through. Refused because a
// notification just arrived (on the emulator one always does right after
// a11yLink turns the service on), it waits 10 s and tries again, as the
// refusal tells the agent to, up to 5 times. That refusal comes before the
// notice: no overlay goes up.
func (g *appGuard) read(t *testing.T, op string, args any) (ipc.Exit, string, []byte) {
	t.Helper()
	for try := 1; ; try++ {
		exit, data, out := g.helper(op, args)
		if try == 5 || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, "A notification that just arrived") {
			return exit, data, out
		}
		t.Logf("%s, try %d: %s", op, try, exit.Error)
		if w := g.e.overlayWindows(t); len(w) != 0 {
			t.Errorf("%s was refused before the notice but the overlay is up: %v", op, w)
		}
		time.Sleep(10 * time.Second)
	}
}

// overlayGone waits up to 8 s for the overlay to hide (4 s after the last
// action), so that what shows it next is what the test does.
func (e *emu) overlayGone(t *testing.T, what string) {
	t.Helper()
	if !poll(8*time.Second, func() bool { return len(e.overlayWindows(t)) == 0 }) {
		t.Fatalf("%s: the overlay is still up 8 s on", what)
	}
}

// rawScreencap is a PNG of the screen by adb, past ADBuddy's host.
func (e *emu) rawScreencap(t *testing.T) image.Image {
	t.Helper()
	var out, errOut bytes.Buffer
	if code, err := e.adb.Run(e.ctx, "", []string{"exec-out", "screencap", "-p"}, nil, &out, &errOut); err != nil || code != 0 {
		t.Fatalf("screencap: exit %d, %v: %s", code, err, errOut.String())
	}
	return decodePNG(t, out.Bytes())
}

func decodePNG(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("not a PNG (%d bytes): %v", len(b), err)
	}
	return img
}

// edgeDiffers reports how many of a few points on the left and right edges
// of the screen (where the frame's 3 dp stroke is) differ between a and b.
func edgeDiffers(a, b image.Image) int {
	r := a.Bounds()
	n := 0
	for _, y := range []int{r.Dy() * 3 / 10, r.Dy() / 2, r.Dy() * 7 / 10} {
		for _, x := range []int{r.Min.X + 2, r.Max.X - 3} {
			ar, ag, ab, _ := a.At(x, y).RGBA()
			br, bg, bb, _ := b.At(x, y).RGBA()
			if ar != br || ag != bg || ab != bb {
				n++
			}
		}
	}
	return n
}

// diffFrac is the fraction of the pixels of region r (left, top, right,
// bottom) that differ between a and b.
func diffFrac(a, b image.Image, r [4]int) float64 {
	n, total := 0, 0
	for y := r[1]; y < r[3]; y++ {
		for x := r[0]; x < r[2]; x++ {
			total++
			ar, ag, ab, _ := a.At(x, y).RGBA()
			br, bg, bb, _ := b.At(x, y).RGBA()
			if ar != br || ag != bg || ab != bb {
				n++
			}
		}
	}
	return float64(n) / float64(total)
}

func TestTheNoticeWhenTheHostReadsTheScreen(t *testing.T) {
	l := a11yLink(t)
	e := l.e
	g := newAppGuard(t, l)

	// The screen as it is, without the overlay: what the frame is told from.
	e.overlayGone(t, "after the first dump")
	plain := e.rawScreencap(t)

	// adbuddy screenshot: the notice goes up, comes off the screen for the
	// capture (so the frame is not in the image: the other test checks it) and
	// is put back; the overlay may be gone again by now on a slow emulator.
	exit, data, img := g.read(t, daemon.OpScreenshot, daemon.ScreenshotArgs{Scale: 1, Format: "png"})
	if exit.Code != 0 {
		t.Fatalf("adbuddy screenshot: %+v", exit)
	}
	t.Logf("screenshot %s: overlay windows after the command: %v", data, e.overlayWindows(t))
	if n := edgeDiffers(plain, decodePNG(t, img)); n != 0 {
		t.Errorf("the frame is in the screenshot: %d of 6 edge points differ", n)
	}

	// adbuddy screen: the app's dump shows the notice too.
	e.overlayGone(t, "after the screenshot")
	if exit, data, _ := g.read(t, bridge.UIOpDump, bridge.UIDumpArgs{}); exit.Code != 0 || !strings.Contains(data, `"package":"`+settingsPkg+`"`) {
		t.Fatalf("adbuddy screen: %+v %.200s", exit, data)
	}
	e.waitOverlay(t)

	// Paused from the phone (session.state, as the host says it): no notice,
	// and both reads are refused. First only the app knows (a pause that has
	// not reached the host's gates), then the host too.
	e.overlayGone(t, "after the dump")
	l.send(t, bridge.TypeSessionState, bridge.SessionState{Agent: "emutest", State: "working", Paused: true})
	if !poll(10*time.Second, func() bool {
		r := l.uiCall(t, bridge.UIOpNotice, bridge.Empty{})
		return !r.OK && r.Error != nil && r.Error.Code == bridge.CodeProtected
	}) {
		t.Fatal("the app does not take itself as paused 10 s after session.state paused")
	}
	e.overlayGone(t, "after the paused probe")
	for _, c := range []struct {
		name, op, code string
		args           any
		why            string
	}{
		{"screenshot, the app paused", daemon.OpScreenshot, "UI_BLOCKED", daemon.ScreenshotArgs{Scale: 1, Format: "png"}, "on-screen notice"},
		{"screen, the app paused", bridge.UIOpDump, "UI_PROTECTED", bridge.UIDumpArgs{}, "paused"},
	} {
		exit, data, img := g.read(t, c.op, c.args)
		if exit.Code != daemon.ExitDenied || exit.ErrorCode != c.code || !strings.Contains(exit.Error, c.why) || len(img) != 0 {
			t.Errorf("%s: %+v %.200s (%d bytes out), want blocked because %q", c.name, exit, data, len(img), c.why)
		}
	}
	g.paused.Store(true)
	for _, op := range []string{daemon.OpScreenshot, bridge.UIOpDump} {
		var args any = bridge.UIDumpArgs{}
		if op == daemon.OpScreenshot {
			args = daemon.ScreenshotArgs{Scale: 1, Format: "png"}
		}
		exit, data, img := g.read(t, op, args)
		if exit.Code != daemon.ExitDenied || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, daemon.PausedMessage) || len(img) != 0 {
			t.Errorf("%s, paused: %+v %.200s (%d bytes out), want blocked as paused", op, exit, data, len(img))
		}
	}
	if w := e.overlayWindows(t); len(w) != 0 {
		t.Errorf("the overlay went up while paused: %v", w)
	}
}

// P1.2: the capture of adbuddy screenshot has neither the frame nor the pill,
// and both are back on the screen once it is done.
func TestTheScreenshotDoesNotCarryTheNotice(t *testing.T) {
	l := a11yLink(t)
	e := l.e
	g := newAppGuard(t, l)

	// A first read: it waits out the notification that always arrives right
	// after the service is turned on (it would sit over the pill's region),
	// and it shows the overlay, so that the pill's place is known.
	if exit, _, _ := g.read(t, bridge.UIOpDump, bridge.UIDumpArgs{}); exit.Code != 0 {
		t.Fatalf("adbuddy screen: %+v", exit)
	}
	pill := e.waitOverlay(t)
	e.overlayGone(t, "before the plain capture")
	plain := e.rawScreencap(t)

	// The control of this test: a raw capture with the overlay up has the
	// pill. Without it, a clean screenshot would prove nothing.
	l.uiCall(t, bridge.UIOpNotice, bridge.Empty{})
	e.waitOverlay(t)
	if d := diffFrac(plain, e.rawScreencap(t), pill); d < 0.5 {
		t.Fatalf("a raw capture with the overlay up differs from the plain one in only %.3f of the pill's region: the control does not see the pill", d)
	} else {
		t.Logf("control: a raw capture with the overlay up differs in %.3f of the pill's region %v", d, pill)
	}
	e.overlayGone(t, "after the control")
	g.noticeOps() // forget the ops so far

	start := time.Now()
	exit, data, img := g.read(t, daemon.OpScreenshot, daemon.ScreenshotArgs{Scale: 1, Format: "png"})
	took := time.Since(start)
	back := e.overlayWindows(t) // right after the command, before the 4 s that the notice stays
	if exit.Code != 0 {
		t.Fatalf("adbuddy screenshot: %+v", exit)
	}
	var res daemon.ScreenshotResult
	if err := json.Unmarshal([]byte(data), &res); err != nil || res.NoticeInImage {
		t.Fatalf("result %s (%v): the notice must have been taken off", data, err)
	}
	// The retries of g.read for a notification that just arrived come before any notice op.
	if got, want := g.noticeOps(), []string{bridge.UIOpNotice, bridge.UIOpHideNotice, bridge.UIOpNotice}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ops %v, want %v: the notice, then it comes off for the capture, then it is put back", got, want)
	}
	shot := decodePNG(t, img)
	if d := diffFrac(plain, shot, pill); d > 0.02 {
		t.Errorf("the pill is in the screenshot: %.3f of its region differs from the screen without the overlay", d)
	} else {
		t.Logf("the screenshot differs in %.3f of the pill's region", d)
	}
	if n := edgeDiffers(plain, shot); n != 0 {
		t.Errorf("the frame is in the screenshot: %d of 6 edge points differ", n)
	}
	if len(back) != 2 {
		t.Errorf("the frame and the pill are not back after the screenshot: %v", back)
	}
	t.Logf("adbuddy screenshot took %v with the notice taken off", took)
}

// The notice that was taken off comes back by itself, stays at least a moment
// before it goes, and does not come back for a paused session.
func TestTheHiddenNoticeComesBackByItself(t *testing.T) {
	l := a11yLink(t)
	e := l.e
	e.overlayGone(t, "start")

	// Nothing on the screen (the service on, the overlay gone): hideNotice
	// says it is off, and it does not put anything up when its time is up.
	if r := l.uiCall(t, bridge.UIOpHideNotice, bridge.UIHideNoticeArgs{Ms: 1500}); !r.OK {
		t.Fatalf("hideNotice with no overlay: %+v", r.Error)
	}
	if poll(3*time.Second, func() bool { return len(e.overlayWindows(t)) != 0 }) {
		t.Errorf("hideNotice with no overlay put the overlay up: %v", e.overlayWindows(t))
	}
	e.overlayGone(t, "after hideNotice with no overlay")

	hide := func(ms int) time.Time {
		t.Helper()
		l.uiCall(t, bridge.UIOpNotice, bridge.Empty{})
		e.waitOverlay(t)
		if r := l.uiCall(t, bridge.UIOpHideNotice, bridge.UIHideNoticeArgs{Ms: ms}); !r.OK {
			t.Fatalf("hideNotice: %+v", r.Error)
		}
		if w := e.overlayWindows(t); len(w) != 0 {
			t.Fatalf("hideNotice answered but the overlay is still up: %v", w)
		}
		return time.Now()
	}

	hide(1500)
	if !poll(5*time.Second, func() bool { return len(e.overlayWindows(t)) == 2 }) {
		t.Fatal("the overlay did not come back by itself 5 s after hideNotice 1500")
	}
	e.overlayGone(t, "after it came back")

	// The person sees the notice before it goes: it is up for 400 ms at least.
	l.uiCall(t, bridge.UIOpNotice, bridge.Empty{})
	t0 := time.Now()
	if r := l.uiCall(t, bridge.UIOpHideNotice, bridge.UIHideNoticeArgs{Ms: 1500}); !r.OK {
		t.Fatalf("hideNotice: %+v", r.Error)
	} else if dt := time.Since(t0); dt < 300*time.Millisecond {
		t.Errorf("hideNotice answered %v after the notice: the overlay was not up for the minimum", dt)
	} else {
		t.Logf("hideNotice answered %v after the notice", dt)
	}
	e.overlayGone(t, "after the dwell test")

	// Out of range: refused, and the overlay is untouched.
	for _, ms := range []int{0, bridge.MaxHideNoticeMs + 1} {
		if r := l.uiCall(t, bridge.UIOpHideNotice, bridge.UIHideNoticeArgs{Ms: ms}); r.OK || r.Error == nil || r.Error.Code != bridge.CodeInvalid {
			t.Errorf("hideNotice ms=%d: %+v, want invalid", ms, r)
		}
	}

	// Paused while it was off: it does not come back. The longest time, so
	// the app learns of the pause well before the timer.
	hiddenAt := hide(bridge.MaxHideNoticeMs)
	l.send(t, bridge.TypeSessionState, bridge.SessionState{Agent: "emutest", State: "working", Paused: true})
	if !poll(3*time.Second, func() bool {
		r := l.uiCall(t, bridge.UIOpHideNotice, bridge.UIHideNoticeArgs{Ms: 1500}) // the overlay is off: it only answers, it does not reschedule
		return !r.OK && r.Error != nil && r.Error.Code == bridge.CodeProtected
	}) {
		t.Fatal("the app does not take itself as paused 3 s after session.state paused")
	}
	time.Sleep(time.Until(hiddenAt.Add(bridge.MaxHideNoticeMs*time.Millisecond + time.Second)))
	if w := e.overlayWindows(t); len(w) != 0 {
		t.Errorf("the overlay came back for a paused session: %v", w)
	}
	// Nor does the heads-up for acting: while paused, the app refuses the op
	// before it shows anything (ruling of the leader, 2026-10-12). The click
	// is on an element that is on the screen (Settings, from a11yLink): not
	// paused, it would show the overlay and go through.
	if r := l.uiCall(t, bridge.UIOpClick, bridge.UIClickArgs{Text: "Network & internet"}); r.OK || r.Error == nil || r.Error.Code != bridge.CodeProtected {
		t.Errorf("a click while paused: %+v, want protected (paused)", r)
	}
	if w := e.overlayWindows(t); len(w) != 0 {
		t.Errorf("a refused click in a paused session put the overlay up: %v", w)
	}
}

// Plan 2026-10-11-screen-overlay-frame, validation (b): a dump of the locked
// screen either draws the notice over the keyguard or fails with "won't
// read"; what it must never do is read without the notice.
func TestADumpOfTheLockedScreen(t *testing.T) {
	l := a11yLink(t)
	e := l.e
	e.overlayGone(t, "before locking")
	e.lockWithPIN(t)
	r := l.uiCall(t, bridge.UIOpDump, bridge.UIDumpArgs{})
	frames := e.overlayWindows(t)
	switch {
	case r.OK && len(frames) == 2:
		d := payload[bridge.UIDump](t, bridge.Message{Type: "dump", Payload: r.Result})
		t.Logf("locked: the dump is read with the overlay over the keyguard (locked=%v, %d elements, frames %v)", d.Locked, len(d.Elements), frames)
	case r.OK:
		t.Errorf("locked: the dump was read without the overlay on screen (frames %v)", frames)
	case r.Error != nil && r.Error.Code == bridge.CodeFailed && strings.Contains(r.Error.Text, "won't read"):
		t.Logf("locked: the dump fails closed: %s", r.Error.Text)
	default:
		t.Errorf("locked: %+v, want ok with the overlay or FAILED (won't read)", r)
	}
}
