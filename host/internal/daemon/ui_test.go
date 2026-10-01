package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// fakeApp answers Options.UI: by operation, a result or an error.
type fakeApp struct {
	mu      sync.Mutex
	results map[string]any
	errs    map[string]error
	calls   []string
}

func (a *fakeApp) ui(_ context.Context, op string, args json.RawMessage) (json.RawMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, op+" "+string(args))
	if err := a.errs[op]; err != nil {
		return nil, err
	}
	return json.Marshal(a.results[op])
}

func (a *fakeApp) called() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string{}, a.calls...)
}

// withApp gives e an app with Settings in the foreground, and the list of
// protected apps.
func (e *env) withApp(protected ...string) *fakeApp {
	a := &fakeApp{results: map[string]any{
		bridge.UIOpForeground: bridge.UIForeground{Package: "com.android.settings", Activity: "com.android.settings.Settings"},
	}, errs: map[string]error{}}
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

func (e *env) helper(t *testing.T, op string, args any, argv ...string) (*stream, ipc.Exit) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	s := &stream{}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUI, UIOp: op, UIArgs: raw, Argv: argv,
		Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}, s)
	return s, exit
}

func xy(x, y int) bridge.UIClickArgs { return bridge.UIClickArgs{X: &x, Y: &y} }

var dumpResult = bridge.UIDump{Package: "com.android.settings", Width: 1344, Height: 2992, Elements: []bridge.UIElement{
	{ID: "android:id/title", Text: "Wi-Fi", Class: "TextView", Bounds: [4]int{0, 0, 10, 10}, Click: true, Enabled: true, Window: "app"},
}}

func TestScreenByTheApp(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.results[bridge.UIOpDump] = dumpResult
	s, exit := e.helper(t, bridge.UIOpDump, bridge.UIDumpArgs{}, "adbuddy", "screen")
	if exit.Code != 0 || len(s.data) != 1 {
		t.Fatalf("exit %+v data %q", exit, s.data)
	}
	var d bridge.UIDump
	if err := json.Unmarshal(s.data[0], &d); err != nil || len(d.Elements) != 1 || d.Elements[0].Text != "Wi-Fi" {
		t.Fatalf("dump %s (%v)", s.data[0], err)
	}
	j := e.last(t)
	if j.Level != "green" || j.Rule != "helper.screen" || j.Status != journal.StatusOK || j.Reason != "via the app" ||
		j.Source != journal.SourceAgent || strings.Join(j.Argv, " ") != "adbuddy screen" || j.Description != "Read the screen" {
		t.Fatalf("journal %+v", j)
	}
	// The guard asked what is in the foreground first; adb only said whether
	// the shade is open (the app is the one that says if a notification is up).
	if got := a.called(); len(got) != 2 || !strings.HasPrefix(got[0], "foreground") || !reflect.DeepEqual(e.adb.ran(), []string{"precheck: " + fixedFocusCmd}) {
		t.Fatalf("app %q adb %q", got, e.adb.ran())
	}
}

func TestScreenByADB(t *testing.T) {
	xml, err := os.ReadFile("../uidump/testdata/settings.xml")
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t) // no app
	e.adb.shell["dumpsys accessibility"] = "User state[\n  Enabled services:{}\n"
	e.adb.out["exec-out uiautomator dump /dev/tty"] = string(xml)
	s, exit := e.helper(t, bridge.UIOpDump, bridge.UIDumpArgs{}, "adbuddy", "screen")
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	var d bridge.UIDump
	if err := json.Unmarshal(s.data[0], &d); err != nil || d.Package != "com.android.settings" || len(d.Elements) == 0 {
		t.Fatalf("dump %s (%v)", s.data[0], err)
	}
	if j := e.last(t); j.Reason != "via adb" {
		t.Fatalf("journal %+v", j)
	}

	// With ADBuddy's service on, uiautomator would disconnect it.
	e.adb.shell["dumpsys accessibility"] = "Enabled services:{{tech.hybriq.adbuddy/tech.hybriq.adbuddy.a11y.AgentAccessibilityService}}"
	_, exit = e.helper(t, bridge.UIOpDump, bridge.UIDumpArgs{}, "adbuddy", "screen")
	if exit.Code != 75 || exit.ErrorCode != "UI_UNAVAILABLE" || !strings.Contains(exit.Error, "open the ADBuddy app") {
		t.Fatalf("service on: %+v", exit)
	}
	for _, r := range e.adb.ran()[len(e.adb.ran())-1:] {
		if strings.Contains(r, "uiautomator") {
			t.Fatalf("uiautomator ran with the service on: %q", e.adb.ran())
		}
	}
}

