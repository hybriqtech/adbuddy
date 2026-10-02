//go:build unix

package claudecode

import "syscall"

// nonblock makes the open of a FIFO return at once instead of waiting for a
// writer (the agent can swap the transcript for one between Lstat and open).
const nonblock = syscall.O_NONBLOCK
