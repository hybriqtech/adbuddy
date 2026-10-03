package emutest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// The Chrome DevTools Protocol of the terminal's WebView. The debug build of
// the app opens @webview_devtools_remote_<pid> by itself (T1,
// .superpowers/sdd/2026-10-03-test-link-files-terminal/t1-mediciones.md §6);
// `adb forward tcp:0 localabstract:...` brings it to the PC, GET /json lists
// the page and its webSocketDebuggerUrl, and the page is read and watched
// through that WebSocket. xterm 5.5.0 draws in the DOM (no canvas), and
// uiautomator does not see its rows: CDP is the way to read what is painted.

// termPrefix is TermWebPolicy.PREFIX of the app: every request of the
// terminal's WebView must start with it. TestTermPrefixIsTheApps checks that
// this copy is the Kotlin one.
const termPrefix = "https://appassets.androidplatform.net/assets/term/"

// termPage is TermWebPolicy.PAGE.
const termPage = termPrefix + "index.html"

// cdpEvent is a CDP event (a message without id).
type cdpEvent struct {
	Method string
	Params json.RawMessage
}

// cdp is a CDP session on one page: calls with their answers, and the events
// kept, in order, for the test to look at.
type cdp struct {
	conn   *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	next    int
	pending map[int]chan cdpAnswer
	events  []cdpEvent
	wake    chan struct{} // closed and replaced on each event
	readErr error         // why the reader stopped
}

type cdpAnswer struct {
	Result json.RawMessage
	Err    string
}

// dialCDP opens the page's WebSocket. No Origin header is sent (the
// DevTools server refuses browsers' origins; T1 measured it works without).
func dialCDP(ctx context.Context, wsURL string) (*cdp, error) {
	ctx, cancel := context.WithCancel(ctx)
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("dial %s: %w", wsURL, err)
	}
	// A reply to Runtime.evaluate or an event can be larger than the 32 KiB default.
	conn.SetReadLimit(16 << 20)
	c := &cdp{conn: conn, ctx: ctx, cancel: cancel, pending: map[int]chan cdpAnswer{}, wake: make(chan struct{})}
	go c.read()
	return c, nil
}

func (c *cdp) read() {
	for {
		_, data, err := c.conn.Read(c.ctx)
		c.mu.Lock()
		if err != nil {
			c.readErr = err
			for id, ch := range c.pending {
				ch <- cdpAnswer{Err: "connection closed: " + err.Error()}
				delete(c.pending, id)
			}
			c.signalLocked()
			c.mu.Unlock()
			return
		}
		var m struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &m) != nil {
			// Not JSON: kept as an event the tests can see, never dropped silently.
			c.events = append(c.events, cdpEvent{Method: "<unparsable>", Params: append([]byte(nil), data...)})
		} else if m.ID != 0 {
			if ch, ok := c.pending[m.ID]; ok {
				a := cdpAnswer{Result: m.Result}
				if m.Error != nil {
					a.Err = fmt.Sprintf("%d %s", m.Error.Code, m.Error.Message)
				}
				ch <- a
				delete(c.pending, m.ID)
			}
		} else {
			c.events = append(c.events, cdpEvent{Method: m.Method, Params: m.Params})
		}
		c.signalLocked()
		c.mu.Unlock()
	}
}

func (c *cdp) signalLocked() {
	close(c.wake)
	c.wake = make(chan struct{})
}

// call sends a command and returns its result.
func (c *cdp) call(method string, params any) (json.RawMessage, error) {
	if params == nil {
		params = struct{}{}
	}
	ch := make(chan cdpAnswer, 1)
	c.mu.Lock()
	c.next++
	id := c.next
	c.pending[id] = ch
	c.mu.Unlock()
	msg, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	wctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	if err := c.conn.Write(wctx, websocket.MessageText, msg); err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	select {
	case a := <-ch:
		if a.Err != "" {
			return nil, fmt.Errorf("%s: %s", method, a.Err)
		}
		return a.Result, nil
	case <-wctx.Done():
		return nil, fmt.Errorf("%s: no answer in 30 s", method)
	}
}