func TestScreenFallsBackWhenTheServiceIsOff(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.errs[bridge.UIOpDump] = &UIError{Code: bridge.CodeUnavailable, Text: "The accessibility service is off."}
	e.adb.shell["dumpsys accessibility"] = "Enabled services:{}"
	e.adb.out["exec-out uiautomator dump /dev/tty"] = `<hierarchy rotation="0"><node package="p.q" class="a.B" text="Hi" bounds="[0,0][10,10]"/></hierarchy>`
	if _, exit := e.helper(t, bridge.UIOpDump, bridge.UIDumpArgs{}); exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
}

func TestTap(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.results[bridge.UIOpClick] = bridge.UIClicked{Clicked: true, Bounds: [4]int{0, 600, 1344, 690}, How: "action"}
	s, exit := e.helper(t, bridge.UIOpClick, bridge.UIClickArgs{Text: "Network & internet"}, "adbuddy", "tap", "Network & internet")
	if exit.Code != 0 || !strings.Contains(string(s.data[0]), `"how":"action"`) {
		t.Fatalf("exit %+v data %q", exit, s.data)
	}
	j := e.last(t)
	if j.Level != "yellow" || j.Rule != "helper.tap" || j.Description != `Tap "Network & internet"` || j.Status != journal.StatusOK {
		t.Fatalf("journal %+v", j)
	}
}

func TestTapFailures(t *testing.T) {
	cases := []struct {
		err        error
		exit       int
		code, text string
	}{
		{&UIError{Code: bridge.CodeNotFound, Text: "No element matches."}, 1, "UI_NOT_FOUND", "No element"},
		{&UIError{Code: bridge.CodeAmbiguous, Text: "2 elements match \"OK\"."}, 1, "UI_AMBIGUOUS", "Use --id or --xy"},
		{&UIError{Code: bridge.CodeProtected, Text: "ADBuddy does not act on itself.\x1b[2J"}, 126, "UI_PROTECTED", `itself.\u001b[2J`},
		{ErrUITimeout, 75, "UI_TIMEOUT", "may or may not have happened"},
	}
	for _, c := range cases {
		e := newEnv(t)
		a := e.withApp()
		a.errs[bridge.UIOpClick] = c.err
		_, exit := e.helper(t, bridge.UIOpClick, bridge.UIClickArgs{Text: "OK"})
		if exit.Code != c.exit || exit.ErrorCode != c.code || !strings.Contains(exit.Error, c.text) {
			t.Errorf("%v: %+v", c.err, exit)
		}
		// A tap that may have happened is never repeated by adb.
		for _, r := range e.adb.ran() {
			if strings.Contains(r, "input") || strings.Contains(r, "uiautomator") {
				t.Errorf("%v: adb ran %q", c.err, e.adb.ran())
			}
		}
		if j := e.last(t); j.Status != journal.StatusError && j.Status != journal.StatusBlocked {
			t.Errorf("%v: journal %+v", c.err, j)
		}
	}
}

func TestTapByADB(t *testing.T) {
	xml, err := os.ReadFile("../uidump/testdata/settings.xml")
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t) // no app
	e.adb.shell["dumpsys accessibility"] = "Enabled services:{}"
	e.adb.out["exec-out uiautomator dump /dev/tty"] = string(xml)
	if _, exit := e.helper(t, bridge.UIOpClick, xy(10, 20)); exit.Code != 0 {
		t.Fatalf("xy: %+v", exit)
	}
	s, exit := e.helper(t, bridge.UIOpClick, bridge.UIClickArgs{Text: "Network & internet"})
	if exit.Code != 0 || !strings.Contains(string(s.data[0]), `[0,600,1344,690]`) {
		t.Fatalf("text: %+v %q", exit, s.data)
	}
	_, exit = e.helper(t, bridge.UIOpClick, bridge.UIClickArgs{Text: "Ok"})
	if exit.ErrorCode != "UI_AMBIGUOUS" {
		t.Fatalf("ambiguous: %+v", exit)
	}
	_, exit = e.helper(t, bridge.UIOpClick, bridge.UIClickArgs{Text: "Allow"})
	if exit.Code != 126 || exit.ErrorCode != "UI_PROTECTED" {
		t.Fatalf("ADBuddy's button: %+v", exit)
	}
	var taps []string
	for _, r := range e.adb.ran() {
		if strings.HasPrefix(r, "shell input") {
			taps = append(taps, r)
		}
	}
	if strings.Join(taps, "|") != "shell input tap 10 20|shell input tap 672 645" {
		t.Fatalf("taps %q", taps)
	}
}

