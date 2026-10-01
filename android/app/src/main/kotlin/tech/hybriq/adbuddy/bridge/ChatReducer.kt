package tech.hybriq.adbuddy.bridge

/** What the app knows of the chat: M3 fills messages, M4 permissions (protocol spec §5). */
data class ChatState(
    val messages: List<ChatMessage> = emptyList(),
    val permissions: List<PermissionRequest> = emptyList(),
    /** What the agent does with its tools (tool.activity, M4): the last [ChatReducer.MAX_TOOLS], oldest first. Not in a snapshot. */
    val tools: List<ToolActivity> = emptyList(),
)

/** The chat's events: M3 fills the messages; M4 the tool activity and the agent's permission questions. */
object ChatReducer : Reducer {
    /** What the host keeps and sends in a snapshot (spec M3 §5.3). */
    const val MAX_MESSAGES = 100

    /** Tool activity and permission questions kept (M4); the host holds at most 8 questions. */
    const val MAX_TOOLS = 200
    const val MAX_PERMISSIONS = 16

    override val types = setOf(
        Protocol.CHAT_MESSAGE, Protocol.TOOL_ACTIVITY, Protocol.PERMISSION_REQUEST, Protocol.PERMISSION_RESOLVED,
    )

    override fun event(s: BridgeState, m: Message): BridgeState = when (m.type) {
        Protocol.CHAT_MESSAGE -> {
            val msg = Protocol.payload<ChatMessage>(m)
            if (msg == null || msg.id.isEmpty()) s else s.copy(chat = s.chat.copy(messages = upsert(s.chat.messages, msg)))
        }
        Protocol.TOOL_ACTIVITY -> {
            val t = Protocol.payload<ToolActivity>(m)
            if (t == null || t.id.isEmpty()) s else s.copy(chat = s.chat.copy(tools = upsertBy(s.chat.tools, t, MAX_TOOLS) { it.id }))
        }
        Protocol.PERMISSION_REQUEST -> {
            val p = Protocol.payload<PermissionRequest>(m)
            if (p == null || p.id.isEmpty()) s else s.copy(chat = s.chat.copy(permissions = upsertBy(s.chat.permissions, p, MAX_PERMISSIONS) { it.id }))
        }
        // Whoever answered, and wherever: the card goes (spec M4 §5.1).
        Protocol.PERMISSION_RESOLVED -> {
            val r = Protocol.payload<PermissionResolved>(m)
            if (r == null) s else s.copy(chat = s.chat.copy(permissions = s.chat.permissions.filterNot { it.id == r.id }))
        }
        else -> s
    }

    private fun <T> upsertBy(list: List<T>, item: T, max: Int, id: (T) -> String): List<T> {
        val i = list.indexOfFirst { id(it) == id(item) }
        return if (i >= 0) list.toMutableList().also { it[i] = item } else (list + item).takeLast(max)
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
        // The permissions are the host's pending ones: they replace what was known, a bad one skipped (an area reads its part alone, protocol spec §4.3).
        val permissions = snap.permissions.mapNotNull {
            try {
                Protocol.json.decodeFromJsonElement(PermissionRequest.serializer(), it).takeIf { p -> p.id.isNotEmpty() }
            } catch (e: IllegalArgumentException) {
                null
            } catch (e: StackOverflowError) {
                null
            }
        }.distinctBy { it.id }.takeLast(MAX_PERMISSIONS)
        return s.copy(chat = s.chat.copy(messages = unique.takeLast(MAX_MESSAGES), permissions = permissions))
    }

    override fun reset(s: BridgeState, why: Reset): BridgeState = when (why) {
        Reset.NEW_SESSION, Reset.UNREADABLE_SNAPSHOT -> s.copy(chat = ChatState())
        // A card stays while the connection is down (its buttons are off, ChatParts): the events the host resends, or a new snapshot, take it away if it was answered meanwhile.
        Reset.DISCONNECTED, Reset.INCOMPATIBLE -> s
    }
}
