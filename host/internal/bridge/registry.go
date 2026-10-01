package bridge

import "github.com/hybriqtech/adbuddy/host/internal/journal"

// HostPayloads maps each message type the host sends to a new value of its
// payload. Every type has a fixture in protocol/fixtures/host/, checked by
// TestFixtures (protocol spec §12).
var HostPayloads = map[string]func() any{
	TypeWelcome:          func() any { return new(Welcome) },
	TypeSnapshot:         func() any { return new(Snapshot) },
	TypeSessionState:     func() any { return new(SessionState) },
	TypeConfirmReq:       func() any { return new(ConfirmRequest) },
	TypeJournalEntry:     func() any { return new(journal.Entry) },
	TypeNotice:           func() any { return new(Notice) },
	TypeError:            func() any { return new(ErrorReply) },
	TypeChatMessage:      func() any { return new(ChatMessage) },
	TypeToolActivity:     func() any { return new(ToolActivity) },
	TypePermRequest:      func() any { return new(PermissionRequest) },
	TypePermResolved:     func() any { return new(PermissionResolved) },
	TypeAttachReady:      func() any { return new(AttachReady) },
	TypeAttachDone:       func() any { return new(AttachDone) },
	TypeFileEnd:          func() any { return new(FileEnd) },
	TypeCaptureReady:     func() any { return new(CaptureReady) },
	TypeTerminalSnapshot: func() any { return new(TerminalSnapshot) },
	TypeTerminalOutput:   func() any { return new(TerminalOutput) },
	TypeTerminalSize:     func() any { return new(TerminalSize) },
	TypeUIRequest:        func() any { return new(UIRequest) },
	TypeRemoteState:      func() any { return new(RemoteState) },
}

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
}

// AppPayloads maps each message type the app sends to a new value of its
// payload. Every type has a fixture in protocol/fixtures/app/.
var AppPayloads = map[string]func() any{
	TypeHello:           func() any { return new(Hello) },
	TypeConfirmReply:    func() any { return new(ConfirmReply) },
	TypePause:           func() any { return new(Empty) },
	TypeResume:          func() any { return new(Empty) },
	TypeChatSend:        func() any { return new(ChatSend) },
	TypeSessionNew:      func() any { return new(SessionNew) },
	TypeSessionStop:     func() any { return new(Empty) },
	TypeInterrupt:       func() any { return new(Empty) },
	TypePermReply:       func() any { return new(PermissionReply) },
	TypeAttachBegin:     func() any { return new(AttachBegin) },
	TypeAttachEnd:       func() any { return new(AttachEnd) },
	TypeFileGet:         func() any { return new(FileGet) },
	TypeFileCancel:      func() any { return new(FileCancel) },
	TypeCaptureReq:      func() any { return new(Empty) },
	TypeTerminalWatch:   func() any { return new(TerminalWatch) },
	TypeTerminalUnwatch: func() any { return new(Empty) },
	TypeTerminalInput:   func() any { return new(TerminalInput) },
	TypeTerminalResize:  func() any { return new(TerminalResize) },
	TypeUIResponse:      func() any { return new(UIResponse) },
	TypeDeviceInfo:      func() any { return new(DeviceInfo) },
	TypeRemoteSet:       func() any { return new(RemoteSet) },
}
