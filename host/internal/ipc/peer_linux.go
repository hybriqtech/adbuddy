package ipc

import (
	"net"

	"golang.org/x/sys/unix"
)

// capturePeer asks the socket who connected (SO_PEERCRED).
func capturePeer(nc net.Conn) Peer {
	var cred *unix.Ucred
	var err error
	if !withFD(nc, func(fd uintptr) { cred, err = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) }) {
		return Peer{} // not a local socket
	}
	if err != nil {
		return unverifiable(err)
	}
	return pinPeer(int(cred.Pid))
}