// eval runs a JavaScript expression in the page and returns its value as a
// JSON string. A script exception is an error.
func (c *cdp) eval(expr string) (json.RawMessage, error) {
	raw, err := c.call("Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true})
	if err != nil {
		return nil, err
	}
	var r struct {
		Result struct {
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		Exception *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("Runtime.evaluate result %s: %w", raw, err)
	}
	if r.Exception != nil {
		return nil, fmt.Errorf("the script %q threw: %s (%s)", expr, r.Exception.Text, raw)
	}
	return r.Result.Value, nil
}

// evalStrings runs an expression that returns an array of strings.
func (c *cdp) evalStrings(expr string) ([]string, error) {
	v, err := c.eval("JSON.stringify(" + expr + ")")
	if err != nil {
		return nil, err
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return nil, fmt.Errorf("%q did not return a string: %s", expr, v)
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("%q did not return a JSON array of strings: %s", expr, s)
	}
	return out, nil
}

// paintedRows is what xterm has painted: the text of the non-empty rows of
// its DOM (.xterm-rows > div), with no-break spaces made plain (T1 §6: the
// 24 divs hold the grid plus padding, so the empty ones are left out).
func (c *cdp) paintedRows() ([]string, error) {
	rows, err := c.evalStrings(`[...document.querySelectorAll('.xterm-rows > div')].map(d=>d.textContent).filter(s=>s.trim())`)
	for i := range rows {
		rows[i] = strings.TrimSpace(strings.ReplaceAll(rows[i], " ", " "))
	}
	return rows, err
}

// resourceURLs are the URLs of the resources the page loaded (the Resource
// Timing entries, which include the page's own scripts and styles; T1 §7: six).
func (c *cdp) resourceURLs() ([]string, error) {
	return c.evalStrings(`performance.getEntriesByType('resource').map(e=>e.name)`)
}

// requestURLs are the URLs of the Network.requestWillBeSent events so far
// (needs Network.enable), oldest first.
func (c *cdp) requestURLs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var urls []string
	for _, ev := range c.events {
		if ev.Method != "Network.requestWillBeSent" {
			continue
		}
		var p struct {
			Request struct {
				URL string `json:"url"`
			} `json:"request"`
		}
		if json.Unmarshal(ev.Params, &p) == nil && p.Request.URL != "" {
			urls = append(urls, p.Request.URL)
		}
	}
	return urls
}

// outsidePrefix returns the URLs that do not start with prefix. A data: or
// blob: URL counts as outside too: the page must ask for nothing else.
func outsidePrefix(urls []string, prefix string) []string {
	var bad []string
	for _, u := range urls {
		if !strings.HasPrefix(u, prefix) {
			bad = append(bad, u)
		}
	}
	return bad
}

// waitEvent waits up to d for an event with method that ok accepts (nil: any).
func (c *cdp) waitEvent(method string, d time.Duration, ok func(json.RawMessage) bool) bool {
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	seen := 0
	for {
		c.mu.Lock()
		for ; seen < len(c.events); seen++ {
			ev := c.events[seen]
			if ev.Method == method && (ok == nil || ok(ev.Params)) {
				c.mu.Unlock()
				return true
			}
		}
		wake, rerr := c.wake, c.readErr
		c.mu.Unlock()
		if rerr != nil {
			return false
		}
		select {
		case <-wake:
		case <-deadline.C:
			return false
		}
	}
}

func (c *cdp) close() {
	_ = c.conn.Close(websocket.StatusNormalClosure, "")
	c.cancel()
}

// cdpPage is one entry of GET /json.
type cdpPage struct {
	Type string `json:"type"`
	URL  string `json:"url"`
	WS   string `json:"webSocketDebuggerUrl"`
}

// pickTermPage returns the debugger URL of the terminal's page in the answer
// of GET /json, or an error that lists what there is.
func pickTermPage(body []byte) (string, error) {
	var pages []cdpPage
	if err := json.Unmarshal(body, &pages); err != nil {
		return "", fmt.Errorf("GET /json: %w: %s", err, body)
	}
	for _, p := range pages {
		if p.Type == "page" && p.URL == termPage {
			if p.WS == "" {
				return "", fmt.Errorf("the terminal's page has no webSocketDebuggerUrl: %s", body)
			}
			return p.WS, nil
		}
	}
	return "", fmt.Errorf("no page %s in GET /json: %s", termPage, body)
}

// openCDP reaches the terminal's page through the WebView's debugging socket
// and returns a CDP session on it, which is closed at cleanup, as is the
// forward (adb forward --remove-all removes the forwards of this adb server,
// the emulator's own: openLink only removes the reverses).
func (e *emu) openCDP(t *testing.T) *cdp {
	t.Helper()
	sock := "webview_devtools_remote_" + e.pidOf(t)
	var unix string
	if !poll(30*time.Second, func() bool {
		unix = e.shell(t, "cat /proc/net/unix | grep -a devtools; true")
		return strings.Contains(unix, "@"+sock)
	}) {
		t.Fatalf("no @%s in /proc/net/unix after 30 s (the debug build opens it by itself; is the terminal on screen?): %q", sock, unix)
	}
	e.atCleanup(t, "forward", "--remove-all")
	out := e.run(t, "forward", "tcp:0", "localabstract:"+sock)
	port, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || port <= 0 {
		t.Fatalf("adb forward tcp:0 printed %q, want the local port", out)
	}
	var body []byte
	var ws string
	if !poll(30*time.Second, func() bool {
		req, _ := http.NewRequestWithContext(e.ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+"/json", nil)
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			body = []byte(err.Error())
			return false
		}
		defer resp.Body.Close()
		body, _ = io.ReadAll(resp.Body)
		ws, err = pickTermPage(body)
		return err == nil
	}) {
		t.Fatalf("the terminal's page is not in GET /json in 30 s: %s", body)
	}
	c, err := dialCDP(e.ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.close)
	return c
}

