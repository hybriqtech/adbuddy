package cli

// chatArea is the chat (M3, protocol spec §5-§7): session, chat.send,
// attachments, files, captures and the terminal. M3 fills it.
func (l *phoneLink) chatArea() area { return area{} }
