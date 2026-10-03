package emutest

import (
	"context"
	"encoding/json"
	"html"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

// linkWait is how long openLink waits for the app's first device.info.
const linkWait = 10 * time.Second

// link is the real channel with the app on the emulator, as `adbuddy serve`
// opens it: the host's bridge on 127.0.0.1, `adb reverse` of its port and the
// token handed over by phoneapp.Handshake. The tests play the host: they send
// with send or reply and read what the app sends with waitFor.
type link struct {
	e    *emu
	v    phoneapp.Verified
	port int
	// Info is the app's first device.info, which openLink waited for.
	Info bridge.DeviceInfo

	token string // never logged

	srv  *bridge.Server
	stop func() // stops srv and waits for it

	mu     sync.Mutex
	wake   chan struct{} // closed and replaced on each message or hello
	msgs   []bridge.Message
	hellos []bridge.Hello // tokens blanked
}

// openLink installs the app on the emulator (as installApp, in the
// background and out of battery optimisation, as setup leaves it), opens the
// channel and waits for the app's hello and its first device.info. Everything
// is undone at cleanup: the bridge stops, the emulator's reverses go and the
// app is uninstalled.
func openLink(t *testing.T) *link {
	t.Helper()
	e := newEmu(t)
	v := e.installApp(t)
	if err := phoneapp.AllowBackground(e.ctx, e.adb, v); err != nil {
		t.Fatalf("battery optimisation: %v", err)
	}
	e.atCleanup(t, "shell", "dumpsys deviceidle whitelist -"+phoneapp.Package)
	token, err := bridge.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	l := &link{e: e, v: v, token: token, wake: make(chan struct{})}
	l.listen(t, 0)
	t.Cleanup(func() { l.stop() })
	e.atCleanup(t, "reverse", "--remove-all")
	if err := phoneapp.Tunnel(e.ctx, e.adb, l.port); err != nil {
		t.Fatalf("adb reverse: %v", err)
	}
	l.handshake(t)
	m := l.waitFor(t, bridge.TypeDeviceInfo, linkWait)
	l.Info = payload[bridge.DeviceInfo](t, m)
	if len(l.Hellos()) == 0 {
		t.Fatal("device.info came without a hello")
	}
	return l
}

// listen starts the bridge on 127.0.0.1:port (0: a free one) with the link's
// token, and remembers the port.
func (l *link) listen(t *testing.T, port int) {
	t.Helper()
	ln, err := bridge.Listen(port)
	if err != nil {
		t.Fatalf("listen on 127.0.0.1:%d: %v", port, err)
	}
	l.port = ln.Addr().(*net.TCPAddr).Port
	l.srv = bridge.New(bridge.Options{
		Token:       l.token,
		Session:     "emutest",
		HostVersion: "emutest",
		OnMessage:   l.put,
		Admit: func(_ context.Context, h bridge.Hello) bool {
			h.Token = ""
			l.mu.Lock()
			l.hellos = append(l.hellos, h)
			l.signalLocked()
			l.mu.Unlock()
			return true
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = l.srv.Serve(ctx, ln)
	}()
	var once sync.Once
	l.stop = func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

// handshake hands the token and the port to the app again, as serve does
// when the app's process is gone.
func (l *link) handshake(t *testing.T) {
	t.Helper()
	if err := phoneapp.Handshake(l.e.ctx, l.e.adb, l.v, l.token, l.port, "emutest"); err != nil {
		t.Fatalf("handshake: %v", err)
	}
}

// cut stops the host's side of the channel, as a host that went away: the
// app loses its connection and keeps trying.
func (l *link) cut() { l.stop() }

// resume starts the host's side again on the same port with the same token:
// the app reconnects by itself.
func (l *link) resume(t *testing.T) {
	t.Helper()
	l.listen(t, l.port)
}

// put keeps a message of the app for waitFor.
func (l *link) put(m bridge.Message) {
	l.mu.Lock()
	l.msgs = append(l.msgs, m)
	l.signalLocked()
	l.mu.Unlock()
}

func (l *link) signalLocked() {
	close(l.wake)
	l.wake = make(chan struct{})
}

// Hellos returns the hellos of the app so far, oldest first, without their
// token: one per connection.
func (l *link) Hellos() []bridge.Hello {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]bridge.Hello(nil), l.hellos...)
}

// waitHellos waits until the app has said hello n times in all, or fails.
func (l *link) waitHellos(t *testing.T, n int, d time.Duration) {
	t.Helper()
	if !l.until(d, func() bool { return len(l.hellos) >= n }) {
		t.Fatalf("hello %d did not come in %d s (%d so far)", n, int(d/time.Second), len(l.Hellos()))
	}
}

// waitFor returns the oldest message of type typ the app sent that no waitFor
// returned yet, waiting up to d for it, or fails.
func (l *link) waitFor(t *testing.T, typ string, d time.Duration) bridge.Message {
	t.Helper()
	m, ok := l.take(typ, d)
	if !ok {
		t.Fatalf("no %s in %d s", typ, int(d/time.Second))
	}
	return m
}

// drain forgets the messages no waitFor returned: what comes next is new.
func (l *link) drain() {
	l.mu.Lock()
	l.msgs = nil
	l.mu.Unlock()
}

// take is waitFor without failing.
func (l *link) take(typ string, d time.Duration) (bridge.Message, bool) {
	var got bridge.Message
	ok := l.until(d, func() bool {
		for i, m := range l.msgs {
			if m.Type == typ {
				got = m
				l.msgs = append(l.msgs[:i:i], l.msgs[i+1:]...)
				return true
			}
		}
		return false
	})
	return got, ok
}

// until runs cond with l.mu held each time something arrives, until it
// returns true or d passes.
func (l *link) until(d time.Duration, cond func() bool) bool {
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	for {
		l.mu.Lock()
		if cond() {
			l.mu.Unlock()
			return true
		}
		wake := l.wake
		l.mu.Unlock()
		select {
		case <-wake:
		case <-deadline.C:
			return false
		case <-l.e.ctx.Done():
			return false
		}
	}
}

// send sends a message to the app by its class: an event (kept and resent)
// or a stream message. A reply goes with reply.
func (l *link) send(t *testing.T, typ string, p any) {
	t.Helper()
	var err error
	switch bridge.HostClasses[typ] {
	case bridge.ClassEvent:
		err = l.srv.Publish(typ, p)
	case bridge.ClassStream:
		err = l.srv.Send(typ, p)
	default:
		t.Fatalf("send %s: not an event nor a stream message", typ)
	}
	if err != nil {
		t.Fatalf("send %s: %v", typ, err)
	}
}

// reply answers req, a request of the app.
func (l *link) reply(t *testing.T, req bridge.Message, typ string, p any) {
	t.Helper()
	if err := l.srv.Reply(req, typ, p); err != nil {
		t.Fatalf("reply %s: %v", typ, err)
	}
}

// connected reports whether the app is connected now.
func (l *link) connected() bool { return l.srv.Connected() }

// payload decodes m's payload as a T, or fails.
func payload[T any](t *testing.T, m bridge.Message) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(m.Payload, &v); err != nil {
		t.Fatalf("%s payload: %v: %s", m.Type, err, m.Payload)
	}
	return v
}

// atCleanup runs adb with args at cleanup, while the emulator is online (as
// uninstallAtCleanup), ignoring its answer.
func (e *emu) atCleanup(t *testing.T, args ...string) {
	t.Cleanup(func() {
		c := *e
		c.ctx = context.WithoutCancel(e.ctx)
		if c.state() != "device" {
			return
		}
		ctx, cancel := context.WithTimeout(c.ctx, time.Minute)
		defer cancel()
		_, _, _, _ = c.adb.Output(ctx, args...)
	})
}

// The channel of M2 end to end (validation 7 of M2, its link): the app gets
// the token by adb, reaches the host's bridge through `adb reverse` and says
// hello with it; on the welcome it sends device.info with the protected apps.
func TestTheLinkCarriesDeviceInfo(t *testing.T) {
	l := openLink(t)
	open, err := phoneapp.TunnelOpen(l.e.ctx, l.e.adb, l.port)
	if err != nil || !open {
		t.Errorf("reverse of port %d open = %v, %v", l.port, open, err)
	}
	hs := l.Hellos()
	if len(hs) != 1 {
		t.Fatalf("hellos = %d, want 1", len(hs))
	}
	h := hs[0]
	if h.Protocol < bridge.MinAppProtocol || h.AppVersion == "" {
		t.Errorf("hello = protocol %d, appVersion %q", h.Protocol, h.AppVersion)
	}
	if v, err := phoneapp.InstalledVersion(l.e.ctx, l.e.adb); err != nil || v != h.AppVersion {
		t.Errorf("hello's appVersion %q, installed %q (%v)", h.AppVersion, v, err)
	}
	if l.Info.ProtectedApps == nil {
		t.Error("the first device.info has no protectedApps: the host would keep an old list")
	}
	for _, p := range l.Info.ProtectedApps {
		if !bridge.ValidPackage(p) {
			t.Errorf("protected app %q is not a package name", p)
		}
	}
	if !l.connected() {
		t.Error("the app is not connected after device.info")
	}
}

// poll checks cond every half second until it holds or d passes.
func poll(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(500 * time.Millisecond)
	}
	return true
}

