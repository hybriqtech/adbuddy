package ipc

import (
	"net"
	"syscall"
)

// Peer is the process at the other end of a request's connection, as the
// system names it when the connection is accepted (V07): serve can tell a
// request of the agent's process tree without trusting what the client
// declares. It is captured before the first frame is read.
type Peer struct {
	// PID is the process the system names; 0 when Known is false.
	PID int
	// Known is false when the system does not name the peer (this
	// platform, an old Windows, a connection that is not a local socket):
	// then only what the client declares counts, as before V07.
	Known bool
	// Err says why a named process could not be pinned when the connection
	// was accepted (it had already ended, it could not be opened): such a
	// peer cannot be checked, and callers must not trust it.
	Err error

	id peerID // what tells this process from a later one with its PID
}

// Peer is the process at the other end of the request. A nil Conn (a
// handler called by a test) has an unknown peer.
func (c *Conn) Peer() Peer {
	if c == nil {
		return Peer{}
	}
	return c.peer
}

// Same reports whether the process that connected still runs and still has
// PID: false once it ended, even if another process got its PID, and false
// for a peer that is not Known or has Err. Only meaningful while the request
// is being handled: on Windows it waits on the process handle the server
// closes when the handler returns, and after that its answer means nothing.
func (p Peer) Same() bool {
	if !p.Known || p.Err != nil {
		return false
	}
	return p.id.same(p.PID)
}

// withFD runs f with the connection's socket. It reports false when nc has
// none (a net.Pipe) or it cannot be reached.
func withFD(nc net.Conn, f func(fd uintptr)) bool {
	sc, ok := nc.(syscall.Conn)
	if !ok {
		return false
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return false
	}
	return raw.Control(f) == nil
}
