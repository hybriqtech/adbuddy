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
	h := windows.Handle(out.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return func() {}
	}
	_ = windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	return func() { _ = windows.SetConsoleMode(h, mode) }
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