// pidOf returns the pid of the app's process, waiting up to 15 s for one.
func (e *emu) pidOf(t *testing.T) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		out, _, _, _ := e.adb.Output(e.ctx, "shell", "pidof "+phoneapp.Package)
		if pid := strings.TrimSpace(strings.ReplaceAll(out, "\r", "")); pid != "" && !strings.Contains(pid, " ") {
			return pid
		}
		if time.Now().After(deadline) {
			t.Fatalf("no single process of %s: %q", phoneapp.Package, out)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// Validation 6 of M3b: the channel comes back with the app in the
// background (never opened) when the host's side drops and returns; when the
// app's process dies and the system restarts its service (su 0 kill -9: am
// kill does not kill it, T1-9), without a new handshake, because the app kept
// the link; and after am force-stop, with the handshake serve does again.
func TestTheLinkComesBack(t *testing.T) {
	l := openLink(t)
	e := l.e

	l.drain()
	l.cut()
	if !poll(10*time.Second, func() bool { return !l.connected() }) {
		t.Fatal("still connected 10 s after the host's side stopped")
	}
	l.resume(t)
	l.waitHellos(t, 2, 40*time.Second)
	l.waitFor(t, bridge.TypeDeviceInfo, linkWait)

	l.drain()
	pid := e.pidOf(t)
	e.shell(t, "su 0 kill -9 "+pid)
	l.waitHellos(t, 3, 40*time.Second)
	l.waitFor(t, bridge.TypeDeviceInfo, linkWait)
	if now := e.pidOf(t); now == pid {
		t.Errorf("the app's process is still %s after kill -9", pid)
	}

	l.drain()
	e.shell(t, "am force-stop "+phoneapp.Package)
	if !poll(10*time.Second, func() bool { return !l.connected() }) {
		t.Fatal("still connected 10 s after am force-stop")
	}
	l.handshake(t)
	l.waitHellos(t, 4, 20*time.Second)
	l.waitFor(t, bridge.TypeDeviceInfo, linkWait)
	var dump string
	if !poll(20*time.Second, func() bool {
		dump = e.shell(t, "dumpsys activity services "+phoneapp.Package)
		return inForeground(dump, ".bridge.BridgeService")
	}) {
		t.Errorf("BridgeService is not in the foreground after the new handshake:\n%s", dump)
	}
}

// openLink leaves no reverse behind.
func TestTheLinkIsUndoneAtCleanup(t *testing.T) {
	e := newEmu(t)
	port := 0
	t.Run("open", func(t *testing.T) { port = openLink(t).port })
	out := e.run(t, "reverse", "--list")
	if strings.Contains(out, "tcp:"+strconv.Itoa(port)) {
		t.Errorf("the reverse of port %d is still there after the test: %q", port, out)
	}
}

// restoreAtCleanup puts back the value of a setting read before a test
// changed it ("null": it was not set, so it is deleted).
func (e *emu) restoreAtCleanup(t *testing.T, namespace, key, prev string) {
	if prev == "null" || prev == "" {
		e.atCleanup(t, "shell", shq.Join("settings", "delete", namespace, key))
		return
	}
	e.atCleanup(t, "shell", shq.Join("settings", "put", namespace, key, prev))
}

// uiRequest sends a ui.request and returns the app's ui.response to it.
func (l *link) uiRequest(t *testing.T, id, op string, args any) bridge.UIResponse {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	l.send(t, bridge.TypeUIRequest, bridge.UIRequest{ID: id, Op: op, Args: raw, ExpiresAt: time.Now().Add(10 * time.Second).UTC()})
	for {
		r := payload[bridge.UIResponse](t, l.waitFor(t, bridge.TypeUIResponse, 15*time.Second))
		if r.ID == id {
			return r
		}
	}
}

// searchField reads the text of Settings' search field from a uiautomator
// dump of the screen ("" and false when it is not on screen).
func searchField(dump string) (string, bool) {
	node := regexp.MustCompile(`<node [^>]*resource-id="com\.google\.android\.settings\.intelligence:id/open_search_view_edit_text"[^>]*>`).FindString(dump)
	m := regexp.MustCompile(` text="([^"]*)"`).FindStringSubmatch(node)
	if m == nil {
		return "", false
	}
	return html.UnescapeString(m[1]), true
}

func TestSearchField(t *testing.T) {
	dump := `<hierarchy rotation="0"><node index="0" text="&#191;Qu&#233; tal? &amp; 👋" resource-id="com.google.android.settings.intelligence:id/open_search_view_edit_text" class="android.widget.EditText" focused="true" /></hierarchy>`
	if got, ok := searchField(dump); !ok || got != "¿Qué tal? & 👋" {
		t.Errorf("searchField = %q, %v", got, ok)
	}
	if _, ok := searchField(`<hierarchy><node text="x" resource-id="android:id/title" /></hierarchy>`); ok {
		t.Error("a field read where there is none")
	}
}

// Validation 2 of M5a: the agent's `type` (ui.request setText by the real
// channel) writes Unicode text into another app's field, here the focused
// search field of Settings, with ADBuddy's accessibility service on as setup
// leaves it; uiautomator reads the field back.
func TestTypeWritesUnicodeIntoAField(t *testing.T) {
	l := openLink(t)
	e := l.e
	const text = "¿Qué tal? 👋"
	e.restoreAtCleanup(t, "secure", "enabled_accessibility_services", e.shell(t, "settings get secure enabled_accessibility_services"))
	e.restoreAtCleanup(t, "secure", "accessibility_enabled", e.shell(t, "settings get secure accessibility_enabled"))
	if _, err := phoneapp.EnableAccessibility(e.ctx, e.adb, l.v); err != nil {
		t.Fatalf("accessibility: %v", err)
	}
	e.atCleanup(t, "shell", "am force-stop com.google.android.settings.intelligence")
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")
	e.shell(t, "am start -W -a android.settings.APP_SEARCH_SETTINGS")

	// The service binds a moment after the setting (until then the app says
	// unavailable), and the field takes the focus a moment after the activity
	// starts (not_found).
	var r bridge.UIResponse
	for i := 1; ; i++ {
		r = l.uiRequest(t, "u_"+strconv.Itoa(i), bridge.UIOpSetText, bridge.UISetTextArgs{Text: text})
		if r.OK || r.Error == nil || r.Error.Code != bridge.CodeUnavailable && r.Error.Code != bridge.CodeNotFound || i == 20 {
			break
		}
		time.Sleep(time.Second)
	}
	if !r.OK {
		t.Fatalf("setText refused: %+v", r.Error)
	}
	var got string
	var dump string
	if !poll(10*time.Second, func() bool {
		dump = e.shell(t, "uiautomator dump /sdcard/adbuddy-emutest.xml >/dev/null && cat /sdcard/adbuddy-emutest.xml; rm -f /sdcard/adbuddy-emutest.xml")
		got, _ = searchField(dump)
		return got == text
	}) {
		t.Errorf("Settings' search field holds %q, want %q", got, text)
	}
}

// notificationRecord returns the block of `dumpsys notification --noredact`
// of pkg's notification on channel, or "" (the system's autogroup summary
// does not count).
func notificationRecord(dump, pkg, channel string) string {
	for _, rec := range strings.Split(dump, "NotificationRecord(")[1:] {
		// The autogroup summary (tag=ranker_group) shares the channel and has no content.
		if strings.Contains(rec, "pkg="+pkg+" ") && strings.Contains(rec, "channel="+channel+" ") && !strings.Contains(rec, "tag=ranker_group ") {
			return rec
		}
	}
	return ""
}

// Validation 6 of M6c: the phone and the PC in different languages. The app
// in Spanish (its own language, Android 13+: the system's needs a reboot) and
// the host in English: the 🔴 question that comes by the real channel shows
// the host's English title inside the app's Spanish text, with the app's
// Spanish buttons.
func TestSpanishAppEnglishHost(t *testing.T) {
	l := openLink(t)
	e := l.e
	e.shell(t, "pm grant "+phoneapp.Package+" android.permission.POST_NOTIFICATIONS")
	e.shell(t, "cmd locale set-app-locales "+phoneapp.Package+" --locales es")
	l.send(t, bridge.TypeConfirmReq, bridge.ConfirmRequest{
		ID: "j_1", Level: "red", Title: "Restart the phone", Detail: "may restart",
		RawCommand: "adb shell reboot", ExpiresAt: time.Now().Add(2 * time.Minute).UTC(),
	})
	var rec string
	if !poll(20*time.Second, func() bool {
		rec = notificationRecord(e.shell(t, "dumpsys notification --noredact"), phoneapp.Package, "confirm")
		return rec != ""
	}) {
		t.Fatal("no notification on the confirm channel 20 s after the confirm.request")
	}
	for _, want := range []string{"El agente quiere: Restart the phone", `"Permitir"`, `"Rechazar"`} {
		if !strings.Contains(rec, want) {
			t.Errorf("the question's notification lacks %s:\n%s", want, rec)
		}
	}
}
