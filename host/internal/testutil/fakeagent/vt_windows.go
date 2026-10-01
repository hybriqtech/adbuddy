//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVTInput makes the console deliver keys and pastes as VT sequences.
func enableVTInput() {
	h := windows.Handle(os.Stdin.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) == nil {
		_ = windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_INPUT)
	}
}
