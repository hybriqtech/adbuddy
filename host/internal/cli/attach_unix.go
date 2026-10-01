//go:build !windows

package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// enableVT is a no-op: Unix terminals already process escape sequences.
func enableVT(*os.File) func() { return func() {} }

// sizeChanges calls send on each SIGWINCH.
func sizeChanges(ctx context.Context, _ *os.File, send func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	defer signal.Stop(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			send()
		}
	}
}
