package tech.hybriq.adbuddy.chat

import tech.hybriq.adbuddy.bridge.ChatMessage
import tech.hybriq.adbuddy.bridge.ChatState

sealed interface TimelineItem {
    /** Unique and stable: the LazyColumn key. */
    val key: String

    class Host(val msg: ChatMessage) : TimelineItem { override val key = "h" + msg.id }
    class Mine(val out: Outgoing) : TimelineItem { override val key = "m" + out.id }
}

/** What the conversation shows, oldest first (spec M3 §7.2). Pure. */
object Timeline {
    /**
     * conversation: the one of session.state ("" while unknown: all show). The
     * host publishes nothing that empties the chat on session.new, so the
     * messages of other conversations are filtered here; one without a
     * conversation is always shown.
     */
    fun of(chat: ChatState, conversation: String, outgoing: List<Outgoing>): List<TimelineItem> {
        val shown = chat.messages.filter { conversation.isEmpty() || it.conversation.isEmpty() || it.conversation == conversation }
        val known = chat.messages.mapTo(HashSet()) { it.id }
        return shown.map { TimelineItem.Host(it) } + outgoing.filter { it.id !in known }.map { TimelineItem.Mine(it) }
    }
}
