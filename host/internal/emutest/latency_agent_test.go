package emutest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
)

// The model half of the latency harness (docs/superpowers/plans/2026-10-03-latencia.md):
// the real agent (Claude Code, headless) does a few typical tasks on the
// emulator through the host's daemon, once per configuration, and each run
// records the time, the turns, the tokens, the cost and how long the commands
// took, so the model's share of the wait can be told from the phone's. It
// spends tokens, so it needs ADBUDDY_LATENCY_AGENT=1 besides ADBUDDY_LATENCY=1;
// ADBUDDY_LATENCY_CONFIGS ("opus:medium,sonnet:low,…", model alias or id : effort)
// and ADBUDDY_LATENCY_TASKS (the tasks' numbers, "1,3") narrow it. Every run
// has a budget (ADBUDDY_LATENCY_BUDGET, 1.5 dollars by default) and 6 minutes.

type benchTask struct {
	name   string
	prompt string
}

var benchTasks = []benchTask{
	{"1 read: Android version and battery", "What Android version is the phone running and what is its battery level? Answer in one short line."},
	{"2 open an app and read it", "Open the Settings app and tell me the first three entries on its main screen. Answer in one short line."},
	{"3 navigate two screens", "In the Settings app, open Network & internet and list the names of the entries it shows. Answer in one short line."},
	{"4 list installed apps", "How many third-party (user-installed) apps does the phone have? Name them. Answer in one short line."},
	{"5 search and type", "In the Settings app, tap the search bar, type the word wifi and tell me the title of the first result. Answer in one short line."},
}

// modelIDs turns the aliases of the configurations into model ids.
var modelIDs = map[string]string{"opus": "claude-opus-5-5", "sonnet": "claude-sonnet-5-5", "fable": "claude-fable-5-1"}

type benchUsage struct {
	In         int64 `json:"input_tokens"`
	Out        int64 `json:"output_tokens"`
	CacheRead  int64 `json:"cache_read_input_tokens"`
	CacheWrite int64 `json:"cache_creation_input_tokens"`
}

// benchRunResult is the part of claude's JSON that is flat.
type benchRunResult struct {
	IsError    bool    `json:"is_error"`
	Result     string  `json:"result"`
	NumTurns   int     `json:"num_turns"`
	DurationMs int64   `json:"duration_ms"`
	APIMs      int64   `json:"duration_api_ms"`
	Cost       float64 `json:"total_cost_usd"`
}

type benchRun struct {
	Task, Model, Effort string
	WallMs              int64
	benchRunResult
	Usage       benchUsage
	Commands    []string
	CommandMs   []int64
	CommandsSum int64
	Err         string
}

// callLog records each request of the agent with how long the host took.
type callLog struct {
	mu    sync.Mutex
	names []string
	ms    []int64
}

func (c *callLog) add(name string, d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.names = append(c.names, name)
	c.ms = append(c.ms, d.Milliseconds())
}

func (c *callLog) take() ([]string, []int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, m := c.names, c.ms
	c.names, c.ms = nil, nil
	return n, m
}

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
	configs := strings.Split(envOr("ADBUDDY_LATENCY_CONFIGS", "opus:medium,sonnet:low,sonnet:medium"), ",")
	wantTask := map[string]bool{}
	for _, s := range strings.Split(envOr("ADBUDDY_LATENCY_TASKS", "1,2,3,4,5"), ",") {
		wantTask[strings.TrimSpace(s)] = true
	}
	budget := envOr("ADBUDDY_LATENCY_BUDGET", "1.5")

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
	calls := &callLog{}
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		t0 := time.Now()
		exit := h.d.Handle(ctx, req, c)
		name := req.UIOp
		if req.Kind == ipc.KindExec {
			name = "adb " + clip(strings.Join(req.Argv, " "), 50)
		} else if req.Kind == ipc.KindUI {
			name = "adbuddy " + req.UIOp
		}
		calls.add(name, time.Since(t0))
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

	var all []benchRun
	var rep strings.Builder
	defer func() {
		out := os.Getenv("ADBUDDY_LATENCY_OUT")
		if out == "" {
			return
		}
		if f, err := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			defer f.Close()
			fmt.Fprintf(f, "\n### Agent runs (real Claude Code on the emulator, one run per task and configuration)\n\n| task | model:effort | wall s | api s | commands s | turns | commands | in tok | out tok | cache read | cache write | $ | error |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|\n%s\n", rep.String())
		}
		if f, err := os.OpenFile(out+".agent.jsonl", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			defer f.Close()
			for _, r := range all {
				b, _ := json.Marshal(r)
				fmt.Fprintln(f, string(b))
			}
		}
	}()

	for _, task := range benchTasks {
		if !wantTask[task.name[:1]] {
			continue
		}
		for _, cfg := range configs {
			alias, effort, _ := strings.Cut(cfg, ":")
			model := modelIDs[alias]
			if model == "" {
				model = alias
			}
			// Back to the launcher, as when the user starts from the chat.
			e.shell(t, "input keyevent KEYCODE_HOME")
			time.Sleep(1500 * time.Millisecond)
			calls.take()
			ctx, cancel := context.WithTimeout(e.ctx, 6*time.Minute)
			cmd := exec.CommandContext(ctx, claudePath, "-p", task.prompt, "--model", model, "--effort", effort, "--output-format", "json",
				"--permission-mode", "dontAsk", "--max-budget-usd", budget, "--no-session-persistence",
				"--setting-sources", "project,local", "--strict-mcp-config")
			cmd.Dir, cmd.Env = ws, env
			var out, errb bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errb
			t0 := time.Now()
			runErr := cmd.Run()
			cancel()
			r := benchRun{Task: task.name, Model: model, Effort: effort, WallMs: time.Since(t0).Milliseconds()}
			r.Commands, r.CommandMs = calls.take()
			for _, ms := range r.CommandMs {
				r.CommandsSum += ms
			}
			if runErr != nil {
				r.Err = clip(runErr.Error()+" "+errb.String(), 200)
			}
			var raw struct {
				benchRunResult
				Usage benchUsage `json:"usage"`
			}
			if json.Unmarshal(out.Bytes(), &raw) == nil {
				r.benchRunResult, r.Usage = raw.benchRunResult, raw.Usage
				if raw.IsError && r.Err == "" {
					r.Err = clip(raw.Result, 200)
				}
			} else if r.Err == "" {
				r.Err = "unreadable output: " + clip(out.String(), 200)
			}
			all = append(all, r)
			fmt.Fprintf(&rep, "| %s | %s:%s | %.1f | %.1f | %.1f | %d | %d | %d | %d | %d | %d | %.3f | %s |\n", task.name, alias, effort,
				float64(r.WallMs)/1000, float64(r.APIMs)/1000, float64(r.CommandsSum)/1000, r.NumTurns, len(r.Commands),
				r.Usage.In, r.Usage.Out, r.Usage.CacheRead, r.Usage.CacheWrite, r.Cost, r.Err)
			t.Logf("%s %s:%s wall %.1fs api %.1fs commands %.1fs turns %d $%.3f err=%q\n  answer: %s", task.name, alias, effort,
				float64(r.WallMs)/1000, float64(r.APIMs)/1000, float64(r.CommandsSum)/1000, r.NumTurns, r.Cost, r.Err, clip(r.Result, 200))
		}
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
