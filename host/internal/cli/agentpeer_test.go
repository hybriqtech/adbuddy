package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/pty"
	"github.com/hybriqtech/adbuddy/host/internal/session"
)

// peerHelper is what the test binary does as a process of the agent's tree
// (ADBUDDY_PEER_MODE, see TestMain), always with ADBUDDY_SOURCE empty:
// "agent" is the agent in its PTY, which asks serve itself and then through
// two children; "child" asks serve and waits for the answers; "quitter"
// sends its request and ends without waiting for the answer.
func peerHelper(mode string) int {
	data := os.Getenv("ADBUDDY_DATA_DIR")
	ask := func(kind, arg string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		e, err := ipc.Call(ctx, data, ipc.Request{Kind: kind, Argv: []string{arg}}, nil, io.Discard, io.Discard, nil)
		if err != nil {
			return err.Error()
		}
		return e.ErrorCode
	}
	switch mode {
	case "agent":
		fmt.Printf("self:%s;\r\n", ask(ipc.KindSession, "self"))
		exe, _ := os.Executable()
		for _, m := range []string{"child", "quitter"} {
			c := exec.Command(exe)
			c.Env = append(os.Environ(), "ADBUDDY_PEER_MODE="+m)
			out, err := c.CombinedOutput()
			fmt.Printf("%s:%s %v;\r\n", m, strings.TrimSpace(string(out)), err)
		}
		time.Sleep(time.Hour) // until serve stops it
	case "child":
		fmt.Println(ask(ipc.KindSession, "child"), ask(ipc.KindRemote, "status"))
	case "quitter":
		secret, err := ipc.ReadSecret(data)
		if err != nil {
			return 1
		}
		nc, err := net.Dial("unix", ipc.SocketPath(data))
		if err != nil {
			return 1
		}
		b, _ := json.Marshal(ipc.Request{Secret: secret, Kind: ipc.KindSession, Argv: []string{"quitter"}})
		if ipc.WriteFrame(nc, ipc.FrameRequest, b) != nil {
			return 1
		}
	default:
		return 2
	}
	return 0
}

// peerEnv is the fake agent's environment: this one without
// ADBUDDY_SOURCE, as an agent that cleared it.
func peerEnv(data string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(k) {
		case "ADBUDDY_SOURCE", "ADBUDDY_PEER_MODE", "ADBUDDY_DATA_DIR":
			continue
		}
		env = append(env, kv)
	}
	return append(env, "ADBUDDY_PEER_MODE=agent", "ADBUDDY_DATA_DIR="+data)
}

