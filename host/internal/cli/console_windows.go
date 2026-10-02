package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

// cpUTF8 is the code page of UTF-8.
const cpUTF8 = 65001

// utf8Console puts the output code page of the console on UTF-8 until the
// returned function runs, when one of files is a console: ADBuddy writes
// UTF-8 (accents, ñ, the emoji of the journal) and an old console (code page
// 850 or 1252) would draw it wrong. With no console, or when it already is
// UTF-8, it does nothing.
func utf8Console(files ...*os.File) func() {
	console := false
	for _, f := range files {
		var mode uint32
		if f != nil && windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil {
			console = true
		}
	}
	if !console {
		return func() {}
	}
	old, err := windows.GetConsoleOutputCP()
	if err != nil || old == cpUTF8 {
		return func() {}
	}
	if windows.SetConsoleOutputCP(cpUTF8) != nil {
		return func() {}
	}
	return func() { _ = windows.SetConsoleOutputCP(old) }
}
