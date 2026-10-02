//go:build unix

package cli

import "syscall"

// fifosExist: Unix file systems have FIFOs.
const fifosExist = true

func makeFIFO(p string) error { return syscall.Mkfifo(p, 0o600) }
