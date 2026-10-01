package session

// The terminal's size (spec M3 §3.4): the last PC terminal attached rules;
// without any, the phone's if it asked; else 120×40.

// sizeLocked is the size the rules give now. Call it with mu held.
func (m *Manager) sizeLocked() (cols, rows int) {
	if n := len(m.pcs); n > 0 {
		return m.pcs[n-1].size[0], m.pcs[n-1].size[1]
	}
	if m.phone != nil {
		return m.phone[0], m.phone[1]
	}
	return DefaultCols, DefaultRows
}

// AttachPC adds a PC terminal (or updates its size); it rules the size from
// now on.
func (m *Manager) AttachPC(id string, cols, rows int) {
	m.mu.Lock()
	for i, p := range m.pcs {
		if p.id == id {
			m.pcs = append(m.pcs[:i], m.pcs[i+1:]...)
			break
		}
	}
	m.pcs = append(m.pcs, sized{id, [2]int{cols, rows}})
	m.applyLocked()
}

// ResizePC changes the size of an attached PC terminal.
func (m *Manager) ResizePC(id string, cols, rows int) {
	m.mu.Lock()
	for i := range m.pcs {
		if m.pcs[i].id == id {
			m.pcs[i].size = [2]int{cols, rows}
		}
	}
	m.applyLocked()
}

// DetachPC removes a PC terminal.
func (m *Manager) DetachPC(id string) {
	m.mu.Lock()
	for i, p := range m.pcs {
		if p.id == id {
			m.pcs = append(m.pcs[:i], m.pcs[i+1:]...)
			break
		}
	}
	m.applyLocked()
}

// ResizePhone is the size that fits the phone's view; it only rules without
// a PC terminal (protocol spec §7).
func (m *Manager) ResizePhone(cols, rows int) {
	m.mu.Lock()
	m.phone = &[2]int{cols, rows}
	m.applyLocked()
}

// applyLocked resizes the PTY and the screen to the rules' size and updates
// pcAttached. It is called with mu held and unlocks it.
func (m *Manager) applyLocked() {
	cols, rows := m.sizeLocked()
	pr := m.cur
	pcChanged := m.info.PCAttached != (len(m.pcs) > 0)
	m.info.PCAttached = len(m.pcs) > 0
	info := m.info
	m.mu.Unlock()
	if pr != nil {
		if c, r := pr.screen.Size(); c != cols || r != rows {
			pr.screen.Resize(cols, rows)
			_ = pr.p.Resize(cols, rows)
		}
	}
	if pcChanged {
		m.emitInfo(info)
	}
}
