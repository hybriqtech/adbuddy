package session

import "errors"

// treeTeller is a PTY that can tell whether a process is the program or one
// of its descendants: pty.Start's PTYs; the fakes of tests need not be.
type treeTeller interface {
	Contains(pid int) (bool, error)
}

// InAgentTree reports whether the process pid is the agent or one of its
// descendants (V07). known is false when that cannot be told: a PTY that
// cannot say, or one without a record of its tree (errors.ErrUnsupported:
// a Windows agent without a job object, a system whose processes are not
// read). Any other error counts as in the tree: what cannot be checked is
// refused. Without an agent running nothing is in its tree.
func (m *Manager) InAgentTree(pid int) (in, known bool) {
	pr := m.current()
	if pr == nil {
		return false, true
	}
	t, ok := pr.p.(treeTeller)
	if !ok {
		return false, false
	}
	in, err := t.Contains(pid)
	switch {
	case errors.Is(err, errors.ErrUnsupported):
		return false, false
	case err != nil:
		return true, true
	}
	return in, true
}
