//go:build !unix && !windows

package safeio

import (
	"io/fs"
	"os"
)

// ReadFlags and AppendFlags: no FIFOs to block on here.
const (
	ReadFlags   = os.O_RDONLY
	AppendFlags = os.O_RDWR | os.O_APPEND | os.O_CREATE
)

// linkCount: these systems do not say; one name.
func linkCount(*os.File, fs.FileInfo) (uint64, error) { return 1, nil }