func TestTypeByADB(t *testing.T) {
	e := newEnv(t)
	if _, exit := e.helper(t, bridge.UIOpSetText, bridge.UISetTextArgs{Text: "it's a test"}); exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	if ran := e.adb.ran(); ran[len(ran)-1] != `shell input text 'it'\''s%sa%stest'` {
		t.Fatalf("adb ran %q", ran)
	}
	for _, text := range []string{"¿Qué tal?", "50%s off", "tab\there"} {
		_, exit := e.helper(t, bridge.UIOpSetText, bridge.UISetTextArgs{Text: text})
		if exit.Code != 75 || exit.ErrorCode != "UI_UNAVAILABLE" {
			t.Errorf("%q: %+v", text, exit)
		}
	}
}

func TestTypeByTheApp(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.results[bridge.UIOpSetText] = bridge.UITextSet{Set: true}
	if _, exit := e.helper(t, bridge.UIOpSetText, bridge.UISetTextArgs{Text: "¿Qué tal? 👋"}); exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	if j := e.last(t); j.Description != `Type "¿Qué tal? 👋"` {
		t.Fatalf("journal %+v", j)
	}
}

func TestHelpersCheckTheirArgs(t *testing.T) {
	e := newEnv(t)
	long := strings.Repeat("a", bridge.MaxSetText+1)
	for _, c := range []struct {
		op   string
		args any
	}{
		{"nope", struct{}{}},
		{bridge.UIOpClick, bridge.UIClickArgs{}},
		{bridge.UIOpClick, bridge.UIClickArgs{ID: "a", Text: "b"}},
		{bridge.UIOpClick, bridge.UIClickArgs{X: new(int)}},
		{bridge.UIOpClick, xy(-1, 3)},
		{bridge.UIOpSetText, bridge.UISetTextArgs{}},
		{bridge.UIOpSetText, bridge.UISetTextArgs{Text: long}},
		{bridge.UIOpDump, map[string]any{"all": true, "extra": 1}},
		{OpScreenshot, ScreenshotArgs{Scale: 2, Format: "png"}},
		{OpScreenshot, ScreenshotArgs{Scale: 0.5, Format: "gif"}},
	} {
		if _, exit := e.helper(t, c.op, c.args); exit.Code != 2 || exit.ErrorCode != "UI_INVALID" {
			t.Errorf("%s %+v: %+v", c.op, c.args, exit)
		}
	}
	if len(e.adb.ran()) != 0 {
		t.Fatalf("adb ran %q", e.adb.ran())
	}
}

func TestHelpersRespectTheDoors(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.results[bridge.UIOpDump] = dumpResult
	paused := true
	e.d.o.Paused = func() bool { return paused }
	_, exit := e.helper(t, bridge.UIOpDump, bridge.UIDumpArgs{})
	if exit.Code != 126 || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, "Blocked by ADBuddy: paused") {
		t.Fatalf("paused: %+v", exit)
	}
	paused = false
	q := &question{title: "Reboot the phone", withdraw: func(error) {}}
	if busy := e.d.ask(q); busy != "" {
		t.Fatal(busy)
	}
	for _, op := range []string{bridge.UIOpDump, bridge.UIOpClick} {
		_, exit = e.helper(t, op, bridge.UIClickArgs{Text: "Allow"})
		if op == bridge.UIOpDump {
			_, exit = e.helper(t, op, bridge.UIDumpArgs{})
		}
		if exit.Code != 126 || !strings.Contains(exit.Error, `waiting to ask about "Reboot the phone"`) {
			t.Fatalf("%s with a question waiting: %+v", op, exit)
		}
	}
	if got := a.called(); len(got) != 0 {
		t.Fatalf("the app was asked %q", got)
	}
	if j := e.last(t); j.Status != journal.StatusBlocked {
		t.Fatalf("journal %+v", j)
	}
}

