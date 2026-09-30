package pairing

import (
	"strings"

	"rsc.io/qr"
)

// RenderQR draws text as a QR code for the terminal, two modules per
// character with half blocks. With color it forces black on white, which
// scans on dark and light terminals; without color the dark modules are
// drawn as blocks, which scans on light terminals.
func RenderQR(text string, color bool) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	const quiet = 2
	size := code.Size + 2*quiet
	dark := func(x, y int) bool { return code.Black(x-quiet, y-quiet) }
	var b strings.Builder
	for y := 0; y < size; y += 2 {
		if color {
			b.WriteString("\x1b[30;47m")
		}
		for x := 0; x < size; x++ {
			top, bottom := dark(x, y), y+1 < size && dark(x, y+1)
			switch {
			case top && bottom:
				b.WriteString("█")
			case top:
				b.WriteString("▀")
			case bottom:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		if color {
			b.WriteString("\x1b[0m")
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
