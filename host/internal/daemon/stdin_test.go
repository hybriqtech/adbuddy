package daemon

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
)

// execStdin runs argv as the agent would, with stdin piped in.
func (e *env) execStdin(t *testing.T, stdin string, argv ...string) (*stream, ipc.Exit) {
	t.Helper()
	s := &stream{stdin: strings.NewReader(stdin)}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindExec, Argv: argv, Cwd: e.ws, Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}, s)
	return s, exit
}

// A4-1: what runs through an interactive shell (and every other command whose
// effect may come from stdin) is not in the key: it is never allowed for the
// session, whatever the rules said. No key, Stdin set and, for the shells
// without a command, special.
func TestStdinCommandsHaveNoSessionKey(t *testing.T) {
	e := newEnv(t)
	cases := []struct {
		argv    []string
		special bool
	}{
		{[]string{"shell"}, true},
		{[]string{"exec-out"}, true},
		{[]string{"shell", "-t"}, true},
		{[]string{"-s", "dev1", "shell"}, true},
		{[]string{"exec-in", "sh"}, true},
		{[]string{"shell", "xargs rm -rf"}, false},
		{[]string{"shell", "cat > /sdcard/x"}, false},
		// H1: awk runs what comes through stdin, though it is not called sh.
		{[]string{"shell", "awk '{system($0)}'"}, false},
		{[]string{"shell", "sed e"}, false},
	}
	for _, c := range cases {
		e.execStdin(t, "ls\n", c.argv...)
	}
	if len(e.conf.asked) != len(cases) {
		t.Fatalf("asked %d, want %d", len(e.conf.asked), len(cases))
	}
	for i, r := range e.conf.asked {
		if r.Key != "" || !r.Stdin || (cases[i].special && !r.Special) {
			t.Errorf("%q: key %q stdin %v special %v", cases[i].argv, r.Key, r.Stdin, r.Special)
		}
	}
	// A command that does not read stdin keeps its key.
	e.exec(t, "shell", "rm", "-rf", "/sdcard/DCIM")
	if r := e.conf.asked[len(e.conf.asked)-1]; r.Key == "" || r.Stdin {
		t.Errorf("rm -rf: key %q stdin %v", r.Key, r.Stdin)
	}
}

// sessionApp is the app on the phone: it answers allow_session to every
// question, even where the button is not offered (an old or broken app).
type sessionApp struct {
	mu    sync.Mutex
	c     *bridge.Confirmer
	asked []bridge.ConfirmRequest
}

func (a *sessionApp) Connected() bool { return true }
func (a *sessionApp) Publish(typ string, payload any) error {
	q, ok := payload.(bridge.ConfirmRequest)
	if !ok {
		return nil
	}
	a.mu.Lock()
	a.asked = append(a.asked, q)
	a.mu.Unlock()
	go func() {
		raw := []byte(`{"id":"` + q.ID + `","decision":"allow_session"}`)
		a.c.Reply(raw)
	}()
	return nil
}

func (a *sessionApp) questions() []bridge.ConfirmRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]bridge.ConfirmRequest{}, a.asked...)
}

// The scenario of A4-1, end to end: `echo ls | adb shell` is asked without
// "allow during the session"; the next `echo 'pm uninstall …' | adb shell` is
// asked again and not run on the strength of the first answer.
func TestStdinShellIsAskedEveryTime(t *testing.T) {
	e := newEnv(t)
	e.d.o.ConfirmTimeout = 5 * time.Second
	app := &sessionApp{}
	app.c = bridge.NewConfirmer(app, confirm.DenyAll{})
	e.d.o.Confirmer = app.c

	s, exit := e.execStdin(t, "ls\n", "shell")
	if exit.Code != 0 || s.out.String() != "ls\n" {
		t.Fatalf("first: exit %+v out %q err %q", exit, s.out.String(), s.errOut.String())
	}
	q := app.questions()
	if len(q) != 1 || q[0].AllowSession || !q[0].Special {
		t.Fatalf("first question %+v", q)
	}
	s, exit = e.execStdin(t, "pm uninstall com.bank; svc wifi disable\n", "shell")
	if exit.Code != 0 || !strings.Contains(s.out.String(), "pm uninstall") {
		t.Fatalf("second: exit %+v out %q err %q", exit, s.out.String(), s.errOut.String())
	}
	if q = app.questions(); len(q) != 2 || q[1].AllowSession {
		t.Fatalf("the second shell was not asked again: %+v", q)
	}
	// The same holds for exec-out and for a red command that reads stdin
	// without being special.
	e.execStdin(t, "ls\n", "exec-out")
	e.execStdin(t, "ls\n", "exec-out")
	e.execStdin(t, "/sdcard/a\n", "shell", "xargs rm -rf")
	e.execStdin(t, "/sdcard\n", "shell", "xargs rm -rf")
	if q = app.questions(); len(q) != 6 {
		t.Fatalf("asked %d times, want 6", len(q))
	}
	for _, r := range q {
		if r.AllowSession {
			t.Errorf("allow_session offered: %+v", r)
		}
	}
}
