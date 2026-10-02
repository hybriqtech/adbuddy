package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/claudecode"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
)

// runHookCmd runs `adbuddy hook args…` with stdin, as Claude Code does.
func runHookCmd(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	code = mainContext(context.Background(), append([]string{"hook"}, args...), strings.NewReader(stdin), &out, &errb)
	return out.String(), errb.String(), code
}

func hookDaemon(t *testing.T, data string, h func(ipc.Request, string) (string, ipc.Exit)) {
	t.Helper()
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		in, _ := io.ReadAll(c.Stdin())
		out, exit := h(req, string(in))
		if out != "" {
			_, _ = c.Stdout().Write([]byte(out))
		}
		return exit
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
}

func TestHookPassesThePayloadAndTheDecision(t *testing.T) {
	data := isolate(t)
	var got ipc.Request
	var body string
	hookDaemon(t, data, func(req ipc.Request, in string) (string, ipc.Exit) {
		got, body = req, in
		return `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`, ipc.Exit{}
	})
	out, errOut, code := runHookCmd(t, `{"session_id":"x"}`, "PermissionRequest")
	if code != 0 || errOut != "" || !strings.Contains(out, `"behavior":"allow"`) {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got.Kind != ipc.KindHook || len(got.Argv) != 1 || got.Argv[0] != "PermissionRequest" || body != `{"session_id":"x"}` {
		t.Fatalf("request %+v, body %q", got, body)
	}
}

// A hook that blocks the agent is worse than one that does nothing (an exit
// code 2 blocks, spec M4 §3.3): every failure is an empty stdout and a 0.
func TestHookFailsOpen(t *testing.T) {
	isolate(t) // no daemon
	for name, c := range map[string]struct {
		stdin string
		args  []string
	}{
		"daemon not running": {`{"session_id":"x"}`, []string{"Stop"}},
		"no event":           {`{}`, nil},
		"unknown event":      {`{}`, []string{"Evil"}},
		"too many args":      {`{}`, []string{"Stop", "x"}},
		"unknown flag":       {`{}`, []string{"--nope"}},
		"huge payload":       {strings.Repeat("a", claudecode.MaxHookInput+10), []string{"Stop"}},
	} {
		out, _, code := runHookCmd(t, c.stdin, c.args...)
		if code != 0 || out != "" {
			t.Errorf("%s: exit %d, stdout %q", name, code, out)
		}
	}
}

func TestHookFailsOpenOnADaemonThatMisbehaves(t *testing.T) {
	for name, h := range map[string]func(ipc.Request, string) (string, ipc.Exit){
		"exit code": func(ipc.Request, string) (string, ipc.Exit) { return `{"a":1}`, ipc.Exit{Code: 1, ErrorCode: "X"} },
		"not json":  func(ipc.Request, string) (string, ipc.Exit) { return `allow`, ipc.Exit{} },
		"empty":     func(ipc.Request, string) (string, ipc.Exit) { return ``, ipc.Exit{} },
		"huge response": func(ipc.Request, string) (string, ipc.Exit) {
			return `"` + strings.Repeat("a", 2<<20) + `"`, ipc.Exit{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			hookDaemon(t, isolate(t), h)
			out, _, code := runHookCmd(t, `{"session_id":"x"}`, "Stop")
			if code != 0 || out != "" {
				t.Errorf("exit %d, stdout %q", code, out)
			}
		})
	}
}

func TestHookIsNotInTheHelp(t *testing.T) {
	isolate(t)
	out, _, _ := run(t, "--help")
	if strings.Contains(out, "hook") {
		t.Errorf("the hook command is shown in the help:\n%s", out)
	}
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("boom") }

// Review M4: a panic exits with 2, which BLOCKS in Claude Code. The hook fails
// open: nothing written and exit 0.
func TestHookFailsOpenOnAPanic(t *testing.T) {
	isolate(t)
	var out, errb bytes.Buffer
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the panic got out of the hook: %v", r)
		}
	}()
	code := mainContext(context.Background(), []string{"hook", "Stop"}, panicReader{}, &out, &errb)
	if code != 0 || out.Len() != 0 {
		t.Fatalf("exit %d, stdout %q", code, out.String())
	}
}

// V05, round 1: a PermissionRequest whose payload is too large still tells the
// daemon, with an empty payload, so that it holds the chat; other events with
// a payload too large do not reach it.
func TestHookTooLargeStillTellsAPermissionRequest(t *testing.T) {
	data := isolate(t)
	var mu sync.Mutex
	var got []string
	hookDaemon(t, data, func(req ipc.Request, in string) (string, ipc.Exit) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, strings.Join(req.Argv, " ")+" "+in)
		return "", ipc.Exit{}
	})
	huge := strings.Repeat("a", claudecode.MaxHookInput+10)
	for _, ev := range []string{"Stop", "PermissionRequest"} {
		if out, _, code := runHookCmd(t, huge, ev); code != 0 || out != "" {
			t.Fatalf("%s: exit %d, stdout %q", ev, code, out)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != "PermissionRequest {}" {
		t.Fatalf("the daemon got %q", got)
	}
}
