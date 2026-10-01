package tech.hybriq.adbuddy.bridge

/** What the app knows of the chat: M3 fills messages, M4 permissions (protocol spec §5). */
data class ChatState(
    val messages: List<ChatMessage> = emptyList(),
    val permissions: List<PermissionRequest> = emptyList(),
)

/** The chat's events (M3 and M4). Until they implement them, it changes nothing. */
object ChatReducer : Reducer {
    override val types = setOf(
        Protocol.CHAT_MESSAGE, Protocol.TOOL_ACTIVITY, Protocol.PERMISSION_REQUEST, Protocol.PERMISSION_RESOLVED,
    )

    override fun event(s: BridgeState, m: Message): BridgeState = s

    override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState = s
}
