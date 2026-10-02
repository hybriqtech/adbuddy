//go:build unix

package cli

import "os"

// dirLink makes link a symbolic link to target (see the Windows version).
func dirLink(link, target string) error {
	return os.Symlink(target, link)
}
