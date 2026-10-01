package tech.hybriq.adbuddy.bridge

/** What the app knows of the chat: M3 fills messages, M4 permissions (protocol spec §5). */
data class ChatState(
    val messages: List<ChatMessage> = emptyList(),
    val permissions: List<PermissionRequest> = emptyList(),
)

/** The chat's events: M3 fills the messages; permissions and tool activity are M4's. */
object ChatReducer : Reducer {
    /** What the host keeps and sends in a snapshot (spec M3 §5.3). */
    const val MAX_MESSAGES = 100

    override val types = setOf(
        Protocol.CHAT_MESSAGE, Protocol.TOOL_ACTIVITY, Protocol.PERMISSION_REQUEST, Protocol.PERMISSION_RESOLVED,
    )

    override fun event(s: BridgeState, m: Message): BridgeState = when (m.type) {
        Protocol.CHAT_MESSAGE -> {
            val msg = Protocol.payload<ChatMessage>(m)
            if (msg == null || msg.id.isEmpty()) s else s.copy(chat = s.chat.copy(messages = upsert(s.chat.messages, msg)))
        }
        else -> s
    }

    /** One with an id already seen replaces it where it was (spec protocol §5.1); a new one goes last. */
    internal fun upsert(list: List<ChatMessage>, msg: ChatMessage): List<ChatMessage> {
        val i = list.indexOfFirst { it.id == msg.id }
        return if (i >= 0) list.toMutableList().also { it[i] = msg } else (list + msg).takeLast(MAX_MESSAGES)
    }

    override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState {
        val messages = snap.chat.mapNotNull {
            try {
                Protocol.json.decodeFromJsonElement(ChatMessage.serializer(), it).takeIf { m -> m.id.isNotEmpty() }
            } catch (e: IllegalArgumentException) {
                null
            } catch (e: StackOverflowError) {
                null
            }
        }
        // A repeated id keeps its last version, at the place of its last appearance.
        val unique = messages.asReversed().distinctBy { it.id }.asReversed()
        return s.copy(chat = s.chat.copy(messages = unique.takeLast(MAX_MESSAGES)))
    }

    override fun reset(s: BridgeState, why: Reset): BridgeState = when (why) {
        Reset.NEW_SESSION, Reset.UNREADABLE_SNAPSHOT -> s.copy(chat = ChatState())
        Reset.DISCONNECTED, Reset.INCOMPATIBLE -> s
    }
}
