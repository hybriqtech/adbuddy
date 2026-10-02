package session

// Queued is how many writes wait in the agent's queue, not counting the one
// the writer is typing. For tests of callers that must know a write is queued.
func (m *Manager) Queued() int {
	pr := m.current()
	if pr == nil {
		return 0
	}
	return len(pr.q)
}
