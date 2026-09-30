package tech.hybriq.adbuddy.confirm

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.bridge.Decision

/**
 * Receives the notification buttons (not exported: only this app's PendingIntents reach it).
 * It does not remove the notification: once the answer is on its way the state
 * marks the question as answering and ConfirmNotifier posts it again as
 * "Sending…"; if it could not be sent, the buttons stay. Past its expiry the
 * notification is gone (setTimeoutAfter), and a late tap is ignored.
 */
class ConfirmReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != ACTION_REPLY) return
        val id = intent.getStringExtra(EXTRA_ID) ?: return
        val decision = intent.getStringExtra(EXTRA_DECISION)
            ?.takeIf { it == Decision.ALLOW || it == Decision.DENY || it == Decision.ALLOW_SESSION } ?: return
        val bridge = (context.applicationContext as AdbuddyApp).bridge
        // A tap that arrives late (after unlocking, say) answers nothing that has expired,
        // is already on its way or is no longer asked.
        val state = bridge.state.value
        val r = state.pending.firstOrNull { it.id == id } ?: return
        if (ConfirmUi.status(r, id in state.answering, System.currentTimeMillis()) != ConfirmUi.Status.OPEN) return
        bridge.reply(id, decision)
    }

    companion object {
        const val ACTION_REPLY = "tech.hybriq.adbuddy.action.CONFIRM_REPLY"
        const val EXTRA_ID = "id"
        const val EXTRA_DECISION = "decision"
    }
}
