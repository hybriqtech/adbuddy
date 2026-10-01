package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hybriqtech/adbuddy/host/internal/agent"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// startChat starts a real serve, agent included, whose only agent is the
// fake one under the name "codex" (the configured default): the PATH of the
// process holds nothing else, so no real agent can start.
func startChat(t *testing.T, port string) *harness {
	t.Helper()
	fake, err := testutil.Build("github.com/hybriqtech/adbuddy/host/internal/testutil/fakeagent", t.TempDir(), "fakeagent")
	if err != nil {
		t.Fatal(err)
	}
	agentDir := t.TempDir()
	codex := filepath.Join(agentDir, testutil.Exe("codex"))
	bin, err := os.ReadFile(fake)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codex, bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// Every agent the host can start is the fake one or is not found.
	t.Setenv("PATH", agentDir)
	for _, name := range agent.Known {
		if p, err := exec.LookPath(name); err == nil && !strings.EqualFold(p, codex) {
			t.Fatalf("%s resolves to %s with the test PATH: a real agent would start", name, p)
		}
	}

	dataDir, err := os.MkdirTemp("", "e2d")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dataDir) })
	tmp := t.TempDir()
	h := &harness{t: t, data: dataDir, ws: filepath.Join(tmp, "ws"), logPath: filepath.Join(tmp, "adb.log"), serveErr: &syncBuffer{}, serveOut: &syncBuffer{}}
	script, err := testutil.WriteScript(tmp, append(phoneAppRules(port),
		testutil.FakeRule{Match: "^devices$", Stdout: "List of devices attached\ndev1\tdevice\n"},
		testutil.FakeRule{Match: "^shell getprop ro.product.model$", Stdout: "Pixel 9 Pro XL\n"},
		testutil.FakeRule{Match: "^shell settings get global window_animation_scale$", Stdout: "1.0\n"},
	)...)
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(tmp, "cfg"), 0o700)
	cfg := "agent = \"codex\"\nchat_port = " + port + "\n"
	if err := os.WriteFile(filepath.Join(tmp, "cfg", "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	// Without PATH of any case (Windows calls it Path), ADBUDDY_SOURCE and
	// ANDROID_SERIAL; PATH is the fake agent's folder alone.
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if !strings.EqualFold(k, "PATH") && !strings.EqualFold(k, "ADBUDDY_SOURCE") && !strings.EqualFold(k, "ANDROID_SERIAL") {
			env = append(env, kv)
		}
	}
	h.env = append(env, "PATH="+agentDir, "ADBUDDY_DATA_DIR="+dataDir, "ADBUDDY_CONFIG_DIR="+filepath.Join(tmp, "cfg"),
		"FAKEADB_SCRIPT="+script, "FAKEADB_LOG="+h.logPath, "LANG=en_US.UTF-8")

	cmd := exec.Command(adbuddyBin, "serve", "--adb", fakeADB, "--serial", "dev1", "--workspace", h.ws, "--confirm", "terminal")
	cmd.Env = h.env
	cmd.Stdout, cmd.Stderr = h.serveOut, h.serveErr
	if h.serveIn, err = cmd.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// The agent lives in serve's PTY: killing serve alone would leave it
		// running, and Windows cannot delete the folder of a running program.
		stop := exec.Command(adbuddyBin, "stop")
		stop.Env = h.env
		stop.Run()
		cmd.Process.Kill()
		cmd.Wait()
	})
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := ipc.PingDaemon(context.Background(), dataDir); err == nil {
			return h
		}
		if time.Now().After(deadline) {
			t.Fatalf("serve did not start:\n%s", h.serveErr.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// sendRID sends a request that carries a rid (phone.send does not).
func (p *phone) sendRID(typ, rid string, payload any) {
	p.t.Helper()
	raw, _ := json.Marshal(payload)
	data, _ := json.Marshal(bridge.Message{V: bridge.ProtocolVersion, Type: typ, RID: rid, TS: time.Now(), Payload: raw})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.c.Write(ctx, websocket.MessageText, data); err != nil {
		p.t.Fatal(err)
	}
}

// view records everything the fake app receives, so that a wait for one
// thing never loses what another wait needs.
type view struct {
	p      *phone
	screen string
	chats  []bridge.ChatMessage
	states []bridge.SessionState
	files  map[string][]byte
	ended  map[string]bool
	errors []string
}

func newView(p *phone) *view {
	return &view{p: p, files: map[string][]byte{}, ended: map[string]bool{}}
}

// wait reads messages until cond holds.
func (v *view) wait(what string, cond func() bool) {
	v.p.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for !cond() {
		typ, data, err := v.p.c.Read(ctx)
		if err != nil {
			v.p.t.Fatalf("waiting for %s: %v (screen %q)", what, err, v.screen)
		}
		if typ == websocket.MessageBinary {
			id, off, chunk, err := bridge.DecodeFrame(data)
			if err != nil || off != int64(len(v.files[id.String()])) {
				v.p.t.Fatalf("frame %v offset %d (have %d)", err, off, len(v.files[id.String()]))
			}
			v.files[id.String()] = append(v.files[id.String()], chunk...)
			continue
		}
		var m bridge.Message
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch m.Type {
		case bridge.TypeChatMessage:
			var cm bridge.ChatMessage
			json.Unmarshal(m.Payload, &cm)
			v.chats = append(v.chats, cm)
		case bridge.TypeSessionState:
			var st bridge.SessionState
			json.Unmarshal(m.Payload, &st)
			v.states = append(v.states, st)
		case bridge.TypeTerminalSnapshot:
			var ts bridge.TerminalSnapshot
			json.Unmarshal(m.Payload, &ts)
			v.screen = string(ts.Data)
		case bridge.TypeTerminalOutput:
			var to bridge.TerminalOutput
			json.Unmarshal(m.Payload, &to)
			v.screen += string(to.Data)
		case bridge.TypeFileEnd:
			var fe bridge.FileEnd
			json.Unmarshal(m.Payload, &fe)
			v.ended[fe.ID] = true
		case bridge.TypeError:
			v.errors = append(v.errors, string(m.Payload))
			v.p.t.Fatalf("error from the host while waiting for %s: %s", what, m.Payload)
		}
	}
}

func (v *view) chat(what string, ok func(bridge.ChatMessage) bool) bridge.ChatMessage {
	v.p.t.Helper()
	var found bridge.ChatMessage
	v.wait(what, func() bool {
		for _, m := range v.chats {
			if ok(m) {
				found = m
				return true
			}
		}
		return false
	})
	return found
}

func (v *view) onScreen(want string) {
	v.p.t.Helper()
	v.wait("screen "+want, func() bool { return strings.Contains(v.screen, want) })
}

func (v *view) state(what string, ok func(bridge.SessionState) bool) bridge.SessionState {
	v.p.t.Helper()
	// Only states that arrive from now on.
	v.states = nil
	var found bridge.SessionState
	v.wait(what, func() bool {
		for _, s := range v.states {
			if ok(s) {
				found = s
				return true
			}
		}
		return false
	})
	return found
}

// download asks for an attachment and returns its bytes.
func (v *view) download(a bridge.Attachment) []byte {
	v.p.t.Helper()
	v.p.sendRID(bridge.TypeFileGet, "dl-1", bridge.FileGet{ID: a.ID})
	v.wait("file.end", func() bool { return v.ended[a.ID] })
	return v.files[a.ID]
}

func TestChatEndToEndWithAFakeAgent(t *testing.T) {
	port := strconv.Itoa(freePort(t))
	h := startChat(t, port)
	p := connectPhone(t, h, port)

	// The fake agent runs in the host's PTY: the phone's terminal shows it.
	v := newView(p)
	p.sendRID(bridge.TypeTerminalWatch, "w-1", bridge.TerminalWatch{})
	v.onScreen("ready")

	// chat.send is typed as a paste; the app gets its message back, and the
	// agent's echo shows on the terminal.
	p.sendRID(bridge.TypeChatSend, "r-1", bridge.ChatSend{ID: "m-1", Text: "hola desde el movil", Attachments: []string{}})
	cm := v.chat("the chat.message", func(m bridge.ChatMessage) bool { return m.ID == "m-1" })
	if cm.Role != "user" || cm.Text != "hola desde el movil" || cm.Conversation == "" {
		t.Fatalf("chat.message %+v", cm)
	}
	v.onScreen("got:hola desde el movil")

	// A PC terminal attached by IPC sees the same screen.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var screen syncBuffer
	keysR, keysW, _ := os.Pipe()
	defer keysW.Close()
	defer keysR.Close()
	resizes := make(chan [2]int, 1)
	resizes <- [2]int{100, 30}
	attached := make(chan error, 1)
	go func() {
		_, err := ipc.Attach(ctx, h.data, ipc.Request{Kind: ipc.KindAttach}, keysR, &screen, resizes)
		attached <- err
	}()
	waitFor(t, func() bool { return strings.Contains(screen.String(), "got:hola desde el movil") }, h.serveErr)
	cancel()
	select {
	case <-attached:
	case <-time.After(10 * time.Second):
		t.Fatal("attach did not end")
	}

	// The agent hands a file to the phone with `adbuddy send`.
	note := filepath.Join(h.ws, "nota.txt")
	content := []byte("contenido del agente\n")
	if err := os.WriteFile(note, content, 0o600); err != nil {
		t.Fatal(err)
	}
	p.sendRID(bridge.TypeChatSend, "r-2", bridge.ChatSend{ID: "m-2", Text: "send " + note, Attachments: []string{}})
	sent := v.chat("the file message", func(m bridge.ChatMessage) bool { return m.Role == "assistant" && len(m.Attachments) == 1 })
	att := sent.Attachments[0]
	sum := sha256.Sum256(content)
	if att.Name != "nota.txt" || att.Size != int64(len(content)) || att.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("attachment %+v", att)
	}
	if got := v.download(att); !bytes.Equal(got, content) {
		t.Fatalf("downloaded %q", got)
	}
	v.onScreen("sent:ok")

	// adbuddy stop and adbuddy new change the state the app sees.
	if out, errOut, code := h.adbuddy("stop"); code != 0 {
		t.Fatalf("adbuddy stop: %d %q %q", code, out, errOut)
	}
	v.state("stopped", func(s bridge.SessionState) bool { return s.State == "stopped" })
	if out, errOut, code := h.adbuddy("new"); code != 0 {
		t.Fatalf("adbuddy new: %d %q %q", code, out, errOut)
	}
	st := v.state("a new session", func(s bridge.SessionState) bool { return s.State != "stopped" })
	if st.Agent != "codex" || st.Conversation == "" || st.Conversation == cm.Conversation {
		t.Fatalf("after new: %+v (first conversation %q)", st, cm.Conversation)
	}
}
