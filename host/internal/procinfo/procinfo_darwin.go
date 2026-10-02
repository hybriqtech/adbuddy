package procinfo

import (
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// szomb is SZOMB of <sys/proc.h>: ended, not collected by its parent.
const szomb = 5

// Get asks sysctl kern.proc.pid.<pid>.
func Get(pid int) (Info, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return Info{}, err // no such process: EIO from x/sys (an empty answer)
	}
	if int(kp.Proc.P_pid) != pid {
		return Info{}, syscall.ESRCH
	}
	t := kp.Proc.P_starttime
	return Info{
		PPID:   int(kp.Eproc.Ppid),
		Start:  strconv.FormatInt(int64(t.Sec), 10) + "." + strconv.FormatInt(int64(t.Usec), 10),
		Zombie: kp.Proc.P_stat == szomb,
	}, nil
}
