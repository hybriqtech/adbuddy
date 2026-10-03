package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/agent"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
	"github.com/hybriqtech/adbuddy/host/internal/testutil/latbench"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
)

// The model half of the latency harness on a fake phone (latbench.Phone): the
// real agent does typical tasks through the real shim and the host's real
// daemon, and the phone answers at once, so what is left is the model's own
// time, turns and tokens. It spends tokens: ADBUDDY_LATENCY=1 and
// ADBUDDY_LATENCY_AGENT=1. ADBUDDY_LATENCY_CONFIGS ("opus:medium,sonnet:low"),
// ADBUDDY_LATENCY_TASKS ("1,3"), ADBUDDY_LATENCY_MAXRUNS and
// ADBUDDY_LATENCY_BUDGET (dollars a run, 1.5) narrow it.
func TestLatencyAgentFakePhone(t *testing.T) {
	if !testutil.LatencyOn() || os.Getenv("ADBUDDY_LATENCY_AGENT") != "1" {
		t.Skip("set ADBUDDY_LATENCY=1 and ADBUDDY_LATENCY_AGENT=1 (it spends tokens)")
	}
	claude, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude is not installed")
	}
	phone, err := latbench.NewPhone()
	if err != nil {
		t.Fatal(err)
	}
	cls, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	data := filepath.Join(tmp, "data")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	ws := filepath.Join(tmp, "ws")
	d := daemon.New(daemon.Options{
		Classifier: cls, ADB: phone.Executor(), Journal: journal.OpenSigned(filepath.Join(ws, "journal"), []byte(secret)),
		Confirmer: confirm.DenyAll{Out: os.Stderr}, Serial: "dev1", Session: "bench", Lang: "en", Workspace: ws, UI: phone.UI,
	})
	calls := &latbench.CallLog{}
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		t0 := time.Now()
		exit := d.Handle(ctx, req, c)
		name := "adbuddy " + req.UIOp
		if req.Kind == ipc.KindExec {
			name = "adb " + latbench.Clip(strings.Join(req.Argv, " "), 50)
		}
		calls.Add(name, time.Since(t0))
		return exit
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve() }()
	t.Cleanup(func() { srv.Close() })

	// The environment as `adbuddy run` leaves it, without the hooks (nobody
	// answers them here and each would cost the agent a process start).
	if _, err := workspace.Init(workspace.Options{Dir: ws, Exe: adbuddyBin}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".claude", "settings.json"), []byte(`{"permissions":{"allow":["Bash(adb:*)","Bash(adbuddy:*)"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, kv := range agent.Env(os.Environ(), workspace.BinDir(ws), "dev1") {
		k, _, _ := strings.Cut(kv, "=")
		if !strings.EqualFold(k, "ADBUDDY_DATA_DIR") && !strings.EqualFold(k, "ADBUDDY_CONFIG_DIR") {
			env = append(env, kv)
		}
	}
	env = append(env, "ADBUDDY_DATA_DIR="+data, "ADBUDDY_CONFIG_DIR="+filepath.Join(tmp, "cfg"), "ADBUDDY_LANG=en")

	runner := latbench.Env{Claude: claude, Dir: ws, Env: env, Budget: latbench.EnvOr("ADBUDDY_LATENCY_BUDGET", "1.5"), Calls: calls}
	configs := latbench.ParseConfigs(latbench.EnvOr("ADBUDDY_LATENCY_CONFIGS", "opus:medium,sonnet:low,sonnet:medium"))
	want := map[string]bool{}
	for _, s := range strings.Split(latbench.EnvOr("ADBUDDY_LATENCY_TASKS", "1,2,3,4,5"), ",") {
		want[strings.TrimSpace(s)] = true
	}
	maxRuns, _ := strconv.Atoi(latbench.EnvOr("ADBUDDY_LATENCY_MAXRUNS", "100"))

	var runs []latbench.Run
	defer func() {
		latbench.Write(os.Getenv("ADBUDDY_LATENCY_OUT"), "Agent runs on the fake phone (real Claude Code, real shim and daemon, a phone that answers at once)", runs)
	}()
	for _, task := range latbench.Tasks {
		if !want[task.Name[:1]] {
			continue
		}
		for _, cfg := range configs {
			if len(runs) >= maxRuns {
				return
			}
			phone.Reset()
			r := runner.Do(context.Background(), task, cfg)
			runs = append(runs, r)
			t.Logf("%s\n  phone saw: %q", r.Describe(), phone.Log())
		}
	}
}