func TestATapCountsInFlight(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	in, out := make(chan struct{}), make(chan struct{})
	e.d.o.UI = func(ctx context.Context, op string, args json.RawMessage) (json.RawMessage, error) {
		if op == bridge.UIOpClick {
			close(in)
			<-out
		}
		return a.ui(ctx, op, args)
	}
	a.results[bridge.UIOpClick] = bridge.UIClicked{Clicked: true, Bounds: [4]int{1, 1, 2, 2}, How: "gesture"}
	done := make(chan ipc.Exit)
	go func() { _, exit := e.helper(t, bridge.UIOpClick, xy(1, 1)); done <- exit }()
	<-in
	e.d.askMu.Lock()
	n := len(e.d.inFlight)
	e.d.askMu.Unlock()
	close(out)
	if exit := <-done; exit.Code != 0 || n != 1 {
		t.Fatalf("exit %+v, in flight %d", exit, n)
	}
}

func TestScreenshot(t *testing.T) {
	e := newEnv(t)
	img := image.NewNRGBA(image.Rect(0, 0, 40, 80))
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	e.adb.out["exec-out screencap -p"] = b.String()
	s, exit := e.helper(t, OpScreenshot, ScreenshotArgs{Scale: 0.5, Format: "png"}, "adbuddy", "screenshot")
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	got, err := png.Decode(&s.out)
	if err != nil || got.Bounds().Dx() != 20 || got.Bounds().Dy() != 40 {
		t.Fatalf("image %v (%v)", got, err)
	}
	if j := e.last(t); j.Rule != "helper.screenshot" || j.Level != "green" {
		t.Fatalf("journal %+v", j)
	}
}

// The guard (M5 spec §4.3).

func TestGuardBlocksWithAProtectedAppInFront(t *testing.T) {
	cases := []struct {
		name string
		fg   bridge.UIForeground
		list []string
	}{
		{"in the list", bridge.UIForeground{Package: "com.example.bank"}, []string{"com.example.bank"}},
		{"the app says so", bridge.UIForeground{Package: "com.other.bank", Protected: true}, nil},
		{"ADBuddy", bridge.UIForeground{Package: "tech.hybriq.adbuddy"}, nil},
		{"protected without a package", bridge.UIForeground{Protected: true}, nil},
	}
	for _, c := range cases {
		e := newEnv(t)
		a := e.withApp(c.list...)
		a.results[bridge.UIOpForeground] = c.fg
		a.results[bridge.UIOpDump] = dumpResult
		for _, argv := range [][]string{{"shell", "input", "tap", "1", "1"}, {"exec-out", "screencap", "-p"}, {"shell", "uiautomator", "dump"}} {
			s, exit := e.exec(t, argv...)
			if exit.Code != 126 || !strings.Contains(s.errOut.String(), "is a protected app") {
				t.Errorf("%s: %q: %+v %q", c.name, argv, exit, s.errOut.String())
			}
		}
		for _, op := range []string{bridge.UIOpDump, bridge.UIOpClick, OpScreenshot} {
			args := any(bridge.UIDumpArgs{})
			if op == bridge.UIOpClick {
				args = xy(1, 1)
			} else if op == OpScreenshot {
				args = ScreenshotArgs{Scale: 1, Format: "png"}
			}
			if _, exit := e.helper(t, op, args); exit.Code != 126 || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, "protected app") {
				t.Errorf("%s: %s: %+v", c.name, op, exit)
			}
		}
		for _, r := range e.adb.ran() {
			if strings.Contains(r, "input") || strings.Contains(r, "screencap") || strings.Contains(r, "uiautomator") {
				t.Errorf("%s: adb ran %q", c.name, r)
			}
		}
	}
}

func TestGuardBlocksOpeningAProtectedApp(t *testing.T) {
	e := newEnv(t)
	e.withApp("com.example.bank")
	for _, argv := range [][]string{
		{"shell", "am", "start", "-n", "com.example.bank/.Main"},
		{"shell", "am", "start", "-p", "com.example.bank", "-a", "android.intent.action.VIEW"},
		{"shell", "am start 'intent:#Intent;component=com.example.bank/.Main;end'"},
		{"shell", "monkey", "-p", "com.example.bank", "1"},
		{"shell", "am", "start", "-n", "tech.hybriq.adbuddy/.MainActivity"},
	} {
		s, exit := e.exec(t, argv...)
		if exit.Code != 126 || !strings.Contains(s.errOut.String(), "is a protected app") {
			t.Errorf("%q: %+v %q", argv, exit, s.errOut.String())
		}
	}
	// Another app opens.
	if _, exit := e.exec(t, "shell", "am", "start", "-n", "com.android.settings/.Settings"); exit.Code != 0 {
		t.Fatalf("settings: %+v", exit)
	}
}

