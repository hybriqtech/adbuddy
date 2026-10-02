//go:build !linux && !windows

package pty

import (
	"os"

	cpty "github.com/creack/pty"
)

// pollableMaster is only done on Linux: on the other Unixes the master stays
// as creack/pty leaves it (see the plan's manual validations for macOS).
func pollableMaster(f *os.File) *os.File { return f }

func (p *unixPTY) setSize(cols, rows int) error {
	return cpty.Setsize(p.f, &cpty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
