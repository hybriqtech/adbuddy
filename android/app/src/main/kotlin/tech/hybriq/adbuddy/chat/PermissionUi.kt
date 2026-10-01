package tech.hybriq.adbuddy.chat

import java.time.Instant
import tech.hybriq.adbuddy.bridge.PermissionRequest
import tech.hybriq.adbuddy.bridge.PermissionReply

/** What the card of an agent's permission question shows and lets the person do (spec M4 §7.3). Pure. */
object PermissionUi {
    const val ALLOW = "allow"
    const val ALLOW_ALWAYS = "allow_always"
    const val DENY = "deny"

    /** A card just promoted to first (the one before it was resolved) cannot be answered for this long: a second tap meant for the old one must not grant it. */
    const val PROMOTE_GUARD_MS = 600L

    /** The only decisions of the protocol (spec §5.2). */
    fun isDecision(d: String) = d == ALLOW || d == ALLOW_ALWAYS || d == DENY

    /**
     * The question's time limit in epoch milliseconds, by the host's clock; null when it cannot be read (then it never expires here: the host sends permission.resolved).
     * The host's clock is confirm.HostClock (from the welcome's ts), which the screen passes as hostNowMs: nothing to compute here.
     */
    fun expiresAtMs(p: PermissionRequest): Long? = try {
        Instant.parse(p.expiresAt).toEpochMilli()
    } catch (e: Exception) {
        null
    }

    /** Whole seconds left by the host's clock, never below zero; null when the limit cannot be read. */
    fun secondsLeft(p: PermissionRequest, hostNowMs: Long): Long? = expiresAtMs(p)?.let { ((it - hostNowMs + 999) / 1000).coerceAtLeast(0) }

    /**
     * The buttons work only while the host listens, the question is in time, and "always" only if the host offered it.
     * Not while an answer to it is on its way ([answering]: until permission.resolved or a lost connection) nor
     * while it has just been promoted ([guardLeftMs], ChatController.guardLeftMs).
     */
    fun canAnswer(p: PermissionRequest, decision: String, connected: Boolean, hostNowMs: Long, answering: Boolean = false, guardLeftMs: Long = 0): Boolean {
        if (!connected || answering || guardLeftMs > 0) return false
        if (!isDecision(decision)) return false
        if (decision == ALLOW_ALWAYS && !p.allowAlways) return false
        return secondsLeft(p, hostNowMs)?.let { it > 0 } ?: true
    }

    /**
     * The next time (in ms from now) a card changes by itself (its seconds, or its expiry), or null when none does.
     * [guardLeftMs]: what is left of the promote guard of the first card, which also ends by itself.
     */
    fun nextTickMs(permissions: List<PermissionRequest>, hostNowMs: Long, guardLeftMs: Long = 0): Long? {
        val counting = permissions.any { secondsLeft(it, hostNowMs)?.let { s -> s > 0 } == true }
        return when {
            guardLeftMs > 0 -> if (counting) minOf(guardLeftMs, 1000L) else guardLeftMs
            counting -> 1000L
            else -> null
        }
    }

    /** The payload of the answer: a permission.reply, which has no rid (protocol spec §5.2). The only way to build it: ChatController uses it. */
    fun reply(id: String, decision: String) = PermissionReply(id, decision)

    fun reply(p: PermissionRequest, decision: String) = reply(p.id, decision)
}

/**
 * Which card may be answered: the first one, and only once it has been first for
 * [PermissionUi.PROMOTE_GUARD_MS] (the person may be tapping the card that was there before).
 * The clock is injected so a test can move it. Thread safe.
 */
class PermissionGuard(private val clock: () -> Long = System::currentTimeMillis) {
    private var firstId: String? = null
    private var since = 0L

    /** The cards as the screen shows them now, oldest first: a new first one starts the guard. */
    @Synchronized fun observe(ids: List<String>) {
        val first = ids.firstOrNull()
        if (first != firstId) {
            firstId = first
            since = clock()
        }
    }

    /** Milliseconds before [id] may be answered; 0 when it may. Before any observe nothing is known, so nothing is guarded. */
    @Synchronized fun leftMs(id: String): Long {
        val f = firstId ?: return 0
        if (f != id) return Long.MAX_VALUE // not the card on screen
        return (since + PermissionUi.PROMOTE_GUARD_MS - clock()).coerceIn(0, PermissionUi.PROMOTE_GUARD_MS)
    }
}
