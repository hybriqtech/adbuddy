package tech.hybriq.adbuddy.confirm

import java.time.Instant
import java.time.format.DateTimeParseException
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Decision

/**
 * What the notification and the dialog show for each 🔴 question (spec §4.5).
 * Pure, so it is unit-tested. A question answered here stays in pending,
 * marked as answering, until its final journal entry: meanwhile it shows
 * "Sending…" and no buttons, and a dropped connection (which empties
 * answering) makes it answerable again. A question past its expiresAt whose
 * final entry has not come yet shows as expired, without buttons.
 */
object ConfirmUi {
    /** OPEN: it can be answered; SENDING: answered here, on its way; EXPIRED: too late to answer. */
    enum class Status { OPEN, SENDING, EXPIRED }

    /** When the question expires, in epoch milliseconds; null when unreadable. */
    fun expiry(r: ConfirmRequest): Long? = try {
        Instant.parse(r.expiresAt).toEpochMilli()
    } catch (e: DateTimeParseException) {
        null
    }

    /**
     * Whether the question can be answered at now. An answer on its way wins
     * (it left before the expiry), and an unreadable expiry cannot be trusted
     * to be in the future, so it counts as expired.
     */
    fun status(r: ConfirmRequest, answering: Boolean, now: Long): Status {
        if (answering) return Status.SENDING
        val expires = expiry(r) ?: return Status.EXPIRED
        return if (now < expires) Status.OPEN else Status.EXPIRED
    }

    /** The buttons a question offers, in order: none unless it can be answered. */
    fun decisions(r: ConfirmRequest, status: Status): List<String> = when {
        status != Status.OPEN -> emptyList()
        r.allowSession -> listOf(Decision.DENY, Decision.ALLOW, Decision.ALLOW_SESSION)
        else -> listOf(Decision.DENY, Decision.ALLOW)
    }

    /**
     * The question the dialog shows: the one the notification opened, else the
     * first that can be answered, else one being sent or expired (so the
     * person sees what happened to it).
     */
    fun question(state: BridgeState, focus: String?, now: Long): ConfirmRequest? {
        val open = state.pending.filter { status(it, it.id in state.answering, now) == Status.OPEN }
        return open.firstOrNull { it.id == focus } ?: open.firstOrNull() ?: state.pending.firstOrNull()
    }

    /** Milliseconds until the soonest expiry still to come, when the screen must look again; null if none. */
    fun nextExpiry(pending: List<ConfirmRequest>, now: Long): Long? =
        pending.mapNotNull { expiry(it) }.map { it - now }.filter { it > 0 }.minOrNull()

    /** One notification to post (or post again, replacing the one with its id). */
    data class Post(val request: ConfirmRequest, val answering: Boolean, val left: Long, val expires: Instant)

    /** cancel: notifications to remove; shown: what is on screen after this plan, id → posted as answering. */
    data class Plan(val cancel: Set<String>, val post: List<Post>, val shown: Map<String, Boolean>)

    /**
     * What to change in the notifications for a new state, given what is shown.
     * A question is posted when it is new or its answering mark changed, and
     * only while it has not expired (the notification times out by itself).
     * Questions no longer pending are cancelled: answered elsewhere or gone.
     */
    fun plan(shown: Map<String, Boolean>, state: BridgeState, now: Long): Plan {
        val ids = state.pending.map { it.id }.toSet()
        val next = shown.filterKeys { it in ids }.toMutableMap()
        val post = mutableListOf<Post>()
        for (r in state.pending) {
            val answering = r.id in state.answering
            if (shown[r.id] == answering) continue
            val expires = expiry(r) ?: continue
            val left = expires - now
            if (left <= 0) continue // expired: the notification timed out, or never shows
            post += Post(r, answering, left, Instant.ofEpochMilli(expires))
            next[r.id] = answering
        }
        return Plan(cancel = shown.keys - ids, post = post, shown = next)
    }
}
