//go:build linux || darwin

package ipc

import (
	"errors"

	"github.com/hybriqtech/adbuddy/host/internal/procinfo"
)

// peerID is the process's start time: a later process with the same PID
// does not share it.
type peerID struct{ start string }

// pinPeer names the process pid by its start time too.
func pinPeer(pid int) Peer {
	p := Peer{PID: pid, Known: true}
	if pid <= 0 {
		p.Err = errors.New("the system did not name the peer's process")
		return p
	}
	info, err := procinfo.Get(pid)
	switch {
	case err != nil:
		p.Err = err
	case info.Zombie:
		p.Err = errors.New("the peer's process has ended")
	default:
		p.id.start = info.Start
	}
	return p
}

func (id peerID) same(pid int) bool {
	info, err := procinfo.Get(pid)
	return err == nil && !info.Zombie && info.Start == id.start
}

func (p *Peer) release() {}
