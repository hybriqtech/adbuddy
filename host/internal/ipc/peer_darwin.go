package ipc

import (
	"net"

	"golang.org/x/sys/unix"
)

// capturePeer asks the socket who connected (LOCAL_PEERPID).
func capturePeer(nc net.Conn) Peer {
	var pid int
	var err error
	if !withFD(nc, func(fd uintptr) { pid, err = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID) }) {
		return Peer{} // not a local socket
	}
	if err != nil {
		return unverifiable(err)
	}
	return pinPeer(pid)
}
