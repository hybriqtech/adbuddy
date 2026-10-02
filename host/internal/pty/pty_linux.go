//go:build linux

package pty

import (
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// pollableMaster gives back the pty master as a non-blocking os.File that
// Go's poller owns. creack/pty calls f.Fd(), which puts the master in
// blocking mode; then a Write that the agent's unread input blocks is a plain
// write(2) that Close does not interrupt. With the poller, Close evicts it:
// a blocked Write or Read returns os.ErrClosed at once (Kill relies on it).
// Anything that goes wrong returns f as it was: never a non-blocking master
// that the poller does not serve (Read would fail with EAGAIN).
func pollableMaster(f *os.File) *os.File {
	// Atomic close-on-exec: Dup then CloseOnExec would leave a window where a
	// concurrent exec (the host runs adb) inherits the master and keeps the
	// pty alive. This is the last Fd() call on f, which is closed below.
	fd, err := unix.FcntlInt(f.Fd(), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return f
	}
	// O_NONBLOCK belongs to the open file description, shared by f and the dup.
	if err := syscall.SetNonblock(fd, true); err != nil {
		_ = syscall.Close(fd)
		return f
	}
	nf := os.NewFile(uintptr(fd), f.Name())
	if nf == nil {
		_ = syscall.SetNonblock(fd, false)
		_ = syscall.Close(fd)
		return f
	}
	// os.ErrNoDeadline: the file is not in the poller.
	if err := nf.SetWriteDeadline(time.Time{}); err != nil {
		_ = syscall.SetNonblock(fd, false) // restore the shared description for f
		_ = nf.Close()
		return f
	}
	_ = f.Close() // the old fd only: the description lives on in nf
	return nf
}

// setSize changes the terminal's size without Fd(), which would put the
// master back in blocking mode.
func (p *unixPTY) setSize(cols, rows int) error {
	sc, err := p.f.SyscallConn()
	if err != nil {
		return err
	}
	var cerr error
	if err := sc.Control(func(fd uintptr) {
		cerr = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &unix.Winsize{Row: uint16(rows), Col: uint16(cols)})
	}); err != nil {
		return err
	}
	return cerr
}
