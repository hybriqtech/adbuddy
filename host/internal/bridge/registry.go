package bridge

// HostClasses gives the class of each type the host sends (protocol spec
// §3). Publish, Reply and Send refuse a type of another class, and
// TestFixtureClasses checks each fixture's seq and rid against it.
var HostClasses = map[string]Class{
	TypeWelcome:          ClassHandshake,
	TypeSnapshot:         ClassEvent,
	TypeSessionState:     ClassEvent,
	TypeConfirmReq:       ClassEvent,
	TypeJournalEntry:     ClassEvent,
	TypeNotice:           ClassEvent,
	TypeError:            ClassReply,
	TypeChatMessage:      ClassEvent,
	TypeToolActivity:     ClassEvent,
	TypePermRequest:      ClassEvent,
	TypePermResolved:     ClassEvent,
	TypeAttachReady:      ClassReply,
	TypeAttachDone:       ClassReply,
	TypeFileEnd:          ClassReply,
	TypeCaptureReady:     ClassReply,
	TypeTerminalSnapshot: ClassStream,
	TypeTerminalOutput:   ClassStream,
	TypeTerminalSize:     ClassStream,
	TypeUIRequest:        ClassStream,
	TypeRemoteState:      ClassEvent,
	TypeYoloState:        ClassEvent,
}
