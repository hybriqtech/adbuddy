//go:build !unix

package claudecode

// nonblock: outside Unix there are no FIFOs in the file system to block on.
const nonblock = 0