// Reads the Kotlin source: the prefix this copy checks is the app's.
func TestTermPrefixIsTheApps(t *testing.T) {
	src, err := os.ReadFile("../../../android/app/src/main/kotlin/tech/hybriq/adbuddy/term/TermWebPolicy.kt")
	if err != nil {
		t.Fatal(err)
	}
	origin := regexp.MustCompile(`const val ORIGIN = "([^"]+)"`).FindSubmatch(src)
	prefix := regexp.MustCompile(`const val PREFIX = "\$ORIGIN([^"]+)"`).FindSubmatch(src)
	page := regexp.MustCompile(`const val PAGE = PREFIX \+ "([^"]+)"`).FindSubmatch(src)
	if origin == nil || prefix == nil || page == nil {
		t.Fatalf("TermWebPolicy.kt no longer declares ORIGIN, PREFIX and PAGE as this test reads them")
	}
	if got := string(origin[1]) + string(prefix[1]); got != termPrefix {
		t.Errorf("TermWebPolicy.PREFIX = %q, the tests have %q", got, termPrefix)
	}
	if got := termPrefix + string(page[1]); got != termPage {
		t.Errorf("TermWebPolicy.PAGE = %q, the tests have %q", got, termPage)
	}
}

func TestOutsidePrefix(t *testing.T) {
	urls := []string{termPage, termPrefix + "term.js", "https://example.com/i.png", "data:text/plain,x", termPrefix[:len(termPrefix)-1] + "x/a.js"}
	got := outsidePrefix(urls, termPrefix)
	want := []string{"https://example.com/i.png", "data:text/plain,x", termPrefix[:len(termPrefix)-1] + "x/a.js"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("outsidePrefix = %v, want %v", got, want)
	}
	if len(outsidePrefix([]string{termPage}, termPrefix)) != 0 {
		t.Error("the page itself counted as outside")
	}
}

