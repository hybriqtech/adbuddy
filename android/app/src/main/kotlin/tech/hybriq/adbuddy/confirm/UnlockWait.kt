package tech.hybriq.adbuddy.confirm

import java.util.concurrent.atomic.AtomicBoolean

/**
 * An action (Allow, a direct reply, a screenshot) that a notification button delivered while the phone was locked
 * (M2-8). On Android 12+ the system holds the button until the PIN is typed, but it delivers it about 300 ms before
 * KeyguardManager.isDeviceLocked turns false. This waits for the APP to read "unlocked", for at most [WINDOW_MS]
 * by [now] (not by the number of polls: a stalled main thread that polls late gets false). Whatever else happens
 * (the window ends, an exception, Android 11 and before) the answer is false: the action is dropped, as before.
 * Android 11 has no setAuthenticationRequired: its button arrives from the lock screen with no authentication, so it
 * never waits.
 */
internal class UnlockWait(
    private val sdk: Int,
    private val isLocked: () -> Boolean,
    private val now: () -> Long,
    private val schedule: (delayMs: Long, step: () -> Unit) -> Unit,
    private val onResult: (unlocked: Boolean) -> Unit,
) {
    private val done = AtomicBoolean(false)
    private var start = 0L

    fun begin() {
        if (sdk < MIN_SDK) return finish(false)
        start = now()
        step()
    }

    private fun step() {
        if (done.get()) return
        val elapsed = now() - start
        val locked = try { isLocked() } catch (e: RuntimeException) { true }
        when {
            !locked && elapsed <= WINDOW_MS -> finish(true)
            elapsed >= WINDOW_MS -> finish(false)
            else -> try { schedule(POLL_MS, ::step) } catch (e: RuntimeException) { finish(false) }
        }
    }

    private fun finish(unlocked: Boolean) {
        if (done.compareAndSet(false, true)) onResult(unlocked)
    }

    companion object {
        const val WINDOW_MS = 2000L
        const val POLL_MS = 50L
        const val MIN_SDK = 31 // Build.VERSION_CODES.S: setAuthenticationRequired

        /**
         * What every button that needs the phone unlocked does: [onResult] gets true at once when it was not locked
         * at delivery (the caller still decides with a fresh read), after the wait when it was, false when it never
         * unlocked. Calls [onResult] exactly once.
         */
        fun afterUnlock(
            lockedAtDelivery: Boolean, sdk: Int, isLocked: () -> Boolean, now: () -> Long,
            schedule: (Long, () -> Unit) -> Unit, onResult: (Boolean) -> Unit,
        ) {
            if (!lockedAtDelivery) return onResult(true)
            UnlockWait(sdk, isLocked, now, schedule, onResult).begin()
        }
    }
}
