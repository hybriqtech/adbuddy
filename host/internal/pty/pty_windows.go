package pty

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/UserExistsError/conpty"
)

type winPTY struct {
	c        *conpty.ConPty
	killOnce sync.Once
}

// Start starts the program in a ConPTY (validated in spike S3).
func Start(o Options) (PTY, error) {
	cmd := quote(o.Path)
	for _, a := range o.Args {
		cmd += " " + quote(a)
	}
	c, err := conpty.Start(cmd, conpty.ConPtyDimensions(o.Cols, o.Rows), conpty.ConPtyWorkDir(o.Dir), conpty.ConPtyEnv(o.Env))
	if err != nil {
		return nil, err
	}
	return &winPTY{c: c}, nil
}

// quote quotes one argument for a Windows command line (CommandLineToArgvW rules).
func quote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return syscall.EscapeArg(s)
}

func (p *winPTY) Read(b []byte) (int, error)  { return p.c.Read(b) }
func (p *winPTY) Write(b []byte) (int, error) { return p.c.Write(b) }
func (p *winPTY) Resize(cols, rows int) error { return p.c.Resize(cols, rows) }
func (p *winPTY) Pid() int                    { return p.c.Pid() }

func (p *winPTY) Wait(ctx context.Context) (int, error) {
	code, err := p.c.Wait(ctx)
	if err != nil {
		return 0, err
	}
	return int(code), nil
}

// Hangup does nothing: conpty closes the pseudo console only together with
// the process and pipe handles (ConPty.Close), which would leave Wait
// without its handle. Stop waits a little and then kills the tree.
func (p *winPTY) Hangup() error { return nil }

// Kill ends the whole process tree (the agent starts node, shells…) and
// closes the console, which ends the output.
func (p *winPTY) Kill() error {
	var err error
	p.killOnce.Do(func() {
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(p.c.Pid()))
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = kill.Run() // it may have ended already
		err = p.c.Close()
	})
	if errors.Is(err, syscall.EINVAL) {
		return nil
	}
	return err
}