func TestGuardFailsClosed(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.errs[bridge.UIOpForeground] = ErrNoApp
	delete(e.adb.shell, "dumpsys activity activities") // dumpsys fails too
	for _, argv := range [][]string{{"shell", "input", "keyevent", "3"}, {"exec-out", "screencap", "-p"}} {
		s, exit := e.exec(t, argv...)
		if exit.Code != 126 || !strings.Contains(s.errOut.String(), "could not tell which app is in the foreground") {
			t.Errorf("%q: %+v %q", argv, exit, s.errOut.String())
		}
	}
	// What does not touch the screen is not guarded.
	if _, exit := e.exec(t, "shell", "getprop", "ro.product.model"); exit.Code != 0 {
		t.Fatalf("getprop: %+v", exit)
	}
}

// An empty package from the app with Protected=false carries no information:
// the guard goes to adb, as before.
func TestGuardEmptyPackageNotProtectedFallsBackToAdb(t *testing.T) {
	e := newEnv(t) // dumpsys says Settings
	a := e.withApp()
	a.results[bridge.UIOpForeground] = bridge.UIForeground{}
	if _, exit := e.exec(t, "shell", "input", "tap", "1", "1"); exit.Code != 0 {
		t.Fatalf("empty package, not protected: %+v", exit)
	}
	e.d.o.Protected = func(p string) bool { return p == "com.android.settings" }
	if _, exit := e.exec(t, "shell", "input", "tap", "1", "1"); exit.Code != 126 {
		t.Fatalf("adb says a protected app: %+v", exit)
	}
}

func TestGuardFallsBackToDumpsys(t *testing.T) {
	e := newEnv(t) // no app: dumpsys says Settings
	if _, exit := e.exec(t, "shell", "input", "tap", "1", "1"); exit.Code != 0 {
		t.Fatalf("settings in front: %+v", exit)
	}
	e.d.o.Protected = func(p string) bool { return p == "com.android.settings" }
	if _, exit := e.exec(t, "shell", "input", "tap", "1", "1"); exit.Code != 126 {
		t.Fatalf("protected by dumpsys: %+v", exit)
	}
	// Nothing resumed and the screen off: nothing to protect, the agent may
	// wake the screen.
	e.adb.shell["dumpsys activity activities"] = "ACTIVITY MANAGER ACTIVITIES (dumpsys activity activities)\n"
	e.adb.shell["dumpsys power"] = "POWER MANAGER\n  mWakefulness=Asleep\n"
	if _, exit := e.exec(t, "shell", "input", "keyevent", "224"); exit.Code != 0 {
		t.Fatalf("screen off: %+v", exit)
	}
	// Nothing resumed with the screen on (the lock screen, or a dumpsys this
	// code does not read): unknown, so it fails closed.
	e.adb.shell["dumpsys power"] = "POWER MANAGER\n  mWakefulness=Awake\n"
	if s, exit := e.exec(t, "exec-out", "screencap", "-p"); exit.Code != 126 || !strings.Contains(s.errOut.String(), "the screen is not off") {
		t.Fatalf("screen on: %+v %q", exit, s.errOut.String())
	}
	// A split screen with the bank in the other half.
	e.adb.shell["dumpsys activity activities"] = "ACTIVITY MANAGER ACTIVITIES\n  topResumedActivity=ActivityRecord{1 u0 com.android.settings/.Settings t1}\n    mResumedActivity: ActivityRecord{2 u0 com.example.bank/.Main t2}\n"
	e.d.o.Protected = func(p string) bool { return p == "com.example.bank" }
	if s, exit := e.exec(t, "shell", "input", "tap", "1", "1"); exit.Code != 126 || !strings.Contains(s.errOut.String(), "com.example.bank is a protected app") {
		t.Fatalf("split screen: %+v %q", exit, s.errOut.String())
	}
}

