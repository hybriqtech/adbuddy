package tech.hybriq.adbuddy.listener

import android.annotation.SuppressLint
import android.content.Context
import android.content.SharedPreferences
import androidx.core.content.edit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flowOn

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

    /**
     * The mark now and on every change, whoever makes it (the listener clears it as the system unbinds it, while
     * the screen is open). It only reads: observing never sets the mark. The first read is off the main thread.
     */
    fun observe(): Flow<Boolean> = callbackFlow {
        val listener = SharedPreferences.OnSharedPreferenceChangeListener { _, key -> if (key == null || key == KEY) trySend(confirmed()) }
        prefs.registerOnSharedPreferenceChangeListener(listener) // held here: the preferences keep listeners weakly
        trySend(confirmed())
        awaitClose { prefs.unregisterOnSharedPreferenceChangeListener(listener) }
    }.distinctUntilChanged().flowOn(Dispatchers.IO)

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
