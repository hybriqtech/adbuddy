package tech.hybriq.adbuddy.notify

import androidx.annotation.StringRes
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ChatState
import tech.hybriq.adbuddy.ui.LinkStatus
import tech.hybriq.adbuddy.ui.StatusView

data class LastMessage(val id: String, val fromUser: Boolean, val text: String)

/** A string resource and its one argument, so the text is chosen where there is a Context and tested here without one. */
data class Headline(@param:StringRes val res: Int, val arg: String? = null)

/**
 * Everything the conversation notification shows. A data class so the service
 * posts the notification again only when it changes (like StatusView in M2).
 */
data class ConversationView(
    val status: StatusView,
    val agent: String,
    val state: String,
    val activity: String,
    val last: LastMessage?,
) {
    fun headline(): Headline = when {
        status.status != LinkStatus.CONNECTED -> Headline(status.status.text)
        state == "working" && activity.isNotEmpty() -> Headline(R.string.notif_working_on, activity)
        state == "working" -> Headline(R.string.notif_working)
        state == "waiting" -> Headline(R.string.notif_waiting)
        state == "stopped" -> Headline(R.string.notif_stopped)
        else -> Headline(R.string.status_connected, status.host)
    }

    /**
     * What the lock screen may show: no activity (it says what the agent touches on the phone) and no PC name.
     */
    fun publicHeadline(): Headline = when (val h = headline()) {
        Headline(R.string.notif_working_on, h.arg) -> Headline(R.string.notif_working)
        Headline(R.string.status_connected, h.arg) -> Headline(R.string.notif_public_connected)
        else -> h
    }

    companion object {
        const val MAX_TEXT = 300

        fun of(s: BridgeState, cut: Boolean): ConversationView {
            val conv = s.sessionState.conversation
            val m = s.chat.messages.lastOrNull { conv.isEmpty() || it.conversation.isEmpty() || it.conversation == conv }
            val text = m?.let { it.text.ifEmpty { it.attachments.joinToString { a -> a.name } } }
            val last = if (m == null || text.isNullOrEmpty()) null else LastMessage(m.id, m.role == "user", cutText(text))
            val working = s.sessionState.state == "working"
            return ConversationView(StatusView.of(s, cut), s.sessionState.agent, s.sessionState.state, if (working) s.sessionState.activity else "", last)
        }

        private fun cutText(t: String) = if (t.length <= MAX_TEXT) t else t.take(MAX_TEXT - 1) + "…"
    }
}

object Notify {
    private const val MAX_LINE = 120

    /** The first line of the agent's last answer in this conversation, for "Done"; null when it said nothing. */
    fun doneLine(chat: ChatState, conversation: String): String? {
        val m = chat.messages.lastOrNull {
            it.role == "assistant" && (conversation.isEmpty() || it.conversation.isEmpty() || it.conversation == conversation)
        } ?: return null
        val line = m.text.lineSequence().map { it.trim() }.firstOrNull { it.isNotEmpty() } ?: return null
        return if (line.length <= MAX_LINE) line else line.take(MAX_LINE)
    }
}
