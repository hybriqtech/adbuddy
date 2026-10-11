package tech.hybriq.adbuddy.confirm

import android.app.KeyguardManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import java.util.concurrent.atomic.AtomicBoolean
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.Decision

/**
 * Receives the notification buttons (not exported: only this app's PendingIntents reach it).
 *
 * It sends only what ConfirmUi.canSend allows right now: the question is
 * still pending, open (not answered here, not expired, the PC connected) and
 * the notification offers that decision (never Allow during the session, nor Allow for a special question
 * or a command it cuts: those are allowed from the dialog); Allow also needs the phone unlocked
 * (isDeviceLocked), besides setAuthenticationRequired, and its button shown for ConfirmUi.ARMING_MS
 * (ConfirmNotifier.arming: a stray tap as the notification pops up allows nothing; after the PIN of a locked
 * phone it is long armed). A tap that arrives late, too soon or from a stale notification sends nothing.
 *
 * M2-8: on Android 12+ the system delivers a button that asks to unlock about 300 ms before isDeviceLocked turns
 * false, so Allow arriving with the phone locked waits (UnlockWait, 2 s) for the app to read it unlocked, then
 * decides again (the arming is not counted from the unlock, but it must already hold at delivery: an Allow that
 * arrives locked and not yet armed is dropped without waiting).
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
        val km = context.getSystemService(KeyguardManager::class.java)
        val pending = goAsync() // may be null (tests call onReceive directly): finish is then a no-op
        // A reading that fails counts as locked (it waits, and a wait that never reads unlocked drops the action).
        val lockedAtDelivery = try { km.isDeviceLocked } catch (e: RuntimeException) { true }
        val armedAtDelivery = ConfirmNotifier.arming.isArmed(id)
        val handler = Handler(Looper.getMainLooper())
        handle(
            decision, lockedAtDelivery, armedAtDelivery, Build.VERSION.SDK_INT, { km.isDeviceLocked }, SystemClock::elapsedRealtime,
            { d, step -> handler.postDelayed(step, d) },
            mayReplyNow = { locked -> mayReply(app.bridge.state.value, id, decision, app.clock.now(), locked, ConfirmNotifier.arming) },
            send = { app.bridge.reply(id, decision) },
            finish = { pending?.finish() },
        )
    }

    /**
     * The logic of a button (M2-8). Deny never waits. Allow (and any other decision) that arrived while the phone was
     * locked waits, up to UnlockWait.WINDOW_MS, until the app reads it unlocked (the system delivers the button about
     * 300 ms before that), and then decides again from scratch with a fresh read of the lock and the state of now
     * ([mayReplyNow]). [finish] is called exactly once, whatever happens.
     */
    internal fun handle(
        decision: String, lockedAtDelivery: Boolean, armedAtDelivery: Boolean, sdk: Int, isLocked: () -> Boolean, now: () -> Long,
        schedule: (Long, () -> Unit) -> Unit, mayReplyNow: (locked: Boolean) -> Boolean, send: () -> Unit, finish: () -> Unit,
    ) {
        val finished = AtomicBoolean(false)
        val finishOnce = { if (finished.compareAndSet(false, true)) finish() }
        try {
            if (decision == Decision.DENY) { // never waits, never needs the unlock
                if (mayReplyNow(lockedAtDelivery)) send()
                finishOnce(); return
            }
            // Allow delivered locked that the notification had not armed yet (a stray tap as it popped up) is not
            // held until a casual unlock: it is dropped at once.
            if (lockedAtDelivery && !armedAtDelivery) { finishOnce(); return }
            UnlockWait.afterUnlock(lockedAtDelivery, sdk, isLocked, now, schedule) { unlocked ->
                try {
                    // A fresh read of the lock, with the state and the time of now: everything is decided again.
                    val lockedNow = try { isLocked() } catch (e: RuntimeException) { true }
                    if (unlocked && mayReplyNow(lockedNow)) send()
                } finally { finishOnce() }
            }
        } catch (e: RuntimeException) { finishOnce(); throw e }
    }

    companion object {
        /**
         * Whether a notification button may send decision for id: ConfirmUi.canSend, with Allow armed only once its
         * notification has shown it for ConfirmUi.ARMING_MS ([arming]). Deny does not wait.
         */
        internal fun mayReply(state: BridgeState, id: String, decision: String, now: Long, locked: Boolean, arming: ConfirmArming): Boolean =
            ConfirmUi.canSend(state, id, decision, now, locked, armed = arming.isArmed(id), notification = true)

        const val ACTION_REPLY = "tech.hybriq.adbuddy.action.CONFIRM_REPLY"
        const val EXTRA_ID = "id"
        const val EXTRA_DECISION = "decision"
    }
}