// V07: serve tells the agent's processes by the other end of the socket, not
// by what they declare. The agent and its children are refused with
// ADBUDDY_SOURCE empty, also one that ends right after sending its request;
// a process outside the agent's tree (this test) is served, unless it
// declares itself the agent.
func TestServeRefusesTheAgentsTreeWhateverItDeclares(t *testing.T) {
	data := isolate(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(session.Options{
		Dir:   t.TempDir(),
		Look:  func(string) (string, error) { return exe, nil },
		Env:   func(string) []string { return peerEnv(data) },
		Start: pty.Start,
	})
	t.Cleanup(sess.Stop)
	type answer struct{ probe, code string }
	answers := make(chan answer, 16)
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		var e ipc.Exit
		probe := "remote"
		if req.Kind == ipc.KindRemote {
			e = (&phoneLink{}).handleRemote(ctx, req, c, nil, sess)
		} else {
			e = handleSessionIn(i18n.EN, ctx, sess, "claude", req, c)
			probe = strings.Join(req.Argv, " ")
		}
		answers <- answer{probe, e.ErrorCode}
		return e
	})
	if err != nil {
		t.Skipf("no socket: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })

	if err := sess.Start("claude", false); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	deadline := time.After(60 * time.Second)
	for len(got) < 4 {
		select {
		case a := <-answers:
			got[a.probe] = a.code
		case <-deadline:
			screen := ""
			if s := sess.Screen(); s != nil {
				screen = s.Text()
			}
			t.Fatalf("answers %v; the agent's screen:\n%s", got, screen)
		}
	}
	for _, probe := range []string{"self", "child", "quitter", "remote"} {
		if got[probe] != "AGENT_SESSION" {
			t.Errorf("%s: %q, want AGENT_SESSION", probe, got[probe])
		}
	}

	// This test is outside the agent's tree: served, unless it declares
	// itself the agent (the declaration only ever adds a refusal).
	call := func(req ipc.Request) string {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		e, err := ipc.Call(ctx, data, req, nil, io.Discard, io.Discard, nil)
		if err != nil {
			t.Fatal(err)
		}
		return e.ErrorCode
	}
	if code := call(ipc.Request{Kind: ipc.KindSession, Argv: []string{"outside"}}); code != "USAGE" {
		t.Errorf("outside: %q, want USAGE (served)", code)
	}
	if code := call(ipc.Request{Kind: ipc.KindRemote, Argv: []string{"status"}}); code != "REMOTE_UNAVAILABLE" {
		t.Errorf("remote outside: %q, want REMOTE_UNAVAILABLE (served)", code)
	}
	agentEnv := map[string]string{"ADBUDDY_SOURCE": "agent"}
	if code := call(ipc.Request{Kind: ipc.KindSession, Argv: []string{"declared"}, Env: agentEnv}); code != "AGENT_SESSION" {
		t.Errorf("declared: %q, want AGENT_SESSION", code)
	}
}

// treeAgentPTY is a fake agent that tells its process tree with contains.
type treeAgentPTY struct {
	*fakeAgentPTY
	contains func(pid int) (bool, error)
}

func (f treeAgentPTY) Contains(pid int) (bool, error) { return f.contains(pid) }

// V07: how fromAgent decides, peer by peer. A peer the system does not name
// leaves it to the declaration; one it names but that cannot be checked (an
// error, a PID below 1, a process that is no longer the one that connected)
// is the agent, and the agent's tree is never asked about a PID below 1 (on
// Unix getsid(0) is the caller's own session).
func TestPeerIsAgent(t *testing.T) {
	var asked []int // written by Contains, on this goroutine
	sess := session.New(session.Options{
		Dir:  t.TempDir(),
		Look: func(name string) (string, error) { return "/bin/" + name, nil },
		Env:  func(string) []string { return nil },
		Start: func(pty.Options) (pty.PTY, error) {
			return treeAgentPTY{newFakeAgentPTY(), func(pid int) (bool, error) {
				asked = append(asked, pid)
				return false, nil
			}}, nil
		},
	})
	t.Cleanup(sess.Stop)
	if err := sess.Start("claude", false); err != nil {
		t.Fatal(err)
	}
	agentEnv := map[string]string{"ADBUDDY_SOURCE": "agent"}
	for _, c := range []struct {
		name  string
		env   map[string]string
		peer  ipc.Peer
		agent bool
		asked []int
	}{
		{"not named", nil, ipc.Peer{}, false, nil},
		{"not named, declared", agentEnv, ipc.Peer{}, true, nil},
		{"named with an error", nil, ipc.Peer{PID: 7, Known: true, Err: errors.New("gone")}, true, nil},
		{"PID 0", nil, ipc.Peer{PID: 0, Known: true}, true, nil},
		{"PID -1", nil, ipc.Peer{PID: -1, Known: true}, true, nil},
		// Out of the tree, but not pinned when it connected: Same is
		// false, as for a process that ended or whose PID was reused.
		{"no longer the same", nil, ipc.Peer{PID: os.Getpid(), Known: true}, true, []int{os.Getpid()}},
	} {
		asked = nil
		if got := peerIsAgent(sess, ipc.Request{Env: c.env}, c.peer); got != c.agent {
			t.Errorf("%s: %v, want %v", c.name, got, c.agent)
		}
		if fmt.Sprint(asked) != fmt.Sprint(c.asked) {
			t.Errorf("%s: the tree was asked about %v, want %v", c.name, asked, c.asked)
		}
	}
	// A request with no connection (a handler called by a test) has an
	// unknown peer: only the declaration decides.
	if fromAgent(sess, ipc.Request{}, nil) || !fromAgent(sess, ipc.Request{Env: agentEnv}, nil) {
		t.Error("no connection: the declaration must decide")
	}
}
