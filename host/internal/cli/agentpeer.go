package cli

import (
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/session"
)

// fromAgent reports whether a request to serve comes from the agent's
// session, so that attach, new, stop and remote refuse it (V07). The agent's
// own declaration (ADBUDDY_SOURCE=agent) only ever adds a refusal. What
// decides is the process at the other end of the socket: in the agent's
// process tree, or named by the system but no longer checkable (it ended,
// its PID went to another process, it could not be opened), it is the
// agent. Only when the system does not name it, or the agent's tree is not
// known (no job object on Windows), does the declaration alone decide.
//
// Not a guarantee (spec §4.6): a process the agent detaches from its tree
// is not seen (setsid -f, systemd-run, tmux, at/cron, launchd; WMI,
// schtasks or explorer/COM on Windows).
//
// Call it only from the IPC handler, while the request is being answered: on
// Windows the process handle Peer.Same needs is closed when the handler
// returns, and after that its answer means nothing.
func fromAgent(sess *session.Manager, req ipc.Request, c *ipc.Conn) bool {
	return peerIsAgent(sess, req, c.Peer())
}

// peerIsAgent is fromAgent with the peer already read from the connection.
func peerIsAgent(sess *session.Manager, req ipc.Request, p ipc.Peer) bool {
	if req.Env["ADBUDDY_SOURCE"] == "agent" {
		return true
	}
	switch {
	case !p.Known:
		return false
	case p.Err != nil:
		return true
	case p.PID <= 0:
		// Named but not a process that can be checked; and the tree must
		// never be asked about it (on Unix getsid(0) is serve's own
		// session, a negative PID an error).
		return true
	}
	in, known := false, true
	if sess != nil {
		in, known = sess.InAgentTree(p.PID)
	}
	if !p.Same() {
		return true // it ended, or its PID went to another process, meanwhile
	}
	return known && in
}
