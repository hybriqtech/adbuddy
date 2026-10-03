package emutest

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// Validation 10 of M3b: the terminal works with no network, and nothing in it
// asks for one. The host is the harness of link_test.go: it answers the app's
// terminal.watch with a terminal.snapshot (sent as a stream message: with
// Reply the app ignores it and asks again, T1 §8) and sends terminal.output at
// consecutive offsets in the same epoch. What the page painted is read
// through CDP (cdp_test.go): uiautomator does not see xterm's rows.
//
// Phase 1, no network. wifi and data are turned off, and the test first
// checks that the network is off for real ("Active default network: none" and
// an nc that fails; ping is no proof: it fails with the network on too) and,
// as a control, that the same nc reached the PC before. Then the terminal
// paints the snapshot and the output, resize reaches the host and a quick key
// (Enter) arrives as terminal.input.
//
// Phase 2, nothing asks for a network.
//
//	(i)  every request of the WebView starts with TermWebPolicy.PREFIX: the
//	     resources the page loaded (Resource Timing), the requests of a
//	     reload (CDP Network) and, as a cheap extra signal, no "Refused to" or
//	     "net::ERR" line of the WebView's chromium log after the warm-up.
//	(ii) with the network ON, the bytes the app's uid moves in netstats (the
//	     history, which does not count the loopback the channel goes
//	     through) do not change, measured AFTER the WebView is warm.
//
// The WebView's first start in a fresh install moves ~7 KB rx / 2 KB tx
// outside the loopback with the app's uid, towards a Google server
// (cr_VariationsUtils: "Requesting new seed", T1 §7): it is the WebView
// component's own, not a request of the terminal's page, so the policy of the
// app never sees it. That is why (ii) is a delta between two readings taken
// once the counters stop moving, never an absolute zero (netstatsTolerance).
//
// The detectors are proven in every run by a control after the claims: an
// image outside the prefix injected through CDP must be seen by the request
// detector and by the log (it is blocked by the page's CSP, so it never
// reaches the network: T1 measured requestWillBeSent + loadingFailed
// blockedReason "csp"; fetch() does not show up in CDP, only in the log), and
// 300 000 bytes sent with the app's uid (run-as nc to the PC) must show in
// netstats. The mutations (ADBUDDY_EMUTEST_MUTATE, see files_test.go) do the
// same BEFORE the claims, and the test must go red:
//
//	inject-request  the image outside the prefix is injected before the check
//	                of (i): the test must fail on "outside TermWebPolicy.PREFIX"
//	app-traffic     the app's uid sends 300 000 bytes before the second reading
//	                of (ii): the test must fail on the netstats delta
func TestTerminalWorksOffline(t *testing.T) {
	mutate := mutation(t, "inject-request", "app-traffic")
	l := openLink(t)
	e := l.e
	sink := newSink(t)
	uid := e.appUID(t)

	// The control of the "no network" proof: with the network on, the same nc
	// reaches the sink. Without it, an nc that always fails would prove nothing.
	if code, out := e.nc(t, sink); code != 0 || !poll(5*time.Second, func() bool { return sink.conns.Load() == 1 }) {
		t.Fatalf("with the network on, nc to the PC fails (exit %d: %s; the sink got %d connections): the test cannot tell the network is off", code, out, sink.conns.Load())
	}

	// ---- Phase 1: no network.
	e.netCleanup(t)
	e.setNetwork(t, false)
	conns := sink.conns.Load()
	if code, out := e.nc(t, sink); code == 0 || sink.conns.Load() != conns {
		t.Fatalf("with wifi and data off nc still reached the PC (exit %d: %s): the network is not off", code, out)
	}
	t.Log("the network is off: Active default network none, nc to the PC fails")

	l.openChat(t)
	pid := e.pidOf(t)
	watch := payload[bridge.TerminalWatch](t, l.waitFor(t, bridge.TypeTerminalWatch, 60*time.Second))
	if watch.Epoch != "" || watch.Offset != 0 {
		t.Errorf("the first terminal.watch is %+v, want the empty epoch and offset 0 of an app with nothing", watch)
	}
	feed := &termFeed{l: l, epoch: "ep1"}
	feed.snapshot(t, 40, 10, "snapshot-line-1")

	c := e.openCDP(t)
	for _, m := range []string{"Network.enable", "Page.enable"} {
		if _, err := c.call(m, nil); err != nil {
			t.Fatal(err)
		}
	}
	if v, err := c.eval("location.href"); err != nil || string(v) != `"`+termPage+`"` {
		t.Fatalf("the WebView's page is %s (%v), want %s", v, err, termPage)
	}
	e.waitPainted(t, c, "snapshot-line-1")

	// terminal.resize: the app says the size that fits its view when the page loads.
	rs := payload[bridge.TerminalResize](t, l.waitFor(t, bridge.TypeTerminalResize, 20*time.Second))
	if rs.Cols < 10 || rs.Rows < 1 || rs.Cols > 1000 || rs.Rows > 1000 {
		t.Errorf("terminal.resize = %+v, want a size that fits a phone's view", rs)
	}

	if now := e.defaultNetwork(t); now != "none" {
		t.Fatalf("the network came back by itself during phase 1: Active default network %q", now)
	}
	feed.output(t, "offline-ok")
	e.waitPainted(t, c, "offline-ok")
	l.pressQuickKey(t, "Enter", "\r")

	// The offsets were consecutive and the epoch the snapshot's: the app has
	// not asked for the terminal again (it does when it sees a gap).
	if m, ok := l.take(bridge.TypeTerminalWatch, 3*time.Second); ok {
		t.Errorf("the app repeated terminal.watch after a valid snapshot and consecutive output: %s", m.Payload)
	}
	if now := e.pidOf(t); now != pid {
		t.Errorf("the app's process changed during phase 1: %s -> %s", pid, now)
	}

	// ---- Phase 2: nothing asks for a network.
	e.setNetwork(t, true)
	// (ii) warm-up: wait until the app's counters stop moving.
	e.waitNetstatsQuiet(t, uid)
	feed.output(t, "online-ok")
	e.waitPainted(t, c, "online-ok")
	l.pressQuickKey(t, "Esc", "\x1b")

	// (i) what the page loaded so far.
	res, err := c.resourceURLs()
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 {
		t.Fatal("Resource Timing lists no resource of the page: nothing to check (T1 measured six)")
	}
	if bad := outsidePrefix(res, termPrefix); len(bad) != 0 {
		t.Errorf("the page loaded resources outside TermWebPolicy.PREFIX: %v", bad)
	}

	e.shell(t, "logcat -c")
	before := e.readNetstats(t, uid)
	// A reload: every request of the page again, seen by CDP's Network.
	if _, err := c.call("Page.reload", nil); err != nil {
		t.Fatal(err)
	}
	if !c.waitEvent("Page.loadEventFired", 30*time.Second, nil) {
		t.Fatal("the page did not finish loading 30 s after Page.reload")
	}
	// The rest of the window: the WebView is left to itself.
	time.Sleep(15 * time.Second)

	const mutationURL = "https://example.com/adbuddy-mutation.png"
	switch mutate {
	case "inject-request":
		injectImage(t, c, mutationURL)
		c.waitRequest(t, mutationURL)
	case "app-traffic":
		e.appTraffic(t, sink, 300000)
	}
	after := e.readNetstats(t, uid)

	urls := c.requestURLs()
	t.Logf("requests seen by CDP: %d: %v", len(urls), urls)
	if !contains(urls, termPage) {
		t.Fatalf("CDP did not see the page's own request after Page.reload (%v): the detector sees nothing", urls)
	}
	if bad := outsidePrefix(urls, termPrefix); len(bad) != 0 {
		t.Errorf("the WebView asked for URLs outside TermWebPolicy.PREFIX: %v", bad)
	}
	if lines := chromiumRefusals(e.shell(t, "logcat -d -s chromium:V")); len(lines) != 0 {
		t.Errorf("the WebView's log has refused or failed requests after the warm-up: %q", lines)
	}
	dRx, dTx := after.Rx-before.Rx, after.Tx-before.Tx
	t.Logf("netstats of uid %d: rx %d -> %d, tx %d -> %d (blocks %d -> %d)", uid, before.Rx, after.Rx, before.Tx, after.Tx, before.Blocks, after.Blocks)
	if dRx > netstatsTolerance || dTx > netstatsTolerance || dRx < 0 || dTx < 0 {
		t.Errorf("the app's uid moved rx %+d, tx %+d bytes outside the loopback in the window after the warm-up (tolerance %d)", dRx, dTx, netstatsTolerance)
	}

	if mutate != "" {
		return
	}
	// The controls: the detectors do see what they are for.
	e.shell(t, "logcat -c")
	injectImage(t, c, mutationURL)
	c.waitRequest(t, mutationURL)
	if bad := outsidePrefix(c.requestURLs(), termPrefix); len(bad) != 1 || bad[0] != mutationURL {
		t.Errorf("control: the request detector sees %v outside the prefix, want exactly [%s]", bad, mutationURL)
	}
	if !poll(15*time.Second, func() bool {
		return len(chromiumRefusals(e.shell(t, "logcat -d -s chromium:V"))) != 0
	}) {
		t.Error("control: the image outside the prefix left no \"Refused to\" line in the WebView's log, so that signal sees nothing")
	}
	const sent = 300000
	e.appTraffic(t, sink, sent)
	if more := e.readNetstats(t, uid); more.Tx-after.Tx < sent {
		t.Errorf("control: the app's uid sent %d bytes and netstats moved tx by %d: the reading sees nothing", sent, more.Tx-after.Tx)
	}
}

