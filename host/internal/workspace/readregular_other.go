//go:build !unix

package workspace

import "os"

// openFlags: outside Unix there are no FIFOs in the file system to block on.
const openFlags = os.O_RDONLY
