package safeio

import (
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

// ReadFlags and AppendFlags: on Windows there are no FIFOs in the file
// system to block on.
const (
	ReadFlags   = os.O_RDONLY
	AppendFlags = os.O_RDWR | os.O_APPEND | os.O_CREATE
)

// linkCount is the number of names of the file (NumberOfLinks).
func linkCount(f *os.File, _ fs.FileInfo) (uint64, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return 0, err
	}
	return uint64(info.NumberOfLinks), nil
}
