package tech.hybriq.adbuddy.confirm

import android.os.SystemClock

/** RED stub. */
class ConfirmArming(private val clock: () -> Long = SystemClock::elapsedRealtime) {
    fun shown(id: String) {}
    fun forget(id: String) {}
    fun isArmed(id: String): Boolean = true
    fun retain(ids: Set<String>) {}
    fun clear() {}
    internal fun tracked(): Set<String> = emptySet()
}
