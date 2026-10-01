// Package pty runs the agent in a pseudo-terminal (spec M3 §3.1): ConPTY on
// Windows, a Unix pty elsewhere. Both are pure Go.
package pty

import (
	"context"
	"io"
)

// PTY is a process running in a pseudo-terminal. Read returns its output,
// Write types into it.
type PTY interface {
	io.ReadWriter
	// Resize changes the terminal's size; the program gets a resize signal.
	Resize(cols, rows int) error
	// Wait waits for the process to end and returns its exit code.
	Wait(ctx context.Context) (int, error)
	// Hangup asks the program to end, as when its terminal closes: SIGHUP
	// to its process group on Unix. It does not wait. On Windows it does
	// nothing: conpty only closes the pseudo console together with the
	// process handles, which is Kill's job.
	Hangup() error
	// Kill ends the process and its children, and releases the terminal.
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
