//go:build unix

package safeio

import (
	"io/fs"
	"os"
	"syscall"
)

// ReadFlags and AppendFlags open without blocking: opening a FIFO otherwise
// waits for the other end.
const (
	ReadFlags   = os.O_RDONLY | syscall.O_NONBLOCK
	AppendFlags = os.O_RDWR | os.O_APPEND | os.O_CREATE | syscall.O_NONBLOCK
)

// linkCount is the number of names of the file (st_nlink).
func linkCount(_ *os.File, st fs.FileInfo) (uint64, error) {
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		return uint64(s.Nlink), nil
	}
	return 1, nil
}
