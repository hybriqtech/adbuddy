package emutest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/agent"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
	"github.com/hybriqtech/adbuddy/host/internal/testutil/latbench"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
)

// The model half of the latency harness on the emulator (docs/superpowers/plans/2026-10-03-latencia.md):
// the real agent (Claude Code, headless) does the typical tasks of latbench
// through the host's daemon, once per configuration, and each run records the
// time, the turns, the tokens, the cost and how long the commands took, so the
// model's share of the wait can be told from the phone's. It spends tokens, so
// it needs ADBUDDY_LATENCY_AGENT=1 besides ADBUDDY_LATENCY=1;
// ADBUDDY_LATENCY_CONFIGS ("opus:medium,sonnet:low,…", model alias or id : effort),
// ADBUDDY_LATENCY_TASKS (the tasks' numbers, "1,3") and ADBUDDY_LATENCY_MAXRUNS
// narrow it. Every run has a budget (ADBUDDY_LATENCY_BUDGET, 1.5 dollars by
// default) and 6 minutes.

// serialUI is appUI that is safe off the test's goroutine: it fails with an
// error, not the test.
func (l *link) serialUI() uiFunc {
	var mu sync.Mutex
	n := 0
	return func(ctx context.Context, op string, args json.RawMessage) (json.RawMessage, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		id := fmt.Sprintf("bench_%d", n)
		if err := l.srv.Send(bridge.TypeUIRequest, bridge.UIRequest{ID: id, Op: op, Args: args, ExpiresAt: time.Now().Add(10 * time.Second).UTC()}); err != nil {
			return nil, daemon.ErrNoApp
		}
		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			m, ok := l.take(bridge.TypeUIResponse, time.Until(deadline))
			if !ok {
				break
			}
			var r bridge.UIResponse
			if json.Unmarshal(m.Payload, &r) != nil || r.ID != id {
				continue
			}
			if r.OK {
				return r.Result, nil
			}
			return nil, &daemon.UIError{Code: r.Error.Code, Text: r.Error.Text}
		}
		return nil, daemon.ErrUITimeout
	}
}

func TestLatencyAgent(t *testing.T) {
	if !testutil.LatencyOn() || os.Getenv("ADBUDDY_LATENCY_AGENT") != "1" {
		t.Skip("set ADBUDDY_LATENCY=1 and ADBUDDY_LATENCY_AGENT=1 (it spends tokens)")
	}
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude is not installed")
	}
	configs := latbench.ParseConfigs(latbench.EnvOr("ADBUDDY_LATENCY_CONFIGS", "opus:medium,sonnet:low,sonnet:medium"))
	wantTask := map[string]bool{}
	for _, s := range strings.Split(latbench.EnvOr("ADBUDDY_LATENCY_TASKS", "1,2,3,4,5"), ",") {
		wantTask[strings.TrimSpace(s)] = true
	}

	l := openLink(t)
	e := l.e
	e.restoreAtCleanup(t, "secure", "enabled_accessibility_services", e.shell(t, "settings get secure enabled_accessibility_services"))
	e.restoreAtCleanup(t, "secure", "accessibility_enabled", e.shell(t, "settings get secure accessibility_enabled"))
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")
	if _, err := phoneapp.EnableAccessibility(e.ctx, e.adb, l.v); err != nil {
		t.Fatalf("accessibility: %v", err)
	}
	if !poll(20*time.Second, func() bool { return a11yBound(e.read(t, "", "accessibility")) }) {
		t.Fatal("ADBuddy's service is not bound 20 s after enabling it")
	}

	// The host: the daemon of serve, on the emulator, behind the real socket.
	tmp := t.TempDir()
	data := filepath.Join(tmp, "data")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	h := newLatencyHost(t, e, l.serialUI())
	calls := &latbench.CallLog{}
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		t0 := time.Now()
		exit := h.d.Handle(ctx, req, c)
		name := req.UIOp
		if req.Kind == ipc.KindExec {
			name = "adb " + latbench.Clip(strings.Join(req.Argv, " "), 50)
		} else if req.Kind == ipc.KindUI {
			name = "adbuddy " + req.UIOp
		}
		calls.Add(name, time.Since(t0))
		return exit
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve() }()
	t.Cleanup(func() { srv.Close() })

	// The environment, as `adbuddy run` leaves it (without the hooks: nothing
	// answers them here, and each would cost the agent a process start).
	bin, err := testutil.Build("github.com/hybriqtech/adbuddy/host/cmd/adbuddy", tmp, "adbuddy")
	if err != nil {
		t.Fatal(err)
	}
	ws := filepath.Join(tmp, "ws")
	if _, err := workspace.Init(workspace.Options{Dir: ws, Exe: bin}); err != nil {
		t.Fatal(err)
	}
	settings := `{"permissions":{"allow":["Bash(adb:*)","Bash(adbuddy:*)"]}}`
	if err := os.WriteFile(filepath.Join(ws, ".claude", "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, kv := range agent.Env(os.Environ(), workspace.BinDir(ws), e.adb.Serial) {
		k, _, _ := strings.Cut(kv, "=")
		if !strings.EqualFold(k, "ADBUDDY_DATA_DIR") && !strings.EqualFold(k, "ADBUDDY_CONFIG_DIR") {
			env = append(env, kv)
		}
	}
	env = append(env, "ADBUDDY_DATA_DIR="+data, "ADBUDDY_CONFIG_DIR="+filepath.Join(tmp, "cfg"), "ADBUDDY_LANG=en")

	runner := latbench.Env{Claude: claudePath, Dir: ws, Env: env, Budget: latbench.EnvOr("ADBUDDY_LATENCY_BUDGET", "1.5"), Calls: calls}
	maxRuns, _ := strconv.Atoi(latbench.EnvOr("ADBUDDY_LATENCY_MAXRUNS", "100"))
	var runs []latbench.Run
	defer func() {
		latbench.Write(os.Getenv("ADBUDDY_LATENCY_OUT"), "Agent runs on the emulator (real Claude Code, one run per task and configuration)", runs)
	}()
	for _, task := range latbench.Tasks {
		if !wantTask[task.Name[:1]] {
			continue
		}
		for _, cfg := range configs {
			if len(runs) >= maxRuns {
				return
			}
			// Back to the launcher, as when the user starts from the chat.
			e.shell(t, "input keyevent KEYCODE_HOME")
			time.Sleep(1500 * time.Millisecond)
			r := runner.Do(e.ctx, task, cfg)
			runs = append(runs, r)
			t.Log(r.Describe())
		}
	}
}