// A command that touches or reads the screen runs alone (M5 spec §4.3):
// otherwise it could open a protected app and act on it after the guard
// looked.
func TestGuardWantsScreenCommandsAlone(t *testing.T) {
	e := newEnv(t)
	e.withApp("com.example.bank")
	for _, argv := range [][]string{
		{"shell", "am start -a android.intent.action.VIEW -d upi://pay; sleep 3; input tap 540 1800"},
		{"shell", "input tap 100 200; input tap 540 1800"},
		{"shell", "input tap 1 1 && screencap -p /sdcard/x.png"},
		{"shell", "for i in 1 2; do input tap 1 1; done"},
		{"shell", "input tap $(echo 1) 2"},
		{"shell", "input", "keyevent", "187", "22", "66"},
		{"shell", "input", "keycombination", "113", "29"},
		{"shell", "input tap 1 1 &"},
	} {
		s, exit := e.exec(t, argv...)
		if exit.Code != 126 || !strings.Contains(s.errOut.String(), "must run alone") {
			t.Errorf("%q: %+v %q", argv, exit, s.errOut.String())
		}
	}
	for _, r := range e.adb.ran() {
		if strings.Contains(r, "input") || strings.Contains(r, "screencap") {
			t.Fatalf("adb ran %q", r)
		}
	}
	// One key, or a key with its options, is fine.
	for _, argv := range [][]string{{"shell", "input", "keyevent", "3"}, {"shell", "input", "keyevent", "--longpress", "26"}} {
		if _, exit := e.exec(t, argv...); exit.Code != 0 {
			t.Errorf("%q: %+v", argv, exit)
		}
	}
}

// A guarded command runs unwrapped: watch, flock, sh -c or env would act
// after the guard looked, in whatever app is then in front.
func TestGuardWantsScreenCommandsUnwrapped(t *testing.T) {
	e := newEnv(t)
	for _, line := range []string{
		"watch -n 1 input tap 540 1800",
		"watch -x -n 1 input keyevent 3",
		"flock /dev/null input tap 540 1800",
		"flock -w 60 /dev/null input tap 540 1800",
		"flock /dev/null screencap -p",
		"sh -c 'input tap 1 1'",
		"env input tap 1 1",
	} {
		s, exit := e.exec(t, "shell", line)
		if exit.Code != 126 || !strings.Contains(s.errOut.String(), "without wrappers") {
			t.Errorf("%q: %+v %q", line, exit, s.errOut.String())
		}
	}
	for _, r := range e.adb.ran() {
		if strings.Contains(r, "input") || strings.Contains(r, "screencap") {
			t.Fatalf("adb ran %q", r)
		}
	}
	for _, argv := range [][]string{
		{"shell", "input", "tap", "1", "1"},
		{"shell", "input tap 1 1"},
		{"exec-out", "screencap", "-p"},
		{"shell", "/system/bin/input tap 1 1"},
	} {
		s, exit := e.exec(t, argv...)
		if strings.Contains(s.errOut.String(), "must run alone") {
			t.Errorf("%q: %+v %q", argv, exit, s.errOut.String())
		}
	}
}

// screenrecord and cmd input are guarded too, once the user allows them.
func TestGuardCoversScreenrecordAndCmdInput(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.results[bridge.UIOpForeground] = bridge.UIForeground{Package: "tech.hybriq.adbuddy"}
	for _, argv := range [][]string{{"shell", "screenrecord", "--time-limit", "5", "/sdcard/a.mp4"}, {"shell", "cmd", "input", "tap", "1", "1"}} {
		s, exit := e.exec(t, argv...)
		if exit.Code != 126 || !strings.Contains(s.errOut.String(), "protected app") {
			t.Errorf("%q: %+v %q", argv, exit, s.errOut.String())
		}
	}
}

// A pause that comes while the guard looks stops the command (the guard can
// take seconds).
func TestPauseDuringTheGuard(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	var paused atomic.Bool
	e.d.o.Paused = paused.Load
	e.d.o.UI = func(ctx context.Context, op string, args json.RawMessage) (json.RawMessage, error) {
		if op == bridge.UIOpForeground {
			paused.Store(true)
		}
		return a.ui(ctx, op, args)
	}
	s, exit := e.exec(t, "shell", "input", "tap", "1", "1")
	if exit.Code != 126 || !strings.Contains(s.errOut.String(), "paused") {
		t.Fatalf("%+v %q", exit, s.errOut.String())
	}
	for _, r := range e.adb.ran() {
		if strings.Contains(r, "input") {
			t.Fatalf("adb ran %q", r)
		}
	}
}

