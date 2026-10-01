//go:build !windows

package pty

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"

	cpty "github.com/creack/pty"
)

type unixPTY struct {
	f        *os.File
	cmd      *exec.Cmd
	done     chan struct{}
	code     int
	err      error
	killOnce sync.Once
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
	p := &unixPTY{f: f, cmd: cmd, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
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
	}()
	return p, nil
}

func (p *unixPTY) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p *unixPTY) Write(b []byte) (int, error) { return p.f.Write(b) }
func (p *unixPTY) Pid() int                    { return p.cmd.Process.Pid }

func (p *unixPTY) Resize(cols, rows int) error {
	return cpty.Setsize(p.f, &cpty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (p *unixPTY) Wait(ctx context.Context) (int, error) {
	select {
	case <-p.done:
		return p.code, p.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Kill ends the process group (creack/pty starts the program with Setsid, so
// its group id is its pid) and closes the terminal.
func (p *unixPTY) Kill() error {
	var err error
	p.killOnce.Do(func() {
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		err = p.f.Close()
	})
	return err
}
