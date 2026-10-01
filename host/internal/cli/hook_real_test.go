package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/agent"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
)

// The real thing (spec M4 §10): a real Claude Code, in a throwaway
// environment, runs the hooks workspace.Init writes, with the real
// `adbuddy hook` binary, against the real adapter and a phone in memory.
// It spends a few cents of Claude Code, so it only runs with
// ADBUDDY_REAL_CLAUDE=1. It never touches a phone, ~/.claude/settings.json
// nor ~/.claude.json: Claude Code gets a configuration directory of its own,
// with a copy of the login (in the temp dir, gone with the test).
//
//	ADBUDDY_REAL_CLAUDE=1 go test ./internal/cli -run TestRealClaudeCode -v -timeout 600s

type realRun struct {
	t  *testing.T
	ws string
	r  *chatRig

	mu       sync.Mutex
	decision string // what the phone answers to a permission request
	msgs     []bridge.Message
}

func newRealRun(t *testing.T) *realRun {
	t.Helper()
	if os.Getenv("ADBUDDY_REAL_CLAUDE") != "1" {
		t.Skip("set ADBUDDY_REAL_CLAUDE=1 to run a real Claude Code")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude is not installed")
	}
	home, _ := os.UserHomeDir()
	creds, err := os.ReadFile(filepath.Join(home, ".claude", ".credentials.json"))
	if err != nil {
		t.Skip("Claude Code is not logged in here: " + err.Error())
	}
	data := isolate(t)
	exe := filepath.Join(t.TempDir(), "adbuddy")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", exe, "../../cmd/adbuddy").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	rr := &realRun{t: t, ws: t.TempDir(), r: newChatRig(t)}

	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	for name, b := range map[string][]byte{".credentials.json": creds, ".claude.json": []byte("{}")} {
		if err := os.WriteFile(filepath.Join(cfg, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := agent.TrustFolder(agent.ClaudeConfigFile(os.Getenv, home), rr.ws); err != nil {
		t.Fatal(err)
	}
	rr.r.l.adapter.cfgDir = cfg
	logbuf := &syncBuffer{}
	rr.r.l.log = logbuf
	t.Cleanup(func() { t.Log("host log:\n" + logbuf.String()) })
	if _, err := workspace.Init(workspace.Options{Dir: rr.ws, Exe: exe}); err != nil {
		t.Fatal(err)
	}
	rr.r.l.useWorkspace(rr.ws)

	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		if req.Kind == ipc.KindHook {
			return rr.r.l.handleHook(ctx, rr.ws, req, c)
		}
		return ipc.Exit{Code: 1, ErrorCode: "UNSUPPORTED"}
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })

	// The phone: one reader for the whole test (a read that times out closes
	// the connection), answering the permission requests as decision says.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		for {
			_, b, err := rr.r.c.Read(ctx)
			if err != nil {
				return
			}
			var m bridge.Message
			if json.Unmarshal(b, &m) != nil {
				continue
			}
			rr.mu.Lock()
			rr.msgs = append(rr.msgs, m)
			d := rr.decision
			rr.mu.Unlock()
			if m.Type == bridge.TypePermRequest {
				var req bridge.PermissionRequest
				_ = json.Unmarshal(m.Payload, &req)
				sendApp(t, rr.r.c, bridge.TypePermReply, "", bridge.PermissionReply{ID: req.ID, Decision: d})
			}
		}
	}()
	return rr
}

// claude runs `claude -p prompt` in the environment and waits for it.
func (rr *realRun) claude(prompt string) {
	rr.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "claude", "-p", prompt, "--model", "sonnet", "--max-turns", "4", "--permission-mode", "default")
	cmd.Dir = rr.ws
	cmd.Stdin = strings.NewReader("")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		rr.t.Fatalf("claude: %v\n%s", err, out.String())
	}
	rr.t.Logf("claude said: %s", strings.TrimSpace(out.String()))
	time.Sleep(time.Second) // the transcript is read every 300 ms
}

// run answers decision to the permission requests of one claude run and
// returns what the phone received meanwhile.
func (rr *realRun) run(decision, prompt string) []bridge.Message {
	rr.mu.Lock()
	rr.decision = decision
	before := len(rr.msgs)
	rr.mu.Unlock()
	rr.claude(prompt)
	rr.mu.Lock()
	defer rr.mu.Unlock()
	return append([]bridge.Message(nil), rr.msgs[before:]...)
}

func count(ms []bridge.Message, typ string) (n int) {
	for _, m := range ms {
		if m.Type == typ {
			n++
		}
	}
	return n
}

func TestRealClaudeCode(t *testing.T) {
	rr := newRealRun(t)
	exists := func(name string) bool { _, err := os.Stat(filepath.Join(rr.ws, name)); return err == nil }

	t.Run("allow", func(t *testing.T) {
		ms := rr.run("allow", "Ejecuta con Bash exactamente: touch uno.txt . Luego responde solo: hecho")
		if !exists("uno.txt") {
			t.Error("allowed from the phone, but the command did not run")
		}
		if count(ms, bridge.TypePermRequest) != 1 || count(ms, bridge.TypePermResolved) != 1 {
			t.Errorf("permission events: %d requests, %d resolved", count(ms, bridge.TypePermRequest), count(ms, bridge.TypePermResolved))
		}
		if count(ms, bridge.TypeToolActivity) < 2 || count(ms, bridge.TypeChatMessage) < 1 {
			t.Errorf("transcript events: %d tool.activity, %d chat.message", count(ms, bridge.TypeToolActivity), count(ms, bridge.TypeChatMessage))
		}
		if v := rr.r.l.state().View; v != "terminal" { // SessionEnd came
			t.Errorf("view %q after the session ended", v)
		}
	})

	t.Run("deny", func(t *testing.T) {
		ms := rr.run("deny", "Ejecuta con Bash exactamente: touch dos.txt . Luego responde solo: hecho o no")
		if exists("dos.txt") {
			t.Error("denied from the phone, but the command ran")
		}
		if count(ms, bridge.TypePermRequest) != 1 {
			t.Errorf("%d requests", count(ms, bridge.TypePermRequest))
		}
	})

	t.Run("allow always is exact", func(t *testing.T) {
		rr.run("allow_always", "Ejecuta con Bash exactamente: touch tres.txt . Luego responde solo: hecho")
		local, err := os.ReadFile(filepath.Join(rr.ws, ".claude", "settings.local.json"))
		if err != nil || !strings.Contains(string(local), `Bash(touch tres.txt)`) || strings.Contains(string(local), "*") {
			t.Fatalf("settings.local.json: %q (%v)", local, err)
		}
		// The rule now allows that exact command with no question asked.
		if err := os.Remove(filepath.Join(rr.ws, "tres.txt")); err != nil {
			t.Fatal(err)
		}
		ms := rr.run("deny", "Ejecuta con Bash exactamente: touch tres.txt . Luego responde solo: hecho")
		if !exists("tres.txt") || count(ms, bridge.TypePermRequest) != 0 {
			t.Errorf("the exact rule did not allow the same command again (%d requests)", count(ms, bridge.TypePermRequest))
		}
	})
}