// termFeed is the host's side of the terminal: one epoch and the next offset
// of its output.
type termFeed struct {
	l     *link
	epoch string
	off   int64
}

// snapshot answers a terminal.watch with a snapshot of cols x rows that clears
// the screen and writes line. It is a stream message: sent with Send.
func (f *termFeed) snapshot(t *testing.T, cols, rows int, line string) {
	t.Helper()
	f.l.send(t, bridge.TypeTerminalSnapshot, bridge.TerminalSnapshot{
		Epoch: f.epoch, Offset: f.off, Cols: cols, Rows: rows,
		Data: []byte("\x1b[2J\x1b[H" + line + "\r\n"),
	})
}

// output sends line as PTY output at the next offset, in the same epoch.
func (f *termFeed) output(t *testing.T, line string) {
	t.Helper()
	data := []byte(line + "\r\n")
	f.l.send(t, bridge.TypeTerminalOutput, bridge.TerminalOutput{Epoch: f.epoch, Offset: f.off, Data: data})
	f.off += int64(len(data))
}

// waitPainted waits up to 15 s for xterm to have painted a row that is want.
func (e *emu) waitPainted(t *testing.T, c *cdp, want string) {
	t.Helper()
	var rows []string
	var err error
	if !poll(15*time.Second, func() bool {
		rows, err = c.paintedRows()
		return err == nil && contains(rows, want)
	}) {
		t.Fatalf("xterm did not paint %q in 15 s: rows %q, %v", want, rows, err)
	}
}

