package emutest

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net"
	"regexp"
	"slices"
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

	// onFrame, when set, receives the app's binary frames (see setOnFrame).
	// It lives in the link, not in the Server, so it survives resume.
	onFrame func(id bridge.FrameID, offset int64, data []byte)
}

// openLink installs the app on the emulator (as installApp, in the
// background, out of battery optimisation as setup leaves it, and allowed to
// post notifications as after its first run), opens the channel and waits
// for the app's hello and its first device.info. Everything is undone at
// cleanup: the bridge stops, the emulator's reverses go and the app is
// uninstalled.
func openLink(t *testing.T) *link {
	t.Helper()
	e := newEmu(t)
	return e.linkTo(t, e.installApp(t))
}

// linkTo is openLink for the app v checked, already installed (and
// uninstalled at cleanup) by its caller: the debug app (installApp) or
// another build, such as the release one (installRelease).
func (e *emu) linkTo(t *testing.T, v phoneapp.Verified) *link {
	t.Helper()
	if err := phoneapp.AllowBackground(e.ctx, e.adb, v); err != nil {
		t.Fatalf("battery optimisation: %v", err)
	}
	e.atCleanup(t, "shell", "dumpsys deviceidle whitelist -"+phoneapp.Package)
	// The tests look for the app's English texts ("Not now", "Send", "Retry"):
	// the app's own language, whatever the emulator's.
	e.shell(t, "cmd locale set-app-locales "+phoneapp.Package+" --locales en")
	// As after the app's first run, where it asks: otherwise its notifications
	// (the conversation's, the questions') are not shown.
	e.shell(t, "pm grant "+phoneapp.Package+" android.permission.POST_NOTIFICATIONS")
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
	srv := bridge.New(bridge.Options{
		Token:       l.token,
		Session:     "emutest",
		HostVersion: "emutest",
		OnMessage:   l.put,
		OnFrame:     l.frame,
		Admit: func(_ context.Context, h bridge.Hello) bool {
			h.Token = ""
			l.mu.Lock()
			l.hellos = append(l.hellos, h)
			l.signalLocked()
			l.mu.Unlock()
			return true
		},
	})
	l.srv = srv
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Serve(ctx, ln)
	}()
	var once sync.Once
	l.stop = func() {
		once.Do(func() {
			cancel()
			<-done
			// What the old server's connection had in flight must not reach
			// the next one's tests: wait until it is gone.
			poll(5*time.Second, func() bool { return !srv.Connected() })
		})
	}
}

// frame hands a binary frame of the app to the handler set with setOnFrame.
// The server only lends data during the call: it is copied here.
func (l *link) frame(id bridge.FrameID, offset int64, data []byte) {
	l.mu.Lock()
	fn := l.onFrame
	l.mu.Unlock()
	if fn == nil {
		return
	}
	fn(id, offset, append([]byte(nil), data...))
}

