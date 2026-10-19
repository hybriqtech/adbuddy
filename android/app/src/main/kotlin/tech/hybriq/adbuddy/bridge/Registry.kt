package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.KSerializer

/**
 * Each message type with its payload, by direction. Mirrors HostPayloads and
 * AppPayloads in host/internal/bridge/registry.go; FixturesTest checks both
 * against protocol/fixtures/.
 */
object Registry {
    /** What the host sends. */
    val HOST: Map<String, KSerializer<*>> = mapOf(
        Protocol.WELCOME to Welcome.serializer(),
        Protocol.SNAPSHOT to Snapshot.serializer(),
        Protocol.SESSION_STATE to SessionState.serializer(),
        Protocol.CONFIRM_REQUEST to ConfirmRequest.serializer(),
        Protocol.JOURNAL_ENTRY to JournalEntry.serializer(),
        Protocol.NOTICE to Notice.serializer(),
        Protocol.ERROR to ErrorReply.serializer(),
        Protocol.CHAT_MESSAGE to ChatMessage.serializer(),
        Protocol.TOOL_ACTIVITY to ToolActivity.serializer(),
        Protocol.PERMISSION_REQUEST to PermissionRequest.serializer(),
        Protocol.PERMISSION_RESOLVED to PermissionResolved.serializer(),
        Protocol.ATTACH_READY to AttachReady.serializer(),
        Protocol.ATTACH_DONE to AttachDone.serializer(),
        Protocol.FILE_END to FileEnd.serializer(),
        Protocol.CAPTURE_READY to CaptureReady.serializer(),
        Protocol.TERMINAL_SNAPSHOT to TerminalSnapshot.serializer(),
        Protocol.TERMINAL_OUTPUT to TerminalOutput.serializer(),
        Protocol.TERMINAL_SIZE to TerminalSize.serializer(),
        Protocol.UI_REQUEST to UiRequest.serializer(),
        Protocol.REMOTE_STATE to RemoteState.serializer(),
        Protocol.YOLO_STATE to YoloState.serializer(),
    )

    /** What the app sends. */
    val APP: Map<String, KSerializer<*>> = mapOf(
        Protocol.HELLO to Hello.serializer(),
        Protocol.CONFIRM_REPLY to ConfirmReply.serializer(),
        Protocol.PAUSE to Empty.serializer(),
        Protocol.RESUME to Empty.serializer(),
        Protocol.CHAT_SEND to ChatSend.serializer(),
        Protocol.SESSION_NEW to SessionNew.serializer(),
        Protocol.SESSION_STOP to Empty.serializer(),
        Protocol.AGENT_INTERRUPT to Empty.serializer(),
        Protocol.PERMISSION_REPLY to PermissionReply.serializer(),
        Protocol.ATTACH_BEGIN to AttachBegin.serializer(),
        Protocol.ATTACH_END to AttachEnd.serializer(),
        Protocol.FILE_GET to FileGet.serializer(),
        Protocol.FILE_CANCEL to FileCancel.serializer(),
        Protocol.CAPTURE_REQUEST to Empty.serializer(),
        Protocol.TERMINAL_WATCH to TerminalWatch.serializer(),
        Protocol.TERMINAL_UNWATCH to Empty.serializer(),
        Protocol.TERMINAL_INPUT to TerminalInput.serializer(),
        Protocol.TERMINAL_RESIZE to TerminalResize.serializer(),
        Protocol.UI_RESPONSE to UiResponse.serializer(),
        Protocol.DEVICE_INFO to DeviceInfo.serializer(),
        Protocol.REMOTE_SET to RemoteSet.serializer(),
        Protocol.YOLO_SET to YoloSet.serializer(),
    )
}
