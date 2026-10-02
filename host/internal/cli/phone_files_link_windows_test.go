//go:build windows

package cli

import (
	"fmt"
	"os/exec"
)

// dirLink makes link a junction (mklink /J) to target: what an agent can make
// on Windows without privileges. It returns an error instead of failing the
// test, so that a goroutine of the host (a hook) can call it. Whoever calls it
// removes link with os.Remove (the junction only, never what it points to)
// before the cleanup of t.TempDir walks the folder.
func dirLink(link, target string) error {
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		return fmt.Errorf("mklink /J: %v %s", err, out)
	}
	return nil
}
