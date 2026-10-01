package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
)

// fakeUIServer records what is sent; current says whether a message comes
// from the current connection.
type fakeUIServer struct {
	mu      sync.Mutex
	sent    []bridge.UIRequest
	err     error
	current bool
}

func (f *fakeUIServer) Send(typ string, payload any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	if typ != bridge.TypeUIRequest {
		panic(typ)
	}
	f.sent = append(f.sent, payload.(bridge.UIRequest))
	return nil
}

func (f *fakeUIServer) Current(bridge.Message) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current
}

// sent waits for the n-th request sent and returns it.
func (f *fakeUIServer) sentNth(t *testing.T, n int) bridge.UIRequest {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		if len(f.sent) >= n {
			r := f.sent[n-1]
			f.mu.Unlock()
			return r
		}
		f.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("request %d not sent", n)
	return bridge.UIRequest{}
}

func uiMessage(t *testing.T, typ string, payload any) bridge.Message {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return bridge.Message{V: bridge.ProtocolVersion, Type: typ, Payload: raw}
}

type callResult struct {
	res json.RawMessage
	err error
}

func call(u *uiLink, op string) chan callResult {
	ch := make(chan callResult, 1)
	go func() {
		res, err := u.Call(context.Background(), op, json.RawMessage("{}"))
		ch <- callResult{res, err}
	}()
	return ch
}

func TestUILinkAnswers(t *testing.T) {
	u := newUILink("", &bytes.Buffer{})
	srv := &fakeUIServer{current: true}
	u.setServer(srv)
	h := u.area().handlers[bridge.TypeUIResponse]

	ch := call(u, bridge.UIOpDump)
	req := srv.sentNth(t, 1)
	if req.Op != bridge.UIOpDump || !strings.HasPrefix(req.ID, "u_"+u.nonce+"_") || len(u.nonce) != 8 || time.Until(req.ExpiresAt) <= 0 {
		t.Fatalf("request %+v", req)
	}
	// An answer for another id is dropped; this one goes through.
	h(uiMessage(t, bridge.TypeUIResponse, bridge.UIResponse{ID: "u_999", OK: true, Result: json.RawMessage(`{"x":1}`)}))
	h(uiMessage(t, bridge.TypeUIResponse, bridge.UIResponse{ID: req.ID, OK: true, Result: json.RawMessage(`{"locked":false}`)}))
	if r := <-ch; r.err != nil || string(r.res) != `{"locked":false}` {
		t.Fatalf("%s %v", r.res, r.err)
	}

	ch = call(u, bridge.UIOpClick)
	req = srv.sentNth(t, 2)
	h(uiMessage(t, bridge.TypeUIResponse, bridge.UIResponse{ID: req.ID, Error: &bridge.ErrorReply{Code: bridge.CodeNotFound, Text: "No element matches."}}))
	var ue *daemon.UIError
	if r := <-ch; !errors.As(r.err, &ue) || ue.Code != bridge.CodeNotFound {
		t.Fatalf("%v", r.err)
	}
}

func TestUILinkWithoutAnApp(t *testing.T) {
	u := newUILink("", &bytes.Buffer{})
	if _, err := u.Call(context.Background(), bridge.UIOpDump, nil); !errors.Is(err, daemon.ErrNoApp) {
		t.Fatalf("no server: %v", err)
	}
	u.setServer(&fakeUIServer{err: bridge.ErrNoApp})
	if _, err := u.Call(context.Background(), bridge.UIOpDump, nil); !errors.Is(err, daemon.ErrNoApp) {
		t.Fatalf("no app: %v", err)
	}
}

// After the request went out, a lost connection or no answer is uncertain:
// a timeout, never "no app" (which would let adb tap again).
func TestUILinkLostAndTimedOut(t *testing.T) {
	setVar(t, &uiWait, 200*time.Millisecond)
	u := newUILink("", &bytes.Buffer{})
	srv := &fakeUIServer{current: true}
	u.setServer(srv)

	ch := call(u, bridge.UIOpClick)
	srv.sentNth(t, 1)
	u.area().attach()
	if r := <-ch; !errors.Is(r.err, daemon.ErrUITimeout) {
		t.Fatalf("replaced: %v", r.err)
	}
	ch = call(u, bridge.UIOpClick)
	if r := <-ch; !errors.Is(r.err, daemon.ErrUITimeout) {
		t.Fatalf("no answer: %v", r.err)
	}
}

