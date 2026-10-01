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
}

/**
 * Which card may be answered: the first one, and only once it has been first for
 * [PermissionUi.PROMOTE_GUARD_MS] (the person may be tapping the card that was there before).
 * It fails closed: before the first [observe], for a card that is not the first, and for one nobody listed, nothing may be answered.
 * The clock is injected (default: monotonic, [System.nanoTime]) so a test can move it. Thread safe.
 */
class PermissionGuard(private val clock: () -> Long = { System.nanoTime() / 1_000_000 }) {
    private var observed = false
    private var firstId: String? = null
    private var since = 0L

    /** The cards as the screen shows them now, oldest first: a new first one (or the first after none) starts the guard. */
    @Synchronized fun observe(ids: List<String>) {
        val first = ids.firstOrNull()
        if (!observed || first != firstId) since = clock()
        observed = true
        firstId = first
    }

    /** A reconnection: the person may have been tapping while the screen was stale, so the first card is guarded again. */
    @Synchronized fun rearm() {
        since = clock()
    }

    /**
     * Milliseconds (0..PROMOTE_GUARD_MS) before [id] may be answered; 0 when it may. null when it may not be answered at all
     * (nothing observed, not the first card, unknown): that is not a time, so nextTickMs never gets it.
     */
    @Synchronized fun leftMs(id: String): Long? {
        if (!observed || firstId != id) return null
        return (since + PermissionUi.PROMOTE_GUARD_MS - clock()).coerceIn(0, PermissionUi.PROMOTE_GUARD_MS)
    }
}