func TestPickTermPage(t *testing.T) {
	good := `[{"type":"page","url":"` + termPage + `","webSocketDebuggerUrl":"ws://127.0.0.1:1/devtools/page/X"}]`
	if ws, err := pickTermPage([]byte(good)); err != nil || ws != "ws://127.0.0.1:1/devtools/page/X" {
		t.Errorf("pickTermPage = %q, %v", ws, err)
	}
	for _, bad := range []string{`[]`, `not json`, `[{"type":"page","url":"about:blank","webSocketDebuggerUrl":"ws://x"}]`,
		`[{"type":"page","url":"` + termPage + `"}]`} {
		if _, err := pickTermPage([]byte(bad)); err == nil {
			t.Errorf("pickTermPage(%s) accepted it", bad)
		}
	}
}

// The CDP client against a fake DevTools server: a call gets its answer, the
// events are kept in order, a script exception is an error and the painted
// rows are read as the DOM expression asks. (What the real WebView answers is
// for the emulator: see terminal_test.go.)
func TestCDPClient(t *testing.T) {
	srv := newFakeCDP(t, func(method string, params map[string]any) (any, []map[string]any) {
		switch method {
		case "Network.enable":
			return map[string]any{}, nil
		case "Runtime.evaluate":
			expr, _ := params["expression"].(string)
			switch {
			case strings.Contains(expr, "xterm-rows"):
				return map[string]any{"result": map[string]any{"type": "string", "value": `["  hello-from-host  ","a b"]`}}, nil
			case strings.Contains(expr, "boom"):
				return map[string]any{"result": map[string]any{"type": "object"}, "exceptionDetails": map[string]any{"text": "Uncaught"}}, nil
			default: // a script that makes the page ask for an image
				return map[string]any{"result": map[string]any{"type": "undefined"}}, []map[string]any{
					{"method": "Network.requestWillBeSent", "params": map[string]any{"request": map[string]any{"url": "https://example.com/i.png"}}},
					{"method": "Network.requestWillBeSent", "params": map[string]any{"request": map[string]any{"url": termPage}}},
				}
			}
		}
		return nil, nil
	})
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	c, err := dialCDP(ctx, srv)
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	if _, err := c.call("Network.enable", nil); err != nil {
		t.Fatal(err)
	}
	rows, err := c.paintedRows()
	if err != nil || len(rows) != 2 || rows[0] != "hello-from-host" || rows[1] != "a b" {
		t.Errorf("paintedRows = %q, %v", rows, err)
	}
	if _, err := c.eval("boom()"); err == nil {
		t.Error("a script exception was not an error")
	}
	if _, err := c.eval("new Image().src='https://example.com/i.png'"); err != nil {
		t.Fatal(err)
	}
	if !c.waitEvent("Network.requestWillBeSent", 5*time.Second, func(p json.RawMessage) bool {
		return strings.Contains(string(p), "example.com")
	}) {
		t.Fatal("the request event did not arrive")
	}
	urls := c.requestURLs()
	if len(urls) != 2 || urls[0] != "https://example.com/i.png" || urls[1] != termPage {
		t.Errorf("requestURLs = %v", urls)
	}
	if bad := outsidePrefix(urls, termPrefix); len(bad) != 1 || bad[0] != "https://example.com/i.png" {
		t.Errorf("outsidePrefix = %v", bad)
	}
	if c.waitEvent("Page.loadEventFired", 200*time.Millisecond, nil) {
		t.Error("an event that never came was reported")
	}
}

// newFakeCDP serves a DevTools WebSocket: for each command it calls handle,
// answers with the result it returns and then sends the events it returns.
// It returns the ws:// URL.
func newFakeCDP(t *testing.T, handle func(method string, params map[string]any) (any, []map[string]any)) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var m struct {
				ID     int            `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if json.Unmarshal(data, &m) != nil {
				return
			}
			result, events := handle(m.Method, m.Params)
			out, _ := json.Marshal(map[string]any{"id": m.ID, "result": result})
			if conn.Write(r.Context(), websocket.MessageText, out) != nil {
				return
			}
			for _, ev := range events {
				out, _ := json.Marshal(ev)
				if conn.Write(r.Context(), websocket.MessageText, out) != nil {
					return
				}
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}
