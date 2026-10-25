package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// yoloHost serves a daemon's IPC on the data folder's socket, with the YOLO
// state the test holds.
func yoloHost(t *testing.T) *daemon.Yolo {
	t.Helper()
	data := isolate(t)
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	d := daemon.New(daemon.Options{Journal: journal.Open(t.TempDir()), Lang: "en"})
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit { return d.Handle(ctx, req, c) })
	if err != nil {
		t.Skipf("no socket: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	return d.Yolo()
}

func TestYoloStatusAndOffCommands(t *testing.T) {
	y := yoloHost(t)
	if out, _, code := runCtx(t, context.Background(), "yolo", "status"); code != 0 || strings.TrimSpace(out) != "YOLO is off." {
		t.Fatalf("%d %q", code, out)
	}
	y.EnableSession()
	if out, _, code := runCtx(t, context.Background(), "yolo", "status"); code != 0 || !strings.Contains(out, "until the session ends") {
		t.Fatalf("%d %q", code, out)
	}
	if out, _, code := runCtx(t, context.Background(), "--json", "yolo", "status"); code != 0 || !strings.Contains(out, `"active":true`) || !strings.Contains(out, `"session":true`) || !strings.Contains(out, `"serve":true`) {
		t.Fatalf("%d %q", code, out)
	}
	if out, _, code := runCtx(t, context.Background(), "yolo", "off"); code != 0 || strings.TrimSpace(out) != "YOLO is off." || y.Active() {
		t.Fatalf("%d %q, active %v", code, out, y.Active())
	}
}

// The agent may turn it off, never on: `on` is a usage error that does not even reach the host.
func TestYoloOnDoesNotExist(t *testing.T) {
	y := yoloHost(t)
	t.Setenv("ADBUDDY_SOURCE", "agent")
	for _, args := range [][]string{{"yolo", "on"}, {"yolo", "session"}, {"yolo"}, {"yolo", "off", "extra"}, {"yolo", "--on"}} {
		if _, errOut, code := runCtx(t, context.Background(), args...); code != ExitUsage || y.Active() {
			t.Fatalf("%v: %d %q, active %v", args, code, errOut, y.Active())
		}
	}
	y.EnableSession()
	if _, _, code := runCtx(t, context.Background(), "yolo", "off"); code != 0 || y.Active() {
		t.Fatalf("the agent could not turn it off: %d, active %v", code, y.Active())
	}
}

func TestYoloWithoutServe(t *testing.T) {
	isolate(t)
	for _, op := range []string{"status", "off"} {
		out, _, code := runCtx(t, context.Background(), "yolo", op)
		if code != 0 || !strings.Contains(out, "serve is not running, so YOLO is off") {
			t.Fatalf("%s: %d %q", op, code, out)
		}
	}
}