// pressQuickKey taps the quick key with that text under the terminal and
// waits for the terminal.input it makes: exactly want. The tap is tried up to
// three times (the first one can be lost while the screen settles).
func (l *link) pressQuickKey(t *testing.T, label, want string) {
	t.Helper()
	e := l.e
	for {
		if _, ok := l.take(bridge.TypeTerminalInput, 0); !ok {
			break
		}
	}
	var got []string
	for try := 1; try <= 3; try++ {
		e.tap(t, e.waitNode(t, "quick key "+label, 15*time.Second, func(n uiNode) bool { return n.Text == label }))
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			m, ok := l.take(bridge.TypeTerminalInput, time.Until(deadline))
			if !ok {
				break
			}
			in := payload[bridge.TerminalInput](t, m)
			if string(in.Data) == want {
				return
			}
			got = append(got, fmt.Sprintf("%q", in.Data))
		}
	}
	t.Errorf("tapping %q did not send terminal.input %q in 3 tries; inputs seen: %v", label, want, got)
}

// waitNetstatsQuiet waits until the app's uid history does not change in
// three readings 5 s apart (up to 90 s) and returns it, or fails: a WebView
// that keeps talking would make the delta after it meaningless.
func (e *emu) waitNetstatsQuiet(t *testing.T, uid int) uidBytes {
	t.Helper()
	last := e.readNetstats(t, uid)
	still := 0
	for i := 0; i < 18; i++ {
		time.Sleep(5 * time.Second)
		now := e.readNetstats(t, uid)
		if now == last {
			if still++; still == 2 {
				t.Logf("warm-up done: uid %d has moved rx %d, tx %d bytes outside the loopback so far", uid, now.Rx, now.Tx)
				return now
			}
		} else {
			still = 0
		}
		last = now
	}
	t.Fatalf("the app's counters were still moving after 90 s (last %+v): the WebView is not quiet, so no delta can be measured", last)
	return last
}

// injectImage makes the page ask for an image at url, as a page that wanted to
// talk to the network would. The page's CSP blocks it before it goes out.
func injectImage(t *testing.T, c *cdp, url string) {
	t.Helper()
	if _, err := c.eval("new Image().src=" + fmt.Sprintf("%q", url) + "; 1"); err != nil {
		t.Fatal(err)
	}
}

// waitRequest waits up to 15 s for CDP's Network to report a request for url.
func (c *cdp) waitRequest(t *testing.T, url string) {
	t.Helper()
	if !poll(15*time.Second, func() bool { return contains(c.requestURLs(), url) }) {
		t.Fatalf("CDP's Network did not report a request for %s in 15 s: %v", url, c.requestURLs())
	}
}

// chromiumRefusals returns the lines of the WebView's log (tag chromium) that
// say a request was refused or failed: the CSP's "Refused to ..." (also for
// fetch, which CDP does not list; T1 §7) and Chromium's "net::ERR_...".
func chromiumRefusals(log string) []string {
	var out []string
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, "Refused to") || strings.Contains(line, "net::ERR") {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestChromiumRefusals(t *testing.T) {
	log := "10-03 12:00:00.000 1 1 I chromium: [INFO:CONSOLE(1)] \"Refused to load the image 'https://example.com/i.png' because it violates...\"\n" +
		"10-03 12:00:01.000 1 1 W chromium: net::ERR_NAME_NOT_RESOLVED\n" +
		"10-03 12:00:02.000 1 1 I chromium: all fine\n"
	if got := chromiumRefusals(log); len(got) != 2 {
		t.Errorf("chromiumRefusals = %q, want the two lines", got)
	}
	if got := chromiumRefusals("--------- beginning of main\n"); len(got) != 0 {
		t.Errorf("chromiumRefusals of an empty log = %q", got)
	}
}