// setOnFrame sets (nil: removes) the handler of the app's binary frames; it
// holds for the servers resume starts too. The handler runs on the
// connection's reader: it must never call cut (stop waits for the reader),
// and what it keeps is its own copy of the data.
func (l *link) setOnFrame(fn func(id bridge.FrameID, offset int64, data []byte)) {
	l.mu.Lock()
	l.onFrame = fn
	l.mu.Unlock()
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

// serveOpts are the ways a host can misbehave when it serves a file.
type serveOpts struct {
	// limit > 0: only that many bytes from the offset asked for, and no
	// file.end: the host stalls there (the test then cuts or cancels).
	limit int64
	// corrupt damages the first byte of every frame it sends.
	corrupt bool
	// fromZero ignores the offset the app asked for and serves from 0.
	fromZero bool
	// endSize, when not 0, is the Size file.end carries instead of the real one.
	endSize int64
}

// sendFile serves req, a file.get of the app, from data (the whole file): the
// binary frames from the offset asked for, in frames of MaxFrameData, and then
// file.end as a reply; the queue sends them in that order (spec M3 §6.2). It
// blocks while the app is slow to take them.
func (l *link) sendFile(t *testing.T, req bridge.Message, data []byte) {
	t.Helper()
	l.serveFile(t, req, data, serveOpts{})
}

// serveFile is sendFile with the host's misbehaviour of opt.
func (l *link) serveFile(t *testing.T, req bridge.Message, data []byte, opt serveOpts) {
	t.Helper()
	g := payload[bridge.FileGet](t, req)
	id, err := bridge.ParseFrameID(g.ID)
	if err != nil {
		t.Fatalf("file.get id %q: %v", g.ID, err)
	}
	if g.Offset < 0 || g.Offset > int64(len(data)) {
		t.Fatalf("file.get offset %d outside the %d bytes of the file", g.Offset, len(data))
	}
	from, end := g.Offset, int64(len(data))
	if opt.fromZero {
		from = 0
	}
	if opt.limit > 0 {
		end = min(end, from+opt.limit)
	}
	for off := from; off < end; {
		n := min(int64(bridge.MaxFrameData), end-off)
		chunk := data[off : off+n]
		if opt.corrupt {
			chunk = append([]byte(nil), chunk...)
			chunk[0] ^= 0xff
		}
		frame, err := bridge.EncodeFrame(id, off, chunk)
		if err != nil {
			t.Fatalf("frame at %d: %v", off, err)
		}
		if err := l.srv.SendFrameTo(l.e.ctx, req, frame); err != nil {
			t.Fatalf("send the frame at %d: %v", off, err)
		}
		off += n
	}
	if opt.limit > 0 && end < int64(len(data)) {
		return
	}
	size := int64(len(data))
	if opt.endSize != 0 {
		size = opt.endSize
	}
	l.reply(t, req, bridge.TypeFileEnd, bridge.FileEnd{ID: g.ID, Size: size})
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
	t.Helper()
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

var pidRe = regexp.MustCompile(`^\d+$`)

// pidOf returns the pid of the app's process, waiting up to 15 s for one.
func (e *emu) pidOf(t *testing.T) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		out, _, _, _ := e.adb.Output(e.ctx, "shell", "pidof "+phoneapp.Package)
		if pid := strings.TrimSpace(strings.ReplaceAll(out, "\r", "")); pidRe.MatchString(pid) {
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
	n := len(l.Hellos())
	l.cut()
	if !poll(10*time.Second, func() bool { return !l.connected() }) {
		t.Fatal("still connected 10 s after the host's side stopped")
	}
	l.resume(t)
	l.waitHellos(t, n+1, 40*time.Second)
	l.waitFor(t, bridge.TypeDeviceInfo, linkWait)

	l.drain()
	n = len(l.Hellos())
	pid := e.pidOf(t)
	e.shell(t, "su 0 kill -9 "+pid)
	l.waitHellos(t, n+1, 40*time.Second)
	l.waitFor(t, bridge.TypeDeviceInfo, linkWait)
	if now := e.pidOf(t); now == pid {
		t.Errorf("the app's process is still %s after kill -9", pid)
	}

	l.drain()
	n = len(l.Hellos())
	e.shell(t, "am force-stop "+phoneapp.Package)
	if !poll(10*time.Second, func() bool { return !l.connected() }) {
		t.Fatal("still connected 10 s after am force-stop")
	}
	l.handshake(t)
	l.waitHellos(t, n+1, 20*time.Second)
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
	if !t.Run("open", func(t *testing.T) { port = openLink(t).port }) {
		return
	}
	open, err := phoneapp.TunnelOpen(e.ctx, e.adb, port)
	if err != nil {
		t.Fatalf("reverse --list: %v", err)
	}
	if open {
		t.Errorf("the reverse of port %d is still there after the test", port)
	}
}

// restoreAtCleanup puts back the value of a setting read before a test
// changed it ("null": it was not set, so it is deleted).
func (e *emu) restoreAtCleanup(t *testing.T, namespace, key, prev string) {
	t.Helper()
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
		dump = e.dumpScreen(t)
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
	e.atCleanup(t, "shell", "cmd locale set-app-locales "+phoneapp.Package+" --locales en")
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

// publishRID publishes an event that confirms req, a request of the app.
func (l *link) publishRID(t *testing.T, req bridge.Message, typ string, p any) {
	t.Helper()
	if err := l.srv.PublishRID(typ, req.RID, p); err != nil {
		t.Fatalf("publish %s: %v", typ, err)
	}
}

// uiNode is a node of a uiautomator dump.
type uiNode struct {
	Text, Desc, ID, Class string
	Focused, Enabled      bool
	X, Y                  int // the middle of its bounds
	Parent                int // the index of its parent in the dump's nodes; -1 at the top
}

var boundsRe = regexp.MustCompile(`^\[(\d+),(\d+)\]\[(\d+),(\d+)\]$`)

// uiNodes reads the nodes of a uiautomator dump.
func uiNodes(dump string) ([]uiNode, error) {
	var nodes []uiNode
	var open []int // the nodes not closed yet, innermost last
	d := xml.NewDecoder(strings.NewReader(dump))
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return nodes, nil
		}
		if err != nil {
			return nodes, err
		}
		if ee, ok := tok.(xml.EndElement); ok && ee.Name.Local == "node" && len(open) > 0 {
			open = open[:len(open)-1]
			continue
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "node" {
			continue
		}
		n := uiNode{Parent: -1}
		if len(open) > 0 {
			n.Parent = open[len(open)-1]
		}
		for _, a := range se.Attr {
			switch a.Name.Local {
			case "text":
				n.Text = a.Value
			case "content-desc":
				n.Desc = a.Value
			case "resource-id":
				n.ID = a.Value
			case "class":
				n.Class = a.Value
			case "focused":
				n.Focused = a.Value == "true"
			case "enabled":
				n.Enabled = a.Value == "true"
			case "bounds":
				if m := boundsRe.FindStringSubmatch(a.Value); m != nil {
					l, _ := strconv.Atoi(m[1])
					tp, _ := strconv.Atoi(m[2])
					r, _ := strconv.Atoi(m[3])
					b, _ := strconv.Atoi(m[4])
					n.X, n.Y = (l+r)/2, (tp+b)/2
				}
			}
		}
		open = append(open, len(nodes))
		nodes = append(nodes, n)
	}
}

func TestUINodes(t *testing.T) {
	dump := `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?><hierarchy rotation="0"><node text="Not now" resource-id="" class="android.widget.TextView" content-desc="" enabled="true" bounds="[407,2216][544,2269]"><node text="" resource-id="a:id/b" class="android.view.View" content-desc="Send &amp; go" enabled="false" bounds="[965,2222][1028,2285]" /></node><node text="Next" class="android.widget.TextView" bounds="[0,0][20,20]" /></hierarchy>`
	got, err := uiNodes(dump)
	if err != nil {
		t.Fatal(err)
	}
	want := []uiNode{
		{Text: "Not now", Class: "android.widget.TextView", Enabled: true, X: 475, Y: 2242, Parent: -1},
		{ID: "a:id/b", Class: "android.view.View", Desc: "Send & go", X: 996, Y: 2253, Parent: 0},
		{Text: "Next", Class: "android.widget.TextView", X: 10, Y: 10, Parent: -1},
	}
	if !slices.Equal(got, want) {
		t.Errorf("uiNodes = %+v, want %+v", got, want)
	}
}

// screen returns the nodes on the emulator's screen now.
func (e *emu) screen(t *testing.T) []uiNode {
	t.Helper()
	nodes, err := uiNodes(e.dumpScreen(t))
	if err != nil {
		t.Fatalf("uiautomator dump: %v", err)
	}
	return nodes
}

// dumpScreen returns the uiautomator dump of the emulator's screen now.
//
// uiautomator dump sometimes fails (exit 255, "could not get idle state") while
// the screen is changing or the app is starting cold (the terminal's WebView
// keeps the UI busy for ~9 s). The shell's exit code is the dump's (rc is kept
// across the rm), and an empty dump counts as a failure too (it would read as
// an empty screen). It is tried again; the last failure is reported with its
// stderr.
func (e *emu) dumpScreen(t *testing.T) string {
	t.Helper()
	const f = "/sdcard/adbuddy-emutest.xml"
	const cmd = "uiautomator dump " + f + " >/dev/null && cat " + f + "; rc=$?; rm -f " + f + "; exit $rc"
	var last string
	for try := 1; try <= 5; try++ {
		ctx, cancel := context.WithTimeout(e.ctx, 3*time.Minute)
		out, errOut, code, err := e.adb.Output(ctx, "shell", cmd)
		cancel()
		out = strings.TrimSpace(strings.ReplaceAll(out, "\r", ""))
		if err == nil && code == 0 && out != "" {
			return out
		}
		last = fmt.Sprintf("exit %d, %v, stderr %q, %d bytes of dump", code, err, strings.TrimSpace(errOut), len(out))
		time.Sleep(time.Second)
	}
	t.Fatalf("uiautomator dump failed 5 times: %s", last)
	return ""
}

// waitNode waits up to d for a node that match accepts, or fails.
func (e *emu) waitNode(t *testing.T, what string, d time.Duration, match func(uiNode) bool) uiNode {
	t.Helper()
	var found uiNode
	var last []uiNode
	if !poll(d, func() bool {
		last = e.screen(t)
		for _, n := range last {
			if match(n) {
				found = n
				return true
			}
		}
		return false
	}) {
		t.Fatalf("no %s on screen in %d s: %+v", what, int(d/time.Second), last)
	}
	return found
}

// tap taps the middle of n.
func (e *emu) tap(t *testing.T, n uiNode) {
	t.Helper()
	e.shell(t, "input tap "+strconv.Itoa(n.X)+" "+strconv.Itoa(n.Y))
}

// openChat opens the app on its chat, skipping the first-run steps
// (notifications are allowed already: their dialog does not show).
func (l *link) openChat(t *testing.T) {
	t.Helper()
	e := l.e
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")
	e.shell(t, "am start -W -n "+phoneapp.Activity)
	for step := 1; step <= 2; step++ {
		e.tap(t, e.waitNode(t, "Not now (step "+strconv.Itoa(step)+")", 15*time.Second, func(n uiNode) bool {
			return n.Text == "Not now"
		}))
		// The next screen replaces this one: wait until its text changes.
		want := "Step 2 of 2"
		if step == 2 {
			want = "Message the agent"
		}
		// A cold start is slow here: the terminal's WebView loads (~9 s) and the UI is not idle.
		e.waitNode(t, want, 60*time.Second, func(n uiNode) bool { return n.Text == want })
	}
}

// composer returns the chat's text field: the EditText on the row of the
// Send button (the terminal's WebView has an EditText of its own).
func composer(nodes []uiNode) (uiNode, bool) {
	for _, send := range nodes {
		if send.Desc != "Send" {
			continue
		}
		for _, n := range nodes {
			if n.Class == "android.widget.EditText" && n.Y > send.Y-100 && n.Y < send.Y+100 {
				return n, true
			}
		}
	}
	return uiNode{}, false
}

// waitComposer waits up to d for the chat's text field to be as ok wants, or fails.
func (e *emu) waitComposer(t *testing.T, what string, d time.Duration, ok func(uiNode) bool) uiNode {
	t.Helper()
	var c uiNode
	var last []uiNode
	if !poll(d, func() bool {
		last = e.screen(t)
		var found bool
		c, found = composer(last)
		return found && ok(c)
	}) {
		t.Fatalf("the chat's field is not %s in %d s: %+v", what, int(d/time.Second), last)
	}
	return c
}

// Validation 1 of M3b: a message of several lines written in the chat
// reaches the host as one chat.send; with the channel cut it stays "Not
// sent", and Retry sends it again with the same id, which the host takes as
// the same message (onChatSend confirms it instead of typing it twice), and
// the chat shows it once. The text is ASCII: `input text` types no other, and
// API 34 has no clipboard command (T1); Unicode is TestTypeWritesUnicodeIntoAField's.
func TestTheChatSendsOneMessage(t *testing.T) {
	l := openLink(t)
	e := l.e
	l.openChat(t)

	e.tap(t, e.waitComposer(t, "on screen", 10*time.Second, func(uiNode) bool { return true }))
	e.waitComposer(t, "focused", 10*time.Second, func(n uiNode) bool { return n.Focused })
	e.shell(t, "input text first%sline")
	e.shell(t, "input keyevent KEYCODE_ENTER")
	e.shell(t, "input text second%sline")
	const typed = "first line\nsecond line"
	e.waitComposer(t, "holding the two lines", 10*time.Second, func(n uiNode) bool { return n.Text == typed })
	send := e.waitNode(t, "Send", 5*time.Second, func(n uiNode) bool { return n.Desc == "Send" })
	l.drain()
	e.tap(t, send)
	m, ok := l.take(bridge.TypeChatSend, 10*time.Second)
	if !ok {
		t.Fatalf("no chat.send in 10 s; on screen: %+v", e.screen(t))
	}
	cs := payload[bridge.ChatSend](t, m)
	if cs.Text != typed {
		t.Errorf("chat.send text = %q, want the two lines in one message", cs.Text)
	}
	l.publishRID(t, m, bridge.TypeChatMessage, bridge.ChatMessage{ID: cs.ID, Role: "user", Text: cs.Text, Attachments: []bridge.Attachment{}})
	if extra, ok := l.take(bridge.TypeChatSend, 3*time.Second); ok {
		t.Errorf("a second chat.send for one message: %s", extra.Payload)
	}

	// A message on its way when the channel drops: Not sent, then Retry.
	e.tap(t, e.waitComposer(t, "on screen", 10*time.Second, func(uiNode) bool { return true }))
	e.shell(t, "input text again")
	e.waitComposer(t, "holding the new message", 10*time.Second, func(n uiNode) bool { return n.Text == "again" })
	e.tap(t, e.waitNode(t, "Send", 5*time.Second, func(n uiNode) bool { return n.Desc == "Send" }))
	m = l.waitFor(t, bridge.TypeChatSend, 10*time.Second)
	first := payload[bridge.ChatSend](t, m)
	hellos := len(l.Hellos())
	l.cut()
	e.waitNode(t, "Retry", 20*time.Second, func(n uiNode) bool { return n.Text == "Retry" })
	e.waitNode(t, "Not sent", 5*time.Second, func(n uiNode) bool { return n.Text == "Not sent" })
	l.resume(t)
	l.waitHellos(t, hellos+1, 40*time.Second)
	l.drain()
	// Found again: the screen moves when the connection comes back.
	e.tap(t, e.waitNode(t, "Retry", 5*time.Second, func(n uiNode) bool { return n.Text == "Retry" }))
	m, ok = l.take(bridge.TypeChatSend, 10*time.Second)
	if !ok {
		t.Fatalf("no chat.send 10 s after Retry; on screen: %+v", e.screen(t))
	}
	again := payload[bridge.ChatSend](t, m)
	if again.ID != first.ID || again.Text != "again" {
		t.Errorf("Retry sent id %q text %q, want the same id %q and text", again.ID, again.Text, first.ID)
	}
	l.publishRID(t, m, bridge.TypeChatMessage, bridge.ChatMessage{ID: again.ID, Role: "user", Text: again.Text, Attachments: []bridge.Attachment{}})
	var shown int
	var last []uiNode
	if !poll(10*time.Second, func() bool {
		last, shown = e.screen(t), 0
		for _, n := range last {
			if n.Text == "Not sent" || n.Text == "Retry" {
				return false
			}
			if n.Text == "again" {
				shown++
			}
		}
		return shown == 1
	}) {
		t.Errorf("after Retry the chat shows the message %d times (want 1, without Not sent): %+v", shown, last)
	}
	if extra, ok := l.take(bridge.TypeChatSend, 3*time.Second); ok {
		t.Errorf("another chat.send after Retry: %s", extra.Payload)
	}
}

// answerCapture answers the app's next capture.request with a screenshot the
// host "took", with id (its bytes are never asked for here).
func (l *link) answerCapture(t *testing.T, id string) {
	t.Helper()
	req := l.waitFor(t, bridge.TypeCaptureReq, 10*time.Second)
	l.reply(t, req, bridge.TypeCaptureReady, bridge.CaptureReady{Attachment: bridge.Attachment{
		ID: id, Name: "screen-" + id[:4] + ".png", MIME: "image/png", Size: 1234,
		SHA256: strings.Repeat("ab", 32),
	}})
}

// Validation 4 of M3b: a screenshot from the chat's button and from the
// notification's action reaches the host as capture.request, and what the
// host answers comes back: the screenshot as an attachment of the next
// chat.send, or, with access cut, the host's own text on screen. A real cut
// leaves the emulator without adb (S7): the host's refusal is what is played.
func TestTheScreenshotGoesToTheHost(t *testing.T) {
	l := openLink(t)
	e := l.e
	l.openChat(t)
	attach := func() {
		e.tap(t, e.waitNode(t, "Attach", 10*time.Second, func(n uiNode) bool { return n.Desc == "Attach" }))
		e.tap(t, e.waitNode(t, "Screenshot", 10*time.Second, func(n uiNode) bool { return n.Text == "Screenshot" }))
	}

	// The button: the screenshot goes with the next message.
	const fromButton = "0123456789abcdef0123456789abcdef"
	l.drain()
	attach()
	l.answerCapture(t, fromButton)
	e.waitNode(t, "the screenshot in the draft", 10*time.Second, func(n uiNode) bool { return strings.Contains(n.Text+n.Desc, "screen-0123") })
	e.tap(t, e.waitNode(t, "Send", 5*time.Second, func(n uiNode) bool { return n.Desc == "Send" }))
	cs := payload[bridge.ChatSend](t, l.waitFor(t, bridge.TypeChatSend, 10*time.Second))
	if len(cs.Attachments) != 1 || cs.Attachments[0] != fromButton {
		t.Errorf("chat.send after the button's screenshot carries %q, want [%s]", cs.Attachments, fromButton)
	}

	// Access cut: the host refuses and says why.
	const why = "Access is cut from the phone: resume it to take a screenshot."
	attach()
	req := l.waitFor(t, bridge.TypeCaptureReq, 10*time.Second)
	l.reply(t, req, bridge.TypeError, bridge.ErrorReply{Code: bridge.CodeFailed, Text: why})
	e.waitNode(t, "the host's text", 10*time.Second, func(n uiNode) bool { return strings.Contains(n.Text, why) })

	// The notification's action sends the screenshot at once.
	const fromNotification = "fedcba9876543210fedcba9876543210"
	e.atCleanup(t, "shell", "cmd statusbar collapse")
	e.shell(t, "cmd statusbar expand-notifications")
	e.tap(t, e.waitNode(t, "Send screenshot", 10*time.Second, func(n uiNode) bool { return strings.EqualFold(n.Text, "Send screenshot") }))
	l.answerCapture(t, fromNotification)
	cs = payload[bridge.ChatSend](t, l.waitFor(t, bridge.TypeChatSend, 10*time.Second))
	if len(cs.Attachments) != 1 || cs.Attachments[0] != fromNotification {
		t.Errorf("chat.send after the notification's screenshot carries %q, want [%s]", cs.Attachments, fromNotification)
	}
}