// Two links (two serves in a row) never use the same ids.
func TestUILinkIDsDifferAcrossServes(t *testing.T) {
	if a, b := newUILink("", &bytes.Buffer{}), newUILink("", &bytes.Buffer{}); a.nonce == b.nonce {
		t.Fatalf("same nonce %q", a.nonce)
	}
}

// A list from a connection already replaced does not count.
func TestUILinkDropsListsOfAnotherConnection(t *testing.T) {
	u := newUILink("", &bytes.Buffer{})
	u.setServer(&fakeUIServer{current: false})
	u.area().handlers[bridge.TypeDeviceInfo](uiMessage(t, bridge.TypeDeviceInfo, bridge.DeviceInfo{ProtectedApps: []string{"com.example.bank"}}))
	if u.Protected("com.example.bank") {
		t.Fatal("taken from a replaced connection")
	}
}

// An answer from a connection already replaced does not count.
func TestUILinkDropsAnswersOfAnotherConnection(t *testing.T) {
	setVar(t, &uiWait, 300*time.Millisecond)
	u := newUILink("", &bytes.Buffer{})
	srv := &fakeUIServer{current: false}
	u.setServer(srv)
	ch := call(u, bridge.UIOpDump)
	req := srv.sentNth(t, 1)
	u.area().handlers[bridge.TypeUIResponse](uiMessage(t, bridge.TypeUIResponse, bridge.UIResponse{ID: req.ID, OK: true, Result: json.RawMessage(`{}`)}))
	if r := <-ch; !errors.Is(r.err, daemon.ErrUITimeout) {
		t.Fatalf("%s %v", r.res, r.err)
	}
}

func TestUILinkProtectedApps(t *testing.T) {
	file := filepath.Join(t.TempDir(), "data", "protected.json")
	var log syncBuffer
	u := newUILink(file, &log)
	u.setServer(&fakeUIServer{current: true})
	if u.Protected("com.example.bank") || !strings.Contains(log.String(), "only ADBuddy itself is protected") {
		t.Fatalf("no list: %q", log.String())
	}
	u.Protected("x.y")
	if strings.Count(log.String(), "only ADBuddy") != 1 {
		t.Fatalf("said more than once: %q", log.String())
	}
	h := u.area().handlers[bridge.TypeDeviceInfo]
	h(uiMessage(t, bridge.TypeDeviceInfo, bridge.DeviceInfo{ProtectedApps: []string{"com.example.bank"}}))
	if !u.Protected("com.example.bank") || u.Protected("com.android.settings") {
		t.Fatal("list not taken")
	}
	// Absent: no change. Not valid: refused whole.
	h(uiMessage(t, bridge.TypeDeviceInfo, bridge.DeviceInfo{TailscaleIP: "192.0.2.7"}))
	h(uiMessage(t, bridge.TypeDeviceInfo, bridge.DeviceInfo{ProtectedApps: []string{"com.other.bank", "bad name\x1b"}}))
	if !u.Protected("com.example.bank") || u.Protected("com.other.bank") || !strings.Contains(log.String(), "not valid") || strings.Contains(log.String(), "\x1b") {
		t.Fatalf("kept list wrong; log %q", log.String())
	}
	// Saved: a new session starts with it.
	again := newUILink(file, &log)
	if !again.Protected("com.example.bank") {
		t.Fatal("not saved")
	}
	// An empty list is a list: nothing protected but ADBuddy.
	// (Go leaves an empty list out: written as the app sends it.)
	h(bridge.Message{V: bridge.ProtocolVersion, Type: bridge.TypeDeviceInfo, Payload: json.RawMessage(`{"protectedApps":[]}`)})
	if u.Protected("com.example.bank") || newUILink(file, &log).Protected("com.example.bank") {
		t.Fatal("the empty list did not replace the old one")
	}
	// A damaged file protects nothing, and says so.
	if err := os.WriteFile(file, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if newUILink(file, &log).Protected("com.example.bank") || !strings.Contains(log.String(), "unreadable") {
		t.Fatalf("damaged file: %q", log.String())
	}
}
