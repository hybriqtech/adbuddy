package tech.hybriq.adbuddy.confirm

import android.app.KeyguardManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.bridge.BridgeState

/**
 * Receives the notification buttons (not exported: only this app's PendingIntents reach it).
 *
 * It sends only what ConfirmUi.canSend allows right now: the question is
 * still pending, open (not answered here, not expired, the PC connected) and
 * the notification offers that decision (never Allow during the session, nor Allow for a special question
 * or a command it cuts: those are allowed from the dialog); Allow also needs the phone unlocked
 * (isDeviceLocked), besides setAuthenticationRequired. A tap
 * that arrives late or from a stale notification sends nothing.
 *
 * It does not remove the notification: once the answer is on its way the
 * state marks the question as answering and ConfirmNotifier posts it again
 * without buttons; if it could not be sent, the buttons stay.
 */
class ConfirmReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != ACTION_REPLY) return
        val id = intent.getStringExtra(EXTRA_ID) ?: return
        val decision = intent.getStringExtra(EXTRA_DECISION) ?: return
        val app = context.applicationContext as AdbuddyApp
        val locked = context.getSystemService(KeyguardManager::class.java).isDeviceLocked
        if (!mayReply(app.bridge.state.value, id, decision, app.clock.now(), locked, ConfirmNotifier.arming)) return
        app.bridge.reply(id, decision)
    }

    companion object {
        /** RED stub. */
        internal fun mayReply(state: BridgeState, id: String, decision: String, now: Long, locked: Boolean, arming: ConfirmArming): Boolean =
            ConfirmUi.canSend(state, id, decision, now, locked, armed = true, notification = true)

        const val ACTION_REPLY = "tech.hybriq.adbuddy.action.CONFIRM_REPLY"
        const val EXTRA_ID = "id"
        const val EXTRA_DECISION = "decision"
    }
}
