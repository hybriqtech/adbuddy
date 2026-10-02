package tech.hybriq.adbuddy.confirm

import java.time.Instant
import java.time.format.DateTimeParseException
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Decision

/**
 * What the notification, the dialog and the notification buttons do with each
 * 🔴 question (spec §4.5). Pure, so it is unit-tested. Times are in the host's
 * clock (HostClock.now()), which is the one expiresAt is written in.
 *
 * A question offers its buttons only while it is OPEN: pending, not answered
 * here yet, not expired and with the PC connected. Answered here it stays in
 * pending (answering) until its final journal entry and shows "Answered:
 * waiting for the PC"; a dropped connection shows "No connection with the PC"
 * and empties answering, so the buttons come back when the PC does.
 */
object ConfirmUi {
    /**
     * OPEN: it can be answered; SENDING: answered here, on its way; OFFLINE:
     * the PC is not connected; EXPIRED: too late to answer.
     */
    enum class Status { OPEN, SENDING, OFFLINE, EXPIRED }

    /** When the question expires, in epoch milliseconds of the host's clock; null when unreadable. */
    fun expiry(r: ConfirmRequest): Long? = try {
        Instant.parse(r.expiresAt).toEpochMilli()
    } catch (e: DateTimeParseException) {
        null
    }

    /**
     * Whether the question can be answered at now. An answer on its way wins
     * (it left before the expiry); an unreadable expiry cannot be trusted to
     * be in the future, so it counts as expired; then the connection.
     */
    fun status(r: ConfirmRequest, state: BridgeState, now: Long): Status {
        if (r.id in state.answering) return Status.SENDING
        val expires = expiry(r) ?: return Status.EXPIRED
        if (now >= expires) return Status.EXPIRED
        if (state.connection != BridgeState.Connection.CONNECTED) return Status.OFFLINE
        return Status.OPEN
    }

    /** The buttons a question offers, in order: none unless it can be answered. */
    fun decisions(r: ConfirmRequest, status: Status): List<String> = when {
        status != Status.OPEN -> emptyList()
        r.allowSession -> listOf(Decision.DENY, Decision.ALLOW, Decision.ALLOW_SESSION)
        else -> listOf(Decision.DENY, Decision.ALLOW)
    }

    /**
     * Whether a notification button may send decision for id now: the question
     * is still open and offers that decision, and allowing needs the phone
     * unlocked (a tap can arrive late, after the bouncer, or from a stale
     * notification). Denying is always fine.
     */
    fun canSend(state: BridgeState, id: String, decision: String, now: Long, locked: Boolean): Boolean {
        val r = state.pending.firstOrNull { it.id == id } ?: return false
        if (decision !in decisions(r, status(r, state, now))) return false
        return decision == Decision.DENY || !locked
    }

    /**
     * The question the dialog shows: the one the notification opened, else the
     * first that can be answered, else one being sent, expired or offline (so
     * the person sees what happened to it). closed: those the person closed.
     */
    fun question(state: BridgeState, focus: String?, now: Long, closed: Set<String> = emptySet()): ConfirmRequest? {
        val shown = state.pending.filter { it.id !in closed }
        val open = shown.filter { status(it, state, now) == Status.OPEN }
        return open.firstOrNull { it.id == focus } ?: open.firstOrNull() ?: shown.firstOrNull()
    }

    /**
     * What the question says, in order: the host's rule (detail and the ⚠
     * warning) first, then the agent's command, cut to maxCommand characters,
     * and last the footer (expiry or state). The agent's text never pushes
     * the rule's out of sight. With [ltr] the detail and the command start with a left-to-right mark (for plain
     * text that cannot set a direction, the notification): the app adds it when it shows them, so a command that
     * starts with a right-to-left letter is not laid out right to left. The labels are never marked.
     */
    fun lines(r: ConfirmRequest, special: String, footer: String, maxCommand: Int = MAX_COMMAND, ltr: Boolean = false): List<String> = buildList {
        val mark = if (ltr) LRM else ""
        if (r.detail.isNotEmpty()) add(mark + r.detail)
        if (r.special) add("⚠ $special")
        add(mark + (if (r.rawCommand.length <= maxCommand) r.rawCommand else r.rawCommand.take(maxCommand - 1) + "…"))
        add(footer)
    }

    /** Milliseconds until the soonest expiry still to come, when the screen must look again; null if none. */
    fun nextExpiry(pending: List<ConfirmRequest>, now: Long): Long? =
        pending.mapNotNull { expiry(it) }.map { it - now }.filter { it > 0 }.minOrNull()

    /** One notification to post (or post again, replacing the one with its id); expires in the host's clock. */
    data class Post(val request: ConfirmRequest, val status: Status, val left: Long, val expires: Long)

    /** cancel: notifications to remove; shown: what is on screen after this plan, id → status posted. */
    data class Plan(val cancel: Set<String>, val post: List<Post>, val shown: Map<String, Status>)

    /**
     * What to change in the notifications for a new state, given what is shown.
     * A question is posted when it is new or its status changed, and only
     * while it has not expired (the notification times out by itself then).
     * Questions no longer pending are cancelled: answered elsewhere or gone.
     */
    fun plan(shown: Map<String, Status>, state: BridgeState, now: Long): Plan {
        val ids = state.pending.map { it.id }.toSet()
        val next = shown.filterKeys { it in ids }.toMutableMap()
        val post = mutableListOf<Post>()
        for (r in state.pending) {
            // Past the expiry nothing is posted, not even "answered": the notification timed out.
            val expires = expiry(r) ?: continue
            if (expires <= now) continue
            val status = status(r, state, now)
            if (shown[r.id] == status) continue
            post += Post(r, status, expires - now, expires)
            next[r.id] = status
        }
        return Plan(cancel = shown.keys - ids, post = post, shown = next)
    }

    /**
     * What to post to paint again, in the language now in force, the questions already on screen: the same ids (the
     * notification is replaced, not duplicated) and the status each has now; nothing that was not shown, nothing past
     * its expiry (it timed out by itself), and the ones no longer pending are cancelled.
     */
    fun repaint(shown: Map<String, Status>, state: BridgeState, now: Long): Plan {
        val ids = state.pending.map { it.id }.toSet()
        val post = plan(emptyMap(), state, now).post.filter { it.request.id in shown }
        val next = shown.filterKeys { it in ids } + post.associate { it.request.id to it.status }
        return Plan(cancel = shown.keys - ids, post = post, shown = next)
    }

    const val MAX_COMMAND = 300

    private const val LRM = "‎"
}
