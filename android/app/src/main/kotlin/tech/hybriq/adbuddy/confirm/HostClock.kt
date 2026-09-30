package tech.hybriq.adbuddy.confirm

import android.os.SystemClock
import java.time.Instant
import java.time.format.DateTimeParseException
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.Protocol

/**
 * The host's clock, as seen from the phone. The host writes expiresAt in its
 * own time; if the phone's clock is off, a question would expire early or
 * late here.
 *
 * It is taken only from the ts of the welcome: the host makes it with Now()
 * on each connection, never buffers it, and sends it before the backlog. The
 * buffered messages (confirm.request, snapshot…) keep the ts of when they
 * were first sent, so on a reconnection they would set the clock back and
 * make an expired question look current. From the welcome on, the host's
 * time runs with the phone's elapsed time (elapsedRealtime), so changing the
 * phone's clock does not move it. Before any welcome, or if its ts is
 * unreadable (then the last one holds), it is the phone's own time.
 *
 * The welcome's few milliseconds on the way (adb reverse, loopback) are
 * negligible next to a question's minute.
 */
class HostClock(
    private val wall: () -> Long = System::currentTimeMillis,
    private val elapsed: () -> Long = SystemClock::elapsedRealtime,
) {
    /** The host's time at a moment of elapsed; null until a readable welcome. */
    private class Anchor(val host: Long, val elapsed: Long)

    @Volatile private var anchor: Anchor? = null

    /** Now in the host's clock, in epoch milliseconds. */
    fun now(): Long {
        val a = anchor ?: return wall()
        return a.host + (elapsed() - a.elapsed)
    }

    /** A moment of the host's clock in the phone's, to show it. */
    fun toLocal(hostMillis: Long): Long = hostMillis - (now() - wall())

    /** Takes the host's time from a welcome; any other message is ignored. */
    fun observe(m: Message) {
        if (m.type != Protocol.WELCOME) return
        val at = elapsed()
        skewOf(m.ts, 0)?.let { anchor = Anchor(it, at) }
    }

    companion object {
        /** How far ahead of local the time ts is (negative: behind); null for an unreadable ts. */
        fun skewOf(ts: String, local: Long): Long? = try {
            Instant.parse(ts).toEpochMilli() - local
        } catch (e: DateTimeParseException) {
            null
        }
    }
}
