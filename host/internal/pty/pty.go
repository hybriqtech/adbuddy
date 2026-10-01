// Package pty runs the agent in a pseudo-terminal (spec M3 §3.1): ConPTY on
// Windows, a Unix pty elsewhere. Both are pure Go.
package pty

import (
	"context"
	"io"
	"sync/atomic"
	"time"
)

// PTY is a process running in a pseudo-terminal. Write types into it.
type PTY interface {
	// Read returns the program's output. It ends with io.EOF when the
	// program ends: once the output has been quiet for a moment (drainQuiet,
	// drainMax at most), the terminal closes and Read returns what was left
	// and then io.EOF, without a Kill. On Windows that also ends what the
	// program left attached to the console. On Unix, where closing the
	// terminal may not wake a Read (macOS), a child of the program that keeps
	// the terminal open can hold Read until Kill: callers must not depend on
	// Read alone and may Kill once Wait returned.
	io.Reader
	io.Writer
	// Resize changes the terminal's size; the program gets a resize signal.
	Resize(cols, rows int) error
	// Wait waits for the process to end and returns its exit code, the same
	// on every call, also after Kill.
	Wait(ctx context.Context) (int, error)
	// Hangup asks the program to end, as when its terminal closes: SIGHUP
	// to its process group on Unix, nothing once it has ended. It does not
	// wait. On Windows it does nothing: a pseudo console has no hangup the
	// program could catch, and closing it ends the program at once.
	Hangup() error
	// Kill ends the process and its children, and releases the terminal:
	// Read returns io.EOF.
	Kill() error
	// Pid is the process id of the program.
	Pid() int
}

// Options start a program in a PTY.
type Options struct {
	Path string   // absolute path of the program (exec.LookPath)
	Args []string // without the program itself
	Dir  string
	Env  []string
	Cols int
	Rows int
}

// After the program ends its last output may still be on its way (conhost
// paints on a timer): the terminal closes once the output has been quiet
// for drainQuiet, and drainMax after the end at the latest.
const (
	drainQuiet = 200 * time.Millisecond
	drainMax   = 2 * time.Second
)

// drain waits, from the program's end, until the output has been quiet for
// drainQuiet or drainMax has passed. last is when output was last read
// (UnixNano).
func drain(last *atomic.Int64) {
	ended := time.Now()
	limit := ended.Add(drainMax)
	for {
		quiet := ended
		if l := time.Unix(0, last.Load()); l.After(quiet) {
			quiet = l
		}
		wake := quiet.Add(drainQuiet)
		if wake.After(limit) {
			wake = limit
		}
		d := time.Until(wake)
		if d <= 0 {
			return
		}
		time.Sleep(d)
	}
}
