package ipc

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

func TestIoctlUnsupported(t *testing.T) {
	for _, e := range []error{windows.WSAEINVAL, windows.WSAEOPNOTSUPP} {
		if !ioctlUnsupported(e) {
			t.Errorf("%v should mean no PID from the platform", e)
		}
	}
	for _, e := range []error{windows.WSAEACCES, windows.WSAENOTSOCK, errors.New("x")} {
		if ioctlUnsupported(e) {
			t.Errorf("%v must be an unverifiable peer", e)
		}
	}
}
