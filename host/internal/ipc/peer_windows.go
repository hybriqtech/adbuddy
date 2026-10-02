package ipc

import (
	"errors"
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sioAFUnixGetPeerPID is SIO_AF_UNIX_GETPEERPID of afunix.h
// (_WSAIOR(IOC_VENDOR, 256)): the PID of the process that connected. It is
// not in x/sys/windows.
const sioAFUnixGetPeerPID = 0x58000100

// peerID holds the process open: while it is, Windows gives its PID to no
// other process, and the handle says whether it has ended.
type peerID struct{ h windows.Handle }

// capturePeer asks the socket who connected and opens that process.
func capturePeer(nc net.Conn) Peer {
	var pid uint32
	var err error
	if !withFD(nc, func(fd uintptr) {
		var n uint32
		err = windows.WSAIoctl(windows.Handle(fd), sioAFUnixGetPeerPID, nil, 0,
			(*byte)(unsafe.Pointer(&pid)), uint32(unsafe.Sizeof(pid)), &n, nil, 0)
	}) {
		return Peer{} // not a local socket
	}
	if err != nil {
		if ioctlUnsupported(err) {
			return Peer{} // a Windows without the ioctl does not say who
		}
		return unverifiable(err)
	}
	p := Peer{PID: int(pid), Known: true}
	if pid == 0 {
		p.Err = errors.New("the system did not name the peer's process")
		return p
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		p.Err = err
		return p
	}
	p.id.h = h
	return p
}

// ioctlUnsupported reports whether err says the system has no such ioctl, the
// only case in which it does not name the peer; any other failure is a peer
// that could not be verified.
func ioctlUnsupported(err error) bool {
	return errors.Is(err, windows.WSAEINVAL) || errors.Is(err, windows.WSAEOPNOTSUPP)
}

func (id peerID) same(int) bool {
	ev, err := windows.WaitForSingleObject(id.h, 0)
	return err == nil && ev == uint32(windows.WAIT_TIMEOUT)
}

// release closes the process handle; the server calls it when the request
// is done.
func (p *Peer) release() {
	if p.id.h != 0 {
		_ = windows.CloseHandle(p.id.h)
		p.id.h = 0
	}
}
