package ipc

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// TestMain lets the test binary be a client in a process of its own: with
// ADBUDDY_IPC_DIAL set to a data folder it sends a ping, reads one frame of
// the answer and ends without waiting for the rest.
func TestMain(m *testing.M) {
	if data := os.Getenv("ADBUDDY_IPC_DIAL"); data != "" {
		os.Exit(dialAndQuit(data))
	}
	os.Exit(m.Run())
}

func dialAndQuit(data string) int {
	secret, err := ReadSecret(data)
	if err != nil {
		return 1
	}
	nc, err := net.Dial("unix", SocketPath(data))
	if err != nil {
		return 1
	}
	b, _ := json.Marshal(Request{Secret: secret, Kind: KindPing})
	if WriteFrame(nc, FrameRequest, b) != nil {
		return 1
	}
	if _, _, err := ReadFrame(nc); err != nil {
		return 1
	}
	return 0 // the system closes the connection
}

// namesPeers: the systems whose sockets name the process at the other end.
var namesPeers = runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "windows"

// serveOnce listens on a short data folder with h.
func serveOnce(t *testing.T, h Handler) string {
	t.Helper()
	data := shortDir(t)
	secret, err := EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := Listen(data, secret, h)
	if err != nil {
		t.Skipf("no socket: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	return data
}

// V07: the handler knows the process at the other end, without asking the
// client: here this test's own process, alive and the same.
func TestPeerIsTheConnectingProcess(t *testing.T) {
	type seen struct {
		p    Peer
		same bool
	}
	got := make(chan seen, 1)
	data := serveOnce(t, func(ctx context.Context, req Request, c *Conn) Exit {
		p := c.Peer()
		got <- seen{p, p.Same()}
		return Exit{}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := Call(ctx, data, Request{Kind: KindPing}, nil, io.Discard, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	s := <-got
	if !namesPeers {
		if s.p.Known {
			t.Fatalf("%+v: this system was not expected to name the peer", s.p)
		}
		return
	}
	if !s.p.Known || s.p.Err != nil || s.p.PID != os.Getpid() || !s.same {
		t.Fatalf("peer %+v, same %v; want this process (%d), alive", s.p, s.same, os.Getpid())
	}
}

// V07 (C1): a peer that ended after connecting is no longer the same,
// whatever process gets its PID later.
func TestPeerThatEndedIsNotTheSame(t *testing.T) {
	if !namesPeers {
		t.Skip("this system does not name the peer")
	}
	type seen struct {
		p             Peer
		before, after bool
	}
	ended := make(chan struct{})
	got := make(chan seen, 1)
	data := serveOnce(t, func(ctx context.Context, req Request, c *Conn) Exit {
		p := c.Peer()
		before := p.Same()
		_, _ = c.Stdout().Write([]byte("go")) // the client ends once it reads this
		<-ended
		got <- seen{p, before, p.Same()}
		return Exit{}
	})
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), "ADBUDDY_IPC_DIAL="+data)
	runErr := cmd.Run()
	close(ended)
	if runErr != nil {
		t.Fatalf("client: %v", runErr)
	}
	select {
	case s := <-got:
		if !s.p.Known || s.p.PID != cmd.Process.Pid || !s.before || s.after {
			t.Fatalf("peer %+v, same before %v and after %v; want the client (%d), the same only while it ran", s.p, s.before, s.after, cmd.Process.Pid)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the request never reached the handler")
	}
}

// A handler called without a connection (the tests of other packages) has
// an unknown peer.
func TestPeerWithoutAConnectionIsUnknown(t *testing.T) {
	var c *Conn
	if p := c.Peer(); p.Known || p.Same() {
		t.Fatalf("%+v", p)
	}
}
