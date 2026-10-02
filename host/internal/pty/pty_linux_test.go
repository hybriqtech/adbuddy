//go:build linux

package pty

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// startDeaf starts a child that never reads its terminal: this test binary,
// which TestMain turns into a program that only sleeps (ADBUDDY_FAKE_CHILD).
func startDeaf(t *testing.T) *unixPTY {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p, err := Start(Options{Path: exe, Dir: t.TempDir(), Env: append(os.Environ(), "ADBUDDY_FAKE_CHILD=1"), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Kill() })
	return p.(*unixPTY)
}

// getfl is F_GETFL of the master, without Fd() (which would block it).
func getfl(t *testing.T, p *unixPTY) int {
	t.Helper()
	sc, err := p.f.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var fl int
	var ferr error
	if err := sc.Control(func(fd uintptr) { fl, ferr = unix.FcntlInt(fd, unix.F_GETFL, 0) }); err != nil {
		t.Fatal(err)
	}
	if ferr != nil {
		t.Fatal(ferr)
	}
	return fl
}

// The Kill frees a Write that the program's unread input blocks, even while
// somebody still holds the slave open (the kernel then never wakes the Write
// with EIO): the test holds it open itself, as a grandchild in another
// process group would.
func TestKillFreesAWriteBlockedOnADeafChild(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a real child")
	}
	p := startDeaf(t)

	var n uint32
	sc, err := p.f.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	if err := sc.Control(func(fd uintptr) {
		n, err = unix.IoctlGetUint32(int(fd), unix.TIOCGPTN)
	}); err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatalf("cannot hold the slave open: %v", err)
	}
	defer slave.Close()

	// Lines, so that the terminal in canonical mode queues them instead of
	// dropping what no line fits.
	line := []byte(strings.Repeat("x", 1023) + "\r")
	const most = 4 << 20
	var sent atomic.Int64
	result := make(chan error, 1)
	go func() {
		for sent.Load() < most {
			if _, err := p.Write(line); err != nil {
				result <- err
				return
			}
			sent.Add(int64(len(line)))
		}
		result <- nil
	}()

	// Stuck: no progress for a second.
	last, since := int64(-1), time.Now()
	for {
		select {
		case err := <-result:
			if err == nil {
				t.Skipf("the terminal took %d bytes with no Write stuck: nothing to free", sent.Load())
			}
			t.Fatalf("the Write failed before the Kill: %v", err)
		case <-time.After(50 * time.Millisecond):
		}
		if cur := sent.Load(); cur != last {
			last, since = cur, time.Now()
		} else if time.Since(since) >= time.Second {
			break
		}
	}
	t.Logf("the Write got stuck after %d bytes", sent.Load())

	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("the Write returned without an error after the Kill")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the Kill did not free the blocked Write")
	}
}

// The master stays non-blocking under the poller: Close frees a blocked
// Write only then. Resize used to call Fd(), which blocks it again.
func TestMasterStaysNonblocking(t *testing.T) {
	p := startDeaf(t)
	if getfl(t, p)&unix.O_NONBLOCK == 0 {
		t.Skip("pollableMaster fell back to the blocking master")
	}
	if err := p.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	if getfl(t, p)&unix.O_NONBLOCK == 0 {
		t.Fatal("the master is blocking again after Resize")
	}
}
