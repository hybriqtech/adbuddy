//go:build !windows

package cli

import "os"

// utf8Console does nothing outside Windows: terminals are UTF-8 already.
func utf8Console(...*os.File) func() { return func() {} }
