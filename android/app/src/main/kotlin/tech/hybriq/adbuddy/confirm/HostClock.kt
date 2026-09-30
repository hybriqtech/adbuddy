package tech.hybriq.adbuddy.confirm

import java.time.Instant
import java.time.format.DateTimeParseException
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.Protocol

/**
 * The host's clock, as seen from the phone. The host writes expiresAt in its
 * own time; if the phone's clock is off, a question would expire early or
 * late here. The ts of the message that brought the questions (a
 * confirm.request or a snapshot) gives the difference; until one arrives, or
 * when its ts is unreadable, the phone's own time is used.
 *
 * The difference also holds the few milliseconds the message took to arrive
 * (adb reverse, loopback), which is negligible next to a question's minute.
 */
class HostClock(private val clock: () -> Long = System::currentTimeMillis) {
    @Volatile private var skew = 0L

    /** Now in the host's clock, in epoch milliseconds. */
    fun now(): Long = clock() + skew

    /** A moment of the host's clock in the phone's, to show it. */
    fun toLocal(hostMillis: Long): Long = hostMillis - skew

    /** Takes the difference from a message that brings questions. */
    fun observe(m: Message) {
        if (m.type != Protocol.CONFIRM_REQUEST && m.type != Protocol.SNAPSHOT) return
        skewOf(m.ts, clock())?.let { skew = it }
    }

    companion object {
        /** How far ahead of the phone the host is (negative: behind); null for an unreadable ts. */
        fun skewOf(ts: String, local: Long): Long? = try {
            Instant.parse(ts).toEpochMilli() - local
        } catch (e: DateTimeParseException) {
            null
        }
    }
}
