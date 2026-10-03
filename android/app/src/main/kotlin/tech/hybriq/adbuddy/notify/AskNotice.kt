package tech.hybriq.adbuddy.notify

import android.content.Context
import tech.hybriq.adbuddy.bridge.BridgeState

/**
 * When the "the agent needs an answer" notice is posted and withdrawn. It is for a question the person has not seen:
 * with the app in front the card is already there, so the notice is withdrawn (and posted again if the app goes to
 * the background with the question still pending). A question that comes while another is still open posts it again.
 */
object AskNotice {
    /** The ids of the agent's pending permission questions; none when the host is not connected (a lost link answers nothing). */
    fun pending(s: BridgeState): Set<String> =
        if (s.connection == BridgeState.Connection.CONNECTED) s.chat.permissions.map { it.id }.toSet() else emptySet()

    /** Applies what the state asks for and returns the ids announced now (the next call's [announced]). */
    fun apply(ctx: Context, announced: Set<String>, s: BridgeState, foreground: Boolean): Set<String> {
        val want = if (foreground) emptySet() else pending(s)
        when {
            want.isEmpty() -> if (announced.isNotEmpty()) ConversationNotifier.cancelAsk(ctx)
            want.any { it !in announced } -> ConversationNotifier.ask(ctx)
        }
        return want
    }
}
