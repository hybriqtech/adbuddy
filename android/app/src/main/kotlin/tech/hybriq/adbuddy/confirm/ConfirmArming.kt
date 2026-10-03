package tech.hybriq.adbuddy.confirm

import android.os.SystemClock

/**
 * When each question's notification showed its Allow, so that ConfirmReceiver drops an Allow tapped in the first
 * ConfirmUi.ARMING_MS (ConfirmUi.armed): a stray tap or a burst as the notification pops up allows nothing. Deny never
 * asks it.
 *
 * Counted in [clock], the phone's elapsed time by default: neither the PC's clock nor a change of the phone's time
 * moves it. ConfirmNotifier marks a question [shown] when it posts it with Allow, only the first time (posting it again,
 * as a change of status or language does, keeps the wait it had); posted without Allow (answered, offline) it is
 * [forget]: when the button comes back it waits again. Nothing is kept on disk: after the process restarts no question
 * is known, so the Allow of an old notification is dropped until the new process posts it again. The questions no
 * longer pending are dropped ([retain]), so the map never grows.
 */
class ConfirmArming(private val clock: () -> Long = SystemClock::elapsedRealtime) {
    private val shownAt = HashMap<String, Long>()

    /** The notification of id shows Allow now; a question already shown keeps its time. */
    @Synchronized
    fun shown(id: String) {
        shownAt.putIfAbsent(id, clock())
    }

    /** The notification of id no longer shows Allow. */
    @Synchronized
    fun forget(id: String) {
        shownAt.remove(id)
    }

    /** Whether Allow of id has been shown for ConfirmUi.ARMING_MS; an unknown id is not armed. */
    @Synchronized
    fun isArmed(id: String): Boolean = ConfirmUi.armed(shownAt[id], clock())

    /** RED stub. */
    @Synchronized
    fun rearmAll() {}

    /** Forgets every question but ids (those still pending). */
    @Synchronized
    fun retain(ids: Set<String>) {
        shownAt.keys.retainAll(ids)
    }

    @Synchronized
    fun clear() = shownAt.clear()

    /** The questions it knows, for the tests. */
    @Synchronized
    internal fun tracked(): Set<String> = shownAt.keys.toSet()
}
