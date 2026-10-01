package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp/apktest"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// freePort returns a TCP port that was free a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// phone is a fake phone app speaking the bridge protocol.
type phone struct {
	t *testing.T
	c *websocket.Conn
}

func (p *phone) send(typ string, payload any) {
	p.t.Helper()
	raw, _ := json.Marshal(payload)
	data, _ := json.Marshal(bridge.Message{V: bridge.ProtocolVersion, Type: typ, TS: time.Now(), Payload: raw})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.c.Write(ctx, websocket.MessageText, data); err != nil {
		p.t.Fatal(err)
	}
}

// until reads messages until one of type typ arrives.
func (p *phone) until(typ string) bridge.Message {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		_, data, err := p.c.Read(ctx)
		if err != nil {
			p.t.Fatalf("waiting for %s: %v", typ, err)
		}
		var m bridge.Message
		json.Unmarshal(data, &m)
		if m.Type == typ {
			return m
		}
	}
}

// finalEntry reads journal entries until the final one of the question id
// (not "running") and returns its text fields. The app takes the question
// off the screen when an entry with its id arrives: the keys are checked as
// written ("id", "status"), not as a case-insensitive decoder reads them.
func (p *phone) finalEntry(id string) map[string]string {
	p.t.Helper()
	for {
		m := p.until(bridge.TypeJournalEntry)
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(m.Payload, &raw); err != nil {
			p.t.Fatal(err)
		}
		fields := map[string]string{}
		for k, v := range raw {
			var s string
			if json.Unmarshal(v, &s) == nil {
				fields[k] = s
			}
		}
		if fields["id"] != id {
			continue
		}
		if _, ok := raw["status"]; !ok {
			p.t.Fatalf("entry without \"status\": %s", m.Payload)
		}
		if fields["status"] != "running" {
			return fields
		}
	}
}

// e2eAPK is the app the fake phone has installed; each test trusts it as
// the development build (the binary pins no release certificate).
var (
	e2eCert = apktest.Cert("e2e")
	e2eAPK  = apktest.APK(apktest.Block{ID: apktest.V2, Signers: []apktest.Signer{{Cert: e2eCert}}})
)

