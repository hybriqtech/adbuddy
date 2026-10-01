package cli

import (
	"context"
	"io"
	"sync"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
)

// activePhone is the phone serve works with. Its serial changes when the
// phone is found again at another address (M5b spec §7.2), so nobody keeps
// the Runner: they hold the activePhone and every command asks it.
type activePhone struct {
	mu      sync.Mutex
	runner  adb.Runner
	serials []string // every serial used this session, current first
}

func newActivePhone(r adb.Runner) *activePhone {
	return &activePhone{runner: r, serials: []string{r.Serial}}
}

// Runner is a copy of the current runner: a command that already started
// keeps the serial it started with.
func (p *activePhone) Runner() adb.Runner {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.runner
}

func (p *activePhone) Serial() string { return p.Runner().Serial }

func (p *activePhone) Switch(serial string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if serial == p.runner.Serial {
		return
	}
	p.runner.Serial = serial
	rest := p.serials[:0:0]
	for _, s := range p.serials {
		if s != serial {
			rest = append(rest, s)
		}
	}
	p.serials = append([]string{serial}, rest...)
}

// Serials is every serial this session has used, the current one first.
func (p *activePhone) Serials() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.serials...)
}

func (p *activePhone) Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	return p.Runner().Run(ctx, dir, args, stdin, stdout, stderr)
}

func (p *activePhone) Shell(ctx context.Context, c string) (string, int, error) {
	return p.Runner().Shell(ctx, c)
}

func (p *activePhone) Pull(ctx context.Context, remote, local string) error {
	return p.Runner().Pull(ctx, remote, local)
}

func (p *activePhone) Output(ctx context.Context, args ...string) (string, string, int, error) {
	return p.Runner().Output(ctx, args...)
}