// The agent gone while the guard looks: abandoned, not blocked.
func TestAgentGoneDuringTheGuard(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	e.d.o.UI = func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
		cancel()
		return nil, ErrNoApp
	}
	s := &stream{}
	exit := e.d.Handle(ctx, ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "input", "tap", "1", "1"}, Cwd: t.TempDir(), Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}, s)
	if j := e.last(t); exit.Code != 126 || j.Status != journal.StatusDenied || !strings.Contains(j.Reason, "abandoned by the agent") {
		t.Fatalf("%+v %+v", exit, j)
	}
}

func TestForegroundHelper(t *testing.T) {
	e := newEnv(t)
	e.withApp("com.android.settings")
	s, exit := e.helper(t, bridge.UIOpForeground, struct{}{}, "adbuddy", "foreground")
	if exit.Code != 0 || !strings.Contains(string(s.data[0]), `"package":"com.android.settings"`) {
		t.Fatalf("exit %+v data %q", exit, s.data)
	}
}

// M5b: the default capture (scale 0, format "") is decided here, by the
// daemon, before the journal entry is made.
func pngBytes(t *testing.T) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 40, 80))); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestScreenshotDefaults(t *testing.T) {
	cases := []struct {
		name   string
		remote func() bool
		args   ScreenshotArgs
		format string
		w, h   int
		argv   string
	}{
		{"plain default", nil, ScreenshotArgs{}, "png", 20, 40, "adbuddy screenshot --scale 0.5 --format png"},
		{"remote default", func() bool { return true }, ScreenshotArgs{}, "jpeg", 20, 40, "adbuddy screenshot --scale 0.5 --format jpeg"},
		{"remote off", func() bool { return false }, ScreenshotArgs{}, "png", 20, 40, "adbuddy screenshot --scale 0.5 --format png"},
		{"explicit png in remote", func() bool { return true }, ScreenshotArgs{Format: "png"}, "png", 20, 40, "adbuddy screenshot --scale 0.5 --format png"},
		{"explicit scale", nil, ScreenshotArgs{Scale: 1}, "png", 40, 80, "adbuddy screenshot --scale 1 --format png"},
		{"explicit scale and jpeg in remote", func() bool { return true }, ScreenshotArgs{Scale: 1, Format: "jpeg"}, "jpeg", 40, 80, "adbuddy screenshot --scale 1 --format jpeg"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			e.d.o.RemoteActive = c.remote
			e.adb.out["exec-out screencap -p"] = pngBytes(t)
			s, exit := e.helper(t, OpScreenshot, c.args, "adbuddy", "screenshot")
			if exit.Code != 0 {
				t.Fatalf("exit %+v", exit)
			}
			var img image.Image
			var err error
			if c.format == "png" {
				img, err = png.Decode(&s.out)
			} else {
				img, err = jpeg.Decode(&s.out)
			}
			if err != nil || img.Bounds().Dx() != c.w || img.Bounds().Dy() != c.h {
				t.Fatalf("image %v (%v)", img, err)
			}
			if got := strings.Join(e.last(t).Argv, " "); got != c.argv {
				t.Fatalf("journal argv %q, want %q", got, c.argv)
			}
		})
	}
}

// The result says what was done, so the client names its file after it.
func TestScreenshotResultFormat(t *testing.T) {
	e := newEnv(t)
	e.d.o.RemoteActive = func() bool { return true }
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	s, exit := e.helper(t, OpScreenshot, ScreenshotArgs{}, "adbuddy", "screenshot")
	if exit.Code != 0 || len(s.data) != 1 {
		t.Fatalf("exit %+v, data %d", exit, len(s.data))
	}
	var r ScreenshotResult
	if err := json.Unmarshal(s.data[0], &r); err != nil || r.Format != "jpeg" || r.Bytes != s.out.Len() {
		t.Fatalf("result %+v (%v)", r, err)
	}
}

// What is explicit and wrong stays UI_INVALID, in remote mode too.
func TestScreenshotInvalidWithRemote(t *testing.T) {
	e := newEnv(t)
	e.d.o.RemoteActive = func() bool { return true }
	for _, a := range []ScreenshotArgs{{Scale: -0.1}, {Scale: 1.5}, {Format: "gif"}, {Scale: 0.5, Format: "PNG"}} {
		if _, exit := e.helper(t, OpScreenshot, a); exit.Code != 2 || exit.ErrorCode != "UI_INVALID" {
			t.Errorf("%+v: %+v", a, exit)
		}
	}
	if len(e.adb.ran()) != 0 {
		t.Fatalf("adb ran %q", e.adb.ran())
	}
}
