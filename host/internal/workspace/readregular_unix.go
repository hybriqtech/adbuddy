//go:build unix

package workspace

import (
	"os"
	"syscall"
)

// openFlags opens without blocking: opening a FIFO for reading otherwise
// waits for a writer.
const openFlags = os.O_RDONLY | syscall.O_NONBLOCK
