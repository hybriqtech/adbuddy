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
     * Whole seconds left until the question expires at [now] (the host's clock), rounded up as
     * PermissionUi.secondsLeft (so "0:00" never shows with Allow still on), never below zero; null when it
     * has no readable expiry.
     */
    fun secondsLeft(r: ConfirmRequest, now: Long): Long? = expiry(r)?.let { ((it - now + 999) / 1000).coerceAtLeast(0) }

    /**
     * Whether the question can be answered at now. An answer on its way wins
     * (it left before the expiry); an unreadable expiry cannot be trusted to
     * be in the future, so it counts as expired; then the connection.
     */
    fun status(r: ConfirmRequest, state: BridgeState, now: Long): Status {
        if (r.id in state.answering) return Status.SENDING
        val expires = expiry(r) ?: return Status.EXPIRED
        if (now >= expires) return Status.EXPIRED
        if (!state.isConnected) return Status.OFFLINE
        return Status.OPEN
    }

    /**
     * The buttons a question offers in the dialog, in order: none unless it can be answered. Only Deny for a
     * command longer than the dialog shows (H3): nobody can have read its end. Allow during the session only when
     * the host offers it and the notification shows the whole command (A4-2): what a person allows for the rest
     * of the session must fit on screen at a glance.
     */
    fun decisions(r: ConfirmRequest, status: Status): List<String> = when {
        status != Status.OPEN -> emptyList()
        tooLong(r) -> listOf(Decision.DENY)
        r.allowSession && !cut(r) -> listOf(Decision.DENY, Decision.ALLOW, Decision.ALLOW_SESSION)
        else -> listOf(Decision.DENY, Decision.ALLOW)
    }

    /**
     * The buttons of the notification: those of the dialog, but never Allow during the session (M2: collapsed,
     * the notification shows only the title and a line), and only Deny for a special question (its ⚠ must be
     * read, A4-3) or a command the notification cuts (the rest must be read, A4-2). The person opens the dialog
     * for the rest.
     */
    fun notificationDecisions(r: ConfirmRequest, status: Status): List<String> =
        decisions(r, status).filter { it != Decision.ALLOW_SESSION }
            .let { all -> if (r.special || cut(r)) all.filter { it == Decision.DENY } else all }

    /** Why a notification does not allow what the question asks: the command is [TOO_LONG], [CUT] or [SPECIAL]. */
    enum class Why { TOO_LONG, CUT, SPECIAL }

    fun why(r: ConfirmRequest, status: Status): Why? = when {
        status != Status.OPEN -> null
        tooLong(r) -> Why.TOO_LONG
        Decision.ALLOW in notificationDecisions(r, status) -> null
        cut(r) -> Why.CUT
        else -> Why.SPECIAL
    }

    /** Whether the command is longer than the dialog shows ([DIALOG_MAX_COMMAND]): it can only be denied. */
    fun tooLong(r: ConfirmRequest): Boolean = r.rawCommand.length > DIALOG_MAX_COMMAND

    /** Whether the notification cuts the command (longer than [MAX_COMMAND]). */
    fun cut(r: ConfirmRequest): Boolean = r.rawCommand.length > MAX_COMMAND

    /**
     * Whether a button may send decision for id now: the question is still open and offers that decision where
     * it was tapped (the [notification] offers less than the dialog), and allowing needs the phone unlocked (a
     * tap can arrive late, after the bouncer, or from a stale notification) and the button [armed]: shown for
     * [ARMING_MS] ([armed] for the notification, ConfirmHost for the dialog), so a stray tap or a burst as the
     * question appears allows nothing. Denying is always fine.
     */
    fun canSend(state: BridgeState, id: String, decision: String, now: Long, locked: Boolean, armed: Boolean, notification: Boolean): Boolean {
        val r = state.pending.firstOrNull { it.id == id } ?: return false
        val status = status(r, state, now)
        val offered = if (notification) notificationDecisions(r, status) else decisions(r, status)
        if (decision !in offered) return false
        return decision == Decision.DENY || (!locked && armed)
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

    /** The open questions the person closed: the chat says they are still waiting (and "Show" opens the same dialog again). */
    fun closedWaiting(state: BridgeState, closed: Set<String>, now: Long): List<ConfirmRequest> =
        state.pending.filter { it.id in closed && status(it, state, now) == Status.OPEN }

    /**
     * What a question shows: whether it carries the ⚠ warning, the first [maxDetail] lines of the host's detail
     * and how many more there are, and the agent's command cut to [maxCommand] characters with how many of them
     * are hidden.
     */
    data class Shown(val warning: Boolean, val detail: List<String>, val more: Int, val command: String, val hidden: Int)

    fun shown(r: ConfirmRequest, maxDetail: Int = Int.MAX_VALUE, maxCommand: Int = DIALOG_MAX_COMMAND): Shown {
        val all = if (r.detail.isEmpty()) emptyList() else r.detail.split('\n')
        val detail = all.take(maxDetail)
        if (r.rawCommand.length <= maxCommand) return Shown(r.special, detail, all.size - detail.size, r.rawCommand, 0)
        // Cut by characters, not UTF-16 units: half a surrogate pair would show as garbage (B2).
        val kept = r.rawCommand.take(maxCommand - 1).let { if (it.isNotEmpty() && it.last().isHighSurrogate()) it.dropLast(1) else it }
        return Shown(r.special, detail, all.size - detail.size, "$kept…", r.rawCommand.length - kept.length)
    }

    /**
     * What the notification says, in order: the ⚠ warning first (A4-3: the detail is the agent's to make long,
     * the warning must not fall out of sight), the host's detail (at most [NOTIFICATION_MAX_DETAIL] lines and
     * [more] for the rest), the agent's command cut to [MAX_COMMAND] characters, [open] when the notification
     * does not allow it (why to open the app), and last the footer (expiry or state). With [ltr] the detail and
     * the command start with a left-to-right mark (plain text cannot set a direction): a command that starts
     * with a right-to-left letter is not laid out right to left. The labels are never marked.
     */
    fun notificationLines(r: ConfirmRequest, special: String, footer: String, more: (Int) -> String, open: String?, ltr: Boolean = false): List<String> =
        buildList {
            val mark = if (ltr) LRM else ""
            val s = shown(r, NOTIFICATION_MAX_DETAIL, MAX_COMMAND)
            if (s.warning) add("⚠ $special")
            for (line in s.detail) add(mark + line)
            if (s.more > 0) add(more(s.more))
            add(mark + s.command)
            if (open != null) add(open)
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

    /**
     * Whether an Allow button shown at [shownAt] is armed at [now] (both in the phone's elapsed time): [ARMING_MS]
     * have passed. One not known to be shown ([shownAt] null: a notification of an earlier process) never is.
     */
    fun armed(shownAt: Long?, now: Long): Boolean = shownAt != null && now - shownAt >= ARMING_MS

    /**
     * How long Allow and Allow during the session wait from when they appear (the dialog, and again when the app
     * comes back to the front; the notification's button) before they allow: a tap meant for what was there, or a
     * burst as the question pops up, is not an answer. Deny never waits. As PermissionUi.PROMOTE_GUARD_MS.
     */
    const val ARMING_MS = 600L

    /** What the notification shows of the command; a longer one is cut there and only the dialog allows it. */
    const val MAX_COMMAND = 300

    /** What the dialog shows of the command (it scrolls); past it, it says how many characters it hides. */
    const val DIALOG_MAX_COMMAND = 20_000

    /** Lines of the detail in the notification; the dialog shows them all. */
    const val NOTIFICATION_MAX_DETAIL = 3

    private const val LRM = "‎"
}
