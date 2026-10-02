//go:build !windows

package pty

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	cpty "github.com/creack/pty"
)

type unixPTY struct {
	f    *os.File
	cmd  *exec.Cmd
	last atomic.Int64 // when output was last read (UnixNano)
	done chan struct{}
	code int
	err  error

	closeOnce sync.Once
	closeErr  error
	killOnce  sync.Once
}

// Start starts the program in a Unix pty, in its own session so Kill can end
// its whole process group.
func Start(o Options) (PTY, error) {
	cmd := exec.Command(o.Path, o.Args...)
	cmd.Dir = o.Dir
	cmd.Env = o.Env
	f, err := cpty.StartWithSize(cmd, &cpty.Winsize{Cols: uint16(o.Cols), Rows: uint16(o.Rows)})
	if err != nil {
		return nil, err
	}
	// On Linux the master goes back to non-blocking mode under the poller, so
	// that Kill's Close frees a Write blocked on a program that does not read.
	f = pollableMaster(f)
	p := &unixPTY{f: f, cmd: cmd, done: make(chan struct{})}
	go p.watch()
	return p, nil
}

// watch waits for the process and keeps its exit code; once its last output
// had time to come out it closes the terminal, so Read ends also when a
// child of the program still holds it.
func (p *unixPTY) watch() {
	err := p.cmd.Wait()
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		p.code = ee.ExitCode()
		if p.code < 0 {
			p.code = 1 // killed by a signal
		}
	case err != nil:
		p.err = err
	}
	close(p.done)
	drain(&p.last)
	p.closeTerm()
}

func (p *unixPTY) closeTerm() error {
	p.closeOnce.Do(func() { p.closeErr = p.f.Close() })
	return p.closeErr
}

// Read ends with io.EOF: the terminal's last holder closed it (EIO) or the
// terminal was closed here.
func (p *unixPTY) Read(b []byte) (int, error) {
	n, err := p.f.Read(b)
	if n > 0 {
		p.last.Store(time.Now().UnixNano())
	}
	if errors.Is(err, syscall.EIO) || errors.Is(err, os.ErrClosed) {
		err = io.EOF
	}
	return n, err
}

func (p *unixPTY) Write(b []byte) (int, error) { return p.f.Write(b) }
func (p *unixPTY) Pid() int                    { return p.cmd.Process.Pid }

func (p *unixPTY) Resize(cols, rows int) error {
	return p.setSize(cols, rows)
}

func (p *unixPTY) Wait(ctx context.Context) (int, error) {
	select {
	case <-p.done:
		return p.code, p.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Hangup sends SIGHUP to the process group, as a closing terminal does;
// nothing once the program has ended.
func (p *unixPTY) Hangup() error {
	select {
	case <-p.done:
		return nil
	default:
	}
	err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGHUP)
	if errors.Is(err, syscall.ESRCH) {
		return nil // it ended meanwhile
	}
	return err
}

// Kill ends the process group (creack/pty starts the program with Setsid, so
// its group id is its pid), also what is left of it once the program ended,
// and closes the terminal.
func (p *unixPTY) Kill() error {
	var err error
	p.killOnce.Do(func() {
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		err = p.closeTerm()
		if errors.Is(err, os.ErrClosed) {
			err = nil
		}
	})
	return err
}
