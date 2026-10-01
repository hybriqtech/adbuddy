package tech.hybriq.adbuddy.chat

import java.time.Instant
import tech.hybriq.adbuddy.bridge.PermissionRequest
import tech.hybriq.adbuddy.bridge.PermissionReply

/** What the card of an agent's permission question shows and lets the person do (spec M4 §7.3). Pure. */
object PermissionUi {
    const val ALLOW = "allow"
    const val ALLOW_ALWAYS = "allow_always"
    const val DENY = "deny"

    /** The question's time limit in epoch milliseconds, by the host's clock; null when it cannot be read (then it never expires here: the host sends permission.resolved). */
    fun expiresAtMs(p: PermissionRequest): Long? = try {
        Instant.parse(p.expiresAt).toEpochMilli()
    } catch (e: Exception) {
        null
    }

    /** Whole seconds left by the host's clock, never below zero; null when the limit cannot be read. */
    fun secondsLeft(p: PermissionRequest, hostNowMs: Long): Long? = expiresAtMs(p)?.let { ((it - hostNowMs + 999) / 1000).coerceAtLeast(0) }

    /** The buttons work only while the host listens, the question is in time, and "always" only if the host offered it. */
    fun canAnswer(p: PermissionRequest, decision: String, connected: Boolean, hostNowMs: Long): Boolean {
        if (!connected) return false
        if (decision != ALLOW && decision != ALLOW_ALWAYS && decision != DENY) return false
        if (decision == ALLOW_ALWAYS && !p.allowAlways) return false
        return secondsLeft(p, hostNowMs)?.let { it > 0 } ?: true
    }

    /** The next time (in ms from now) a card changes by itself (its seconds, or its expiry), or null when none does. */
    fun nextTickMs(permissions: List<PermissionRequest>, hostNowMs: Long): Long? =
        if (permissions.any { secondsLeft(it, hostNowMs)?.let { s -> s > 0 } == true }) 1000L else null

    /** The payload of the answer: a permission.reply, which has no rid (protocol spec §5.2). */
    fun reply(p: PermissionRequest, decision: String) = PermissionReply(p.id, decision)
}
