package cli

// agentArea is the Claude Code adapter (M4, protocol spec §5): the agent's
// messages, its tool activity and its permission requests. M4 fills it.
func (l *phoneLink) agentArea() area { return area{} }
