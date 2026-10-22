package tech.hybriq.adbuddy.confirm

import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import java.time.format.FormatStyle
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.Decision
import tech.hybriq.adbuddy.notify.notificationManager
import tech.hybriq.adbuddy.ui.Formats

/**
 * Shows each pending 🔴 question as a high-priority notification (spec §4.5)
 * with Allow and Deny, and Open when the dialog offers more (Allow during the
 * session is offered only there, M2); a special question or a command the
 * notification cuts gets only Deny and Open (ConfirmUi.notificationDecisions):
 * it is allowed from the dialog.
 * Allowing needs the phone unlocked (setAuthenticationRequired, Android 12+,
 * and ConfirmReceiver checks it again) and Allow shown for ConfirmUi.ARMING_MS ([arming]). The notification stays on this phone
 * (setLocalOnly: not on watches nor mirrors). Questions answered elsewhere or
 * expired disappear; without buttons it says why (ConfirmUi.Status).
 */
object ConfirmNotifier {
    /** What is on screen: id → status posted. */
    private var shown = emptyMap<String, ConfirmUi.Status>()

    /** When each question's Allow appeared, for ConfirmReceiver (ConfirmUi.ARMING_MS). */
    internal val arming = ConfirmArming()

    @Synchronized
    fun sync(context: Context, state: BridgeState) {
        val nm = context.notificationManager
        val clock = (context.applicationContext as AdbuddyApp).clock
        val plan = ConfirmUi.plan(shown, state, clock.now())
        post(context, nm, plan, state, clock)
    }

    /**
     * Paints again, in the language now in force, the questions on screen (a change of language: [sync] works by
     * differences and would not touch them). The same ids replace the old notifications, with the same status,
     * decisions and codes; only the words of the phone change.
     */
    @Synchronized
    fun repaint(context: Context, state: BridgeState) {
        val nm = context.notificationManager
        val clock = (context.applicationContext as AdbuddyApp).clock
        val plan = ConfirmUi.repaint(shown, state, clock.now())
        post(context, nm, plan, state, clock)
    }

    private fun post(context: Context, nm: NotificationManager, plan: ConfirmUi.Plan, state: BridgeState, clock: HostClock) {
        for (id in plan.cancel) nm.cancel(id, NOTIFICATION_ID)
        for (p in plan.post) {
            nm.notify(p.request.id, NOTIFICATION_ID, build(context, p, clock.toLocal(p.expires)))
            // Allow waits from when it is posted, once; without it (sent, offline) the next Allow waits again.
            if (Decision.ALLOW in ConfirmUi.notificationDecisions(p.request, p.status)) arming.shown(p.request.id) else arming.forget(p.request.id)
        }
        arming.retain(state.pending.map { it.id }.toSet())
        shown = plan.shown
    }

    /**
     * Removes every question notification, those of an earlier process too:
     * nobody can answer them without the channel.
     */
    @Synchronized
    fun clear(context: Context) {
        val nm = context.notificationManager
        for (n in nm.activeNotifications) if (n.id == NOTIFICATION_ID) nm.cancel(n.tag, n.id)
        shown = emptyMap()
        arming.clear()
    }

    /** The last line of a question: when it expires (in the phone's time) or why it has no buttons. */
    fun footer(context: Context, status: ConfirmUi.Status, localExpires: Long?): String = when (status) {
        ConfirmUi.Status.OPEN -> localExpires?.let {
            context.getString(R.string.confirm_expires, Formats.time(it, FormatStyle.MEDIUM))
        } ?: ""
        ConfirmUi.Status.SENDING -> context.getString(R.string.confirm_sending)
        ConfirmUi.Status.OFFLINE -> context.getString(R.string.confirm_offline)
        ConfirmUi.Status.EXPIRED -> context.getString(R.string.confirm_expired)
    }

    private fun build(context: Context, p: ConfirmUi.Post, localExpires: Long): android.app.Notification {
        val r = p.request
        val footer = footer(context, p.status, localExpires)
        // A special question or a cut command is allowed only from the dialog (A4-2, A4-3), and one longer than the
        // dialog shows only denied (H3): say why, and offer to open it.
        val decisions = ConfirmUi.notificationDecisions(r, p.status)
        val withheld = decisions != ConfirmUi.decisions(r, p.status)
        val why = when (ConfirmUi.why(r, p.status)) {
            ConfirmUi.Why.TOO_LONG -> context.getString(R.string.confirm_too_long)
            ConfirmUi.Why.CUT -> context.getString(R.string.confirm_open_cut)
            ConfirmUi.Why.SPECIAL -> context.getString(R.string.confirm_open_special)
            null -> null
        }
        // The ⚠ first, the rule, the agent's command after it and cut: it never pushes the rule out of sight.
        val lines = ConfirmUi.notificationLines(r, context.getString(R.string.confirm_special), footer,
            { n -> context.resources.getQuantityString(R.plurals.confirm_more, n, n) }, why, ltr = true)
        val open = PendingIntent.getActivity(context, 0,
            Intent(context, MainActivity::class.java).setData(uri("open", r.id)).putExtra(MainActivity.EXTRA_CONFIRM, r.id),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val b = NotificationCompat.Builder(context, AdbuddyApp.CHANNEL_CONFIRM)
            .setSmallIcon(R.drawable.ic_stat_shield)
            .setColor(ContextCompat.getColor(context, R.color.ic_launcher_background))
            .setContentTitle(context.getString(R.string.confirm_title, r.title))
            .setContentText(if (p.status == ConfirmUi.Status.OPEN) lines.first() else footer)
            .setStyle(NotificationCompat.BigTextStyle().bigText(lines.joinToString("\n")))
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setCategory(NotificationCompat.CATEGORY_CALL)
            .setContentIntent(open)
            .setTimeoutAfter(p.left)
            .setOnlyAlertOnce(true)
            .setLocalOnly(true)
        for (d in decisions) b.addAction(action(context, r.id, d))
        if (withheld || why != null) b.addAction(NotificationCompat.Action.Builder(0, context.getString(R.string.action_open), open).build())
        return b.build()
    }

    private fun action(context: Context, id: String, decision: String): NotificationCompat.Action {
        val label = when (decision) {
            Decision.ALLOW -> R.string.action_allow
            Decision.ALLOW_SESSION -> R.string.action_allow_session
            else -> R.string.action_deny
        }
        // The data makes each (question, decision) its own PendingIntent: request codes could collide.
        val intent = Intent(context, ConfirmReceiver::class.java)
            .setAction(ConfirmReceiver.ACTION_REPLY)
            .setData(uri("confirm", id, decision))
            .putExtra(ConfirmReceiver.EXTRA_ID, id)
            .putExtra(ConfirmReceiver.EXTRA_DECISION, decision)
        val pi = PendingIntent.getBroadcast(context, 0, intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        return NotificationCompat.Action.Builder(0, context.getString(label), pi)
            .setAuthenticationRequired(decision != Decision.DENY) // unlocking is required to allow (Android 12+)
            .build()
    }

    private fun uri(kind: String, vararg parts: String): Uri =
        Uri.Builder().scheme("adbuddy").authority(kind).apply { parts.forEach { appendPath(it) } }.build()

    /** The notification id of every question; each one's tag is its id. */
    const val NOTIFICATION_ID = 2
}
