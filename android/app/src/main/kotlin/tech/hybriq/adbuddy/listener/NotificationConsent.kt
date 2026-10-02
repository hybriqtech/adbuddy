package tech.hybriq.adbuddy.listener

import android.annotation.SuppressLint
import android.content.Context
import android.content.SharedPreferences
import androidx.core.content.edit

/**
 * The user's own "yes" to the agent reading their notifications (M5c, ruling 15): a mark that ONLY a tap
 * on the confirm button of ADBuddy's notification access screen sets (confirm()). Nothing sets it by seeing
 * that the system says "allowed": an agent that taps the system's switch with `adbuddy tap` still gets
 * `unavailable` until the person confirms here, and it cannot press ADBuddy's buttons (ADBuddy is protected).
 * The system revoking the access clears it: the listener itself when it is unbound (NotificationGate.onUnbound),
 * and the gate and the screen when they see it. It holds a flag, never content.
 */
class NotificationConsent(private val prefs: SharedPreferences) {
    fun confirmed(): Boolean = prefs.getBoolean(KEY, false)

    /** Only the screen's confirm button calls this. */
    fun confirm() {
        prefs.edit { putBoolean(KEY, true) }
    }

    /**
     * Synchronous: the mark is gone, in the file, when this returns (the listener calls it as the system unbinds it,
     * and the process may be killed right after). If the write fails, it tries once more.
     */
    @SuppressLint("ApplySharedPref")
    fun revoke() {
        if (!prefs.edit().putBoolean(KEY, false).commit()) prefs.edit().putBoolean(KEY, false).commit()
    }

    companion object {
        private const val PREFS = "notification_consent"
        private const val KEY = "confirmed"

        fun of(context: Context) = NotificationConsent(context.getSharedPreferences(PREFS, Context.MODE_PRIVATE))
    }
}