// trustE2EApp makes the host in dataDir trust e2eAPK.
func trustE2EApp(t *testing.T, dataDir string) {
	t.Helper()
	sum := sha256.Sum256(e2eAPK)
	if _, err := phoneapp.TrustDevAPK(dataDir, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
}

// phoneAppRules make the fake adb answer like a phone with the app installed
// that takes the handshake: ping returns the nonce of the last handshake.
func phoneAppRules(port string) []testutil.FakeRule {
	return []testutil.FakeRule{
		{Match: `^shell pm path tech\.hybriq\.adbuddy$`, Stdout: "package:/data/app/~~E2e==/tech.hybriq.adbuddy-E2e==/base.apk\n"},
		{Match: `^shell stat -c `, Stdout: strconv.Itoa(len(e2eAPK)) + " 1 1\n"},
		{Match: `^shell base64 `, Stdout: base64.StdEncoding.EncodeToString(e2eAPK) + "\n"},
		{Match: `^shell dumpsys package tech\.hybriq\.adbuddy$`, Stdout: "Packages:\n  Package [tech.hybriq.adbuddy] (1a2b):\n    versionCode=1 minSdk=30 targetSdk=37\n    versionName=0.2.0\n"},
		{Match: `^reverse tcp:` + port + ` tcp:` + port + `$`},
		{Match: `^reverse --list$`, Stdout: "dev1 tcp:" + port + " tcp:" + port + "\n"},
		{Match: `^shell content write --uri content://tech\.hybriq\.adbuddy\.bridge/handshake$`, EchoStdin: true, SaveStdin: "handshake"},
		{Match: `^shell content call --uri content://tech\.hybriq\.adbuddy\.bridge --method ping$`, Stdout: "Result: Bundle[{ok=true, port=" + port + ", nonce={{handshake.nonce}}}]\n"},
	}
}

// connectPhone waits for the handshake, checks that the token never went in
// a command line and connects a fake app with it.
func connectPhone(t *testing.T, h *harness, port string) *phone {
	t.Helper()
	// The token reached the app on stdin, never in a command line.
	var token string
	waitFor(t, func() bool {
		calls, _ := testutil.ReadLog(h.logPath)
		for _, c := range calls {
			if strings.Contains(c.Joined(), "content write") {
				var hs struct {
					Token string
					Port  int
				}
				json.Unmarshal([]byte(c.Stdin), &hs)
				token = hs.Token
			}
		}
		return token != ""
	}, h.serveErr)
	for _, c := range h.adbCalls() {
		if strings.Contains(c, token) {
			t.Fatalf("the token is in a command line: %q", c)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws://127.0.0.1:"+port+"/v1", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	p := &phone{t: t, c: c}
	p.send(bridge.TypeHello, bridge.Hello{Protocol: bridge.ProtocolVersion, Token: token, AppVersion: "0.2.0"})
	p.until(bridge.TypeWelcome)
	waitFor(t, func() bool { return strings.Contains(h.serveErr.String(), "phone app is connected") }, h.serveErr)
	return p
}

func TestPhoneAppConfirmsAndPauses(t *testing.T) {
	port := strconv.Itoa(freePort(t))
	h := startWithData(t, "chat_port = "+port+"\n", func(data string) { trustE2EApp(t, data) },
		append(phoneAppRules(port), testutil.FakeRule{Match: `^shell reboot$`})...)
	p := connectPhone(t, h, port)

	// A 🔴 command asks on the phone; allowing it runs it.
	done := make(chan int)
	go func() {
		_, _, code := h.adb("", "shell", "reboot")
		done <- code
	}()
	m := p.until(bridge.TypeConfirmReq)
	var req bridge.ConfirmRequest
	json.Unmarshal(m.Payload, &req)
	if req.RawCommand != "adb shell reboot" || !req.Special || req.AllowSession {
		t.Fatalf("request %+v", req)
	}
	p.send(bridge.TypeConfirmReply, bridge.ConfirmReply{ID: req.ID, Decision: "allow"})
	if code := <-done; code != 0 {
		t.Fatalf("allowed command exit %d", code)
	}
	if e := p.finalEntry(req.ID); e["status"] != "ok" {
		t.Fatalf("final entry %v", e)
	}
	if strings.Contains(h.serveErr.String(), "Code: ") {
		t.Fatal("the terminal asked too")
	}

	// Pause refuses everything until resume.
	p.send(bridge.TypePause, struct{}{})
	p.until(bridge.TypeSessionState)
	if _, errOut, code := h.adb("", "shell", "getprop", "ro.product.model"); code != 126 || !strings.Contains(errOut, "paused") {
		t.Fatalf("paused: %d %q", code, errOut)
	}
	p.send(bridge.TypeResume, struct{}{})
	p.until(bridge.TypeSessionState)
	if out, _, code := h.adb("", "shell", "getprop", "ro.product.model"); code != 0 || out != "Pixel 9 Pro XL\n" {
		t.Fatalf("resumed: %d %q", code, out)
	}
}

// redOnThePhone starts `adb shell reboot` from the agent and waits for its
// question on the phone.
func redOnThePhone(t *testing.T, h *harness, p *phone) (chan [2]string, bridge.ConfirmRequest) {
	t.Helper()
	done := make(chan [2]string, 1)
	go func() {
		_, errOut, code := h.adb("", "shell", "reboot")
		done <- [2]string{strconv.Itoa(code), errOut}
	}()
	var req bridge.ConfirmRequest
	json.Unmarshal(p.until(bridge.TypeConfirmReq).Payload, &req)
	return done, req
}

func waitDone(t *testing.T, done chan [2]string, h *harness) (code, errOut string) {
	t.Helper()
	select {
	case r := <-done:
		return r[0], r[1]
	case <-time.After(15 * time.Second):
		t.Fatalf("the 🔴 kept waiting; serve stderr:\n%s", h.serveErr.String())
	}
	return "", ""
}

// T1 review, M7: the app disconnects while its question waits; the question
// moves to the terminal at once, and is answered there.
func TestPhoneDisconnectMovesTheQuestionToTheTerminal(t *testing.T) {
	port := strconv.Itoa(freePort(t))
	h := startWithData(t, "chat_port = "+port+"\n", func(data string) { trustE2EApp(t, data) }, phoneAppRules(port)...)
	p := connectPhone(t, h, port)
	done, _ := redOnThePhone(t, h, p)
	p.c.CloseNow()
	waitFor(t, func() bool { return strings.Contains(h.serveErr.String(), "Code: ") }, h.serveErr)
	if !strings.Contains(h.serveErr.String(), "disconnected before you answered “Restart the phone”") {
		t.Fatalf("the terminal was not told why:\n%s", h.serveErr.String())
	}
	time.Sleep(1200 * time.Millisecond) // answers in the first second are ignored as typed ahead
	io.WriteString(h.serveIn, "n\n")
	if code, errOut := waitDone(t, done, h); code != "126" || !strings.Contains(errOut, "Denied by user: Restart the phone") {
		t.Fatalf("%s %q", code, errOut)
	}
}

// T2 review and controller finding: while the question waits on the phone
// every other command is refused; pausing withdraws the question at once and
// the command ends blocked as paused.
func TestPhonePauseWithdrawsTheQuestion(t *testing.T) {
	port := strconv.Itoa(freePort(t))
	h := startWithData(t, "chat_port = "+port+"\n", func(data string) { trustE2EApp(t, data) }, phoneAppRules(port)...)
	p := connectPhone(t, h, port)
	done, req := redOnThePhone(t, h, p)
	if _, errOut, code := h.adb("", "shell", "input", "tap", "1", "1"); code != 126 || strings.TrimSpace(errOut) != `Blocked by ADBuddy: waiting for your answer to "Restart the phone"` {
		t.Fatalf("tap while the question waits: %d %q", code, errOut)
	}
	p.send(bridge.TypePause, struct{}{})
	if code, errOut := waitDone(t, done, h); code != "126" || !strings.Contains(errOut, "Blocked by ADBuddy: paused from the phone") {
		t.Fatalf("%s %q", code, errOut)
	}
	if e := p.finalEntry(req.ID); e["status"] != "blocked" || !strings.Contains(e["reason"], "paused") {
		t.Fatalf("journal %v", e)
	}
	for _, c := range h.adbCalls() {
		if strings.Contains(c, "reboot") || strings.Contains(c, "input") {
			t.Fatalf("%q reached adb", c)
		}
	}
	if strings.Contains(h.serveErr.String(), "Code: ") {
		t.Fatal("the terminal asked")
	}
}

// Minor 7: the app disconnects while the agent is paused: the terminal says
// it is still paused and how to resume.
func TestPhoneDisconnectWhilePaused(t *testing.T) {
	port := strconv.Itoa(freePort(t))
	h := startWithData(t, "chat_port = "+port+"\n", func(data string) { trustE2EApp(t, data) }, phoneAppRules(port)...)
	p := connectPhone(t, h, port)
	p.send(bridge.TypePause, struct{}{})
	p.until(bridge.TypeSessionState)
	p.c.CloseNow()
	waitFor(t, func() bool {
		return strings.Contains(h.serveErr.String(), "Still paused: resume from the app when it reconnects, or restart adbuddy serve.")
	}, h.serveErr)
}
