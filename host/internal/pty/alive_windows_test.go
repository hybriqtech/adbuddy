package pty

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// alive reports whether the process is running.
func alive(pid int) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	ev, err := windows.WaitForSingleObject(h, 0)
	return err == nil && ev == uint32(windows.WAIT_TIMEOUT)
}

// ownConsole gives the child its own hidden console, as node does with its
// children: closing the pseudo console does not end it, only taskkill /T.
func ownConsole(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}
