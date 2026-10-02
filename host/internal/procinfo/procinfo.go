// Package procinfo reads what serve needs of another process to tell
// whether it belongs to the agent (V07): its parent, and its start time,
// which a later process that reuses its PID does not share.
package procinfo

// Info is a process as the system describes it now.
type Info struct {
	PPID   int    // its parent's PID
	Start  string // when it started, in the system's own units: only compared
	Zombie bool   // it has ended and its parent has not collected it yet
}
