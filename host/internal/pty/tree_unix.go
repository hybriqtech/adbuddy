//go:build !windows

package pty

import (
	"errors"

	"github.com/hybriqtech/adbuddy/host/internal/procinfo"
	"golang.org/x/sys/unix"
)

// Contains reports whether pid is the program or one of its descendants
// (V07): in the program's session (creack/pty starts it with Setsid, so the
// session id is the program's pid) or with the program up its chain of
// parents. A descendant that left both (setsid, then its parent ended) is
// not found. A process that is gone is an error.
func (p *unixPTY) Contains(pid int) (bool, error) {
	root := p.cmd.Process.Pid
	if pid == root {
		return true, nil
	}
	sid, err := unix.Getsid(pid)
	switch {
	case err == nil && sid == root:
		return true, nil
	case errors.Is(err, unix.ESRCH):
		return false, err
	}
	// Another session, or one this system does not show (EPERM): the
	// parents decide.
	for i := 0; i < 4096 && pid > 1; i++ {
		info, err := procinfo.Get(pid)
		if err != nil {
			return false, err
		}
		if info.PPID == root {
			return true, nil
		}
		pid = info.PPID
	}
	return false, nil
}
