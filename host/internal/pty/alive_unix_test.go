//go:build !windows

package pty

import (
	"bytes"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// alive reports whether the process is running: a zombie waiting for its
// new parent to reap it (Linux) is not.
func alive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true // no /proc (macOS)
	}
	i := bytes.LastIndexByte(stat, ')')
	return i < 0 || i+2 >= len(stat) || stat[i+2] != 'Z'
}

// ownConsole does nothing on Unix: the child stays in the program's process
// group, which Kill ends.
func ownConsole(c *exec.Cmd) {}
