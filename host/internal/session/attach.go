package session

import (
	"context"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
)

// attachBacklog is how many pieces of output wait for a slow PC terminal
// before it gets the whole screen again instead.
const attachBacklog = 256

// firstSize is how long attach waits for the terminal's size.
var firstSize = 2 * time.Second

// ServeAttach connects a PC terminal (adbuddy attach, IPC KindAttach) to the
// session until the terminal leaves (stdin ends) or ctx ends (spec M3 §3.4).
// The terminal first gets the current screen, then the live output; what it
// types goes to the agent, and its size rules while it is the last attached.
func (m *Manager) ServeAttach(ctx context.Context, c *ipc.Conn, id string) ipc.Exit {
	cols, rows := DefaultCols, DefaultRows
	select {
	case size := <-c.Resizes():
		cols, rows = size[0], size[1]
	case <-time.After(firstSize):
	case <-ctx.Done():
		return ipc.Exit{Code: 1}
	}
	if m.Screen() == nil {
		return ipc.Exit{Code: 1, ErrorCode: "NO_SESSION", Error: "the agent is not running: start it with `adbuddy new`"}
	}
	m.AttachPC(id, cols, rows)
	defer m.DetachPC(id)

	out := make(chan Output, attachBacklog)
	lagging := make(chan struct{}, 1)
	cancel := m.Subscribe(func(o Output) {
		select {
		case out <- o:
		default: // too slow: it gets the whole screen when it catches up
			select {
			case lagging <- struct{}{}:
			default:
			}
		}
	})
	defer cancel()

	stdinDone := make(chan struct{})
	go func() {
		defer close(stdinDone)
		buf := make([]byte, 4096)
		for {
			n, err := c.Stdin().Read(buf)
			if n > 0 {
				_ = m.Input(buf[:n]) // without an agent, typing goes nowhere
			}
			if err != nil {
				return
			}
		}
	}()

	epoch, next := "", int64(0)
	redraw := func() error {
		s := m.Screen()
		if s == nil {
			return nil
		}
		data, off, _, _ := s.Snapshot()
		epoch, next = s.Epoch(), off
		_, err := c.Stdout().Write(data)
		return err
	}
	if err := redraw(); err != nil {
		return ipc.Exit{Code: 1}
	}
	for {
		select {
		case <-ctx.Done():
			return ipc.Exit{Code: 0}
		case <-stdinDone:
			return ipc.Exit{Code: 0}
		case size := <-c.Resizes():
			m.ResizePC(id, size[0], size[1])
		case <-lagging:
			drain(out)
			if redraw() != nil {
				return ipc.Exit{Code: 1}
			}
		case o := <-out:
			if o.Epoch != epoch {
				// A new process of the agent: its whole screen first.
				if redraw() != nil {
					return ipc.Exit{Code: 1}
				}
				continue
			}
			if end := o.Offset + int64(len(o.Data)); end <= next {
				continue // already in the screen it got
			} else if o.Offset < next {
				o.Data = o.Data[next-o.Offset:]
			}
			if _, err := c.Stdout().Write(o.Data); err != nil {
				return ipc.Exit{Code: 1}
			}
			next = o.Offset + int64(len(o.Data))
		}
	}
}

func drain(ch chan Output) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
