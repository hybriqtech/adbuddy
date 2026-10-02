//go:build !linux && !darwin && !windows

package ipc

import "net"

// peerID: these systems do not name the peer.
type peerID struct{}

func capturePeer(net.Conn) Peer { return Peer{} }

func (peerID) same(int) bool { return false }

func (p *Peer) release() {}
