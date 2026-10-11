package tech.hybriq.adbuddy.notify

import androidx.annotation.StringRes
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ChatState
import tech.hybriq.adbuddy.ui.LinkStatus
import tech.hybriq.adbuddy.ui.PauseButton
import tech.hybriq.adbuddy.ui.StatusView

data class LastMessage(val id: String, val fromUser: Boolean, val text: String)

enum class NotifAction { REPLY, CAPTURE, PAUSE, RESUME, CUT }

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
    /** The agent has a permission question pending: a generic notice, never its tool, command or detail. */
    val asking: Boolean = false,
) {
    /** The buttons of the conversation notification, in order: asking leaves only the safe cut (the answer is given in the app). */
    fun actions(): List<NotifAction> = buildList {
        if (!asking) {
            add(NotifAction.REPLY)
            add(NotifAction.CAPTURE)
            status.button?.let { add(if (it == PauseButton.RESUME) NotifAction.RESUME else NotifAction.PAUSE) }
        }
        add(NotifAction.CUT)
    }

    fun headline(): Headline = when {
        status.status != LinkStatus.CONNECTED -> Headline(status.status.text)
        asking -> Headline(R.string.notif_asking)
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
            val status = StatusView.of(s, cut)
            return ConversationView(status, s.sessionState.agent, s.sessionState.state, if (working) s.sessionState.activity else "", last,
                asking = AskNotice.pending(s).isNotEmpty() && status.status == LinkStatus.CONNECTED)
        }

        private fun cutText(t: String) = if (t.length <= MAX_TEXT) t else t.take(MAX_TEXT - 1) + "…"
    }
}

object Notify {
    /** "Done" yields to a pending question: the generic "needs an answer" notice already says it. */
    fun shouldAnnounceDone(s: BridgeState, detectorFired: Boolean): Boolean = detectorFired && s.chat.permissions.isEmpty()

    private const val MAX_LETTERS = 100

    /** Markdown is looked for only in the first of these chars: the rest never reaches the line and the regexes stay cheap. */
    private const val SCAN = 1000
    private val FENCE_LINE = Regex("(?m)^[ \\t]*```[^\\n`]*$")
    private val FENCE = Regex("```")
    private val IMAGE_OR_LINK = Regex("!?\\[([^\\]\\[]{0,200})]\\([^)\\s]{0,300}\\)")
    private val LINE_MARKS = Regex("(?m)^[ \\t]*(?:#{1,6}[ \\t]+|>+[ \\t]*|[-*+][ \\t]+)")
    private val STRONG = Regex("(\\*\\*|__)(?=[^\\s*_])(.{1,300}?)(?<=[^\\s*_])\\1")
    private val STRIKE = Regex("~~(?=\\S)(.{1,300}?)(?<=\\S)~~")
    private val CODE = Regex("`([^`\\n]{0,300})`")
    private val EMPHASIS = Regex("(?<![\\p{L}\\p{N}*_])([*_])(?=[^\\s*_])(.{1,300}?)(?<=[^\\s*_])\\1(?![\\p{L}\\p{N}*_])")
    private val INVISIBLE = Regex("[\\p{Z}\\p{Cc}\\p{Cf}]+")

    /** The start of the agent's last answer in this conversation, for "Done"; null when it said nothing. */
    fun doneLine(chat: ChatState, conversation: String): String? {
        val m = chat.messages.lastOrNull {
            it.role == "assistant" && (conversation.isEmpty() || it.conversation.isEmpty() || it.conversation == conversation)
        } ?: return null
        val head = if (m.text.length <= SCAN) m.text else m.text.take(if (m.text[SCAN - 1].isHighSurrogate()) SCAN - 1 else SCAN)
        val plain = head.replace(FENCE_LINE, " ").replace(FENCE, "").replace(IMAGE_OR_LINK, "$1").replace(LINE_MARKS, "")
            .replace(STRONG, "$2").replace(STRIKE, "$1").replace(CODE, "$1").replace(EMPHASIS, "$2")
            .replace(INVISIBLE, " ").trim()
        if (plain.isEmpty()) return null
        if (plain.length <= MAX_LETTERS) return plain
        // Never between the two halves of a surrogate pair; at a word boundary when one is near.
        var end = MAX_LETTERS
        if (plain[end - 1].isHighSurrogate()) end--
        val space = plain.lastIndexOf(' ', end)
        if (space >= MAX_LETTERS / 2) end = space
        return plain.substring(0, end).trimEnd() + "…"
    }
}
