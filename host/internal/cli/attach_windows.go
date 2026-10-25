package cli

import (
	"context"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// enableVT turns on escape sequence processing in the Windows console, so the
// agent's screen draws as it does in a terminal. It returns the restore.
func enableVT(out *os.File) func() {
	restore, _ := enableVTOK(out)
	return restore
}

// enableVTOK is enableVT that also says whether escape sequences work: false
// only when the file is a console whose mode could not be changed. A file
// that is not a console (mintty, a test buffer) already gets them as text.
func enableVTOK(out *os.File) (func(), bool) {
	if out == nil {
		return func() {}, true
	}
	h := windows.Handle(out.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return func() {}, true
	}
	if windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) != nil {
		return func() {}, false
	}
	return func() { _ = windows.SetConsoleMode(h, mode) }, true
}

// sizeChanges calls send when the console's size may have changed: Windows
// has no resize signal, so it looks every quarter of a second.
func sizeChanges(ctx context.Context, _ *os.File, send func()) {
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			send()
		}
	}
}
