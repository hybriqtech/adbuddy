package pty

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// maxJobPIDs bounds the processes Contains lists in the job: an agent with
// more is an error, never a "not in the tree".
const maxJobPIDs = 4096

// jobPIDList is JOBOBJECT_BASIC_PROCESS_ID_LIST with room for maxJobPIDs.
type jobPIDList struct {
	assigned, listed uint32
	ids              [maxJobPIDs]uintptr
}

// Contains reports whether pid is the program or one of its descendants
// (V07): a process in its job object, which nothing the program starts can
// leave (V25). Without a job (it could not be made, or Kill closed it) the
// tree is not known: errors.ErrUnsupported.
func (p *winPTY) Contains(pid int) (bool, error) {
	p.jmu.Lock()
	defer p.jmu.Unlock()
	if p.job == 0 {
		return false, errors.ErrUnsupported
	}
	l := new(jobPIDList)
	if err := windows.QueryInformationJobObject(p.job, windows.JobObjectBasicProcessIdList,
		uintptr(unsafe.Pointer(l)), uint32(unsafe.Sizeof(*l)), nil); err != nil {
		return false, err
	}
	if l.listed < l.assigned {
		return false, errors.New("the job holds more processes than were listed")
	}
	for _, id := range l.ids[:l.listed] {
		if int(id) == pid {
			return true, nil
		}
	}
	return false, nil
}
