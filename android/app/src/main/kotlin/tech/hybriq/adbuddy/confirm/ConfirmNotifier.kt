package tech.hybriq.adbuddy.confirm

import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.core.app.NotificationCompat
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.Decision

/**
 * Shows each pending 🔴 question as a high-priority notification (spec §4.5)
 * with Allow, Deny and, when the host offers it, Allow during the session.
 * Allowing needs the phone unlocked (setAuthenticationRequired, Android 12+,
 * and ConfirmReceiver checks it again). The notification stays on this phone
 * (setLocalOnly: not on watches nor mirrors). Questions answered elsewhere or
 * expired disappear; without buttons it says why (ConfirmUi.Status).
 */
object ConfirmNotifier {
    /** What is on screen: id → status posted. */
    private var shown = emptyMap<String, ConfirmUi.Status>()

    @Synchronized
    fun sync(context: Context, state: BridgeState) {
        val nm = context.getSystemService(NotificationManager::class.java)
        val clock = (context.applicationContext as AdbuddyApp).clock
        val plan = ConfirmUi.plan(shown, state, clock.now())
        for (id in plan.cancel) nm.cancel(id, NOTIFICATION_ID)
        for (p in plan.post) nm.notify(p.request.id, NOTIFICATION_ID, build(context, p, clock.toLocal(p.expires)))
        shown = plan.shown
    }

    /**
     * Removes every question notification, those of an earlier process too:
     * nobody can answer them without the channel.
     */
    @Synchronized
    fun clear(context: Context) {
        val nm = context.getSystemService(NotificationManager::class.java)
        for (n in nm.activeNotifications) if (n.id == NOTIFICATION_ID) nm.cancel(n.tag, n.id)
        shown = emptyMap()
    }

    /** The last line of a question: when it expires (in the phone's time) or why it has no buttons. */
    fun footer(context: Context, status: ConfirmUi.Status, localExpires: Long?): String = when (status) {
        ConfirmUi.Status.OPEN -> localExpires?.let {
            context.getString(R.string.confirm_expires, DateTimeFormatter.ofPattern("HH:mm:ss").withZone(ZoneId.systemDefault()).format(Instant.ofEpochMilli(it)))
        } ?: ""
        ConfirmUi.Status.SENDING -> context.getString(R.string.confirm_sending)
        ConfirmUi.Status.OFFLINE -> context.getString(R.string.confirm_offline)
        ConfirmUi.Status.EXPIRED -> context.getString(R.string.confirm_expired)
    }

    private fun build(context: Context, p: ConfirmUi.Post, localExpires: Long): android.app.Notification {
        val r = p.request
        val footer = footer(context, p.status, localExpires)
        // The rule first, the agent's command after it and cut: it never pushes the rule out of sight.
        val lines = ConfirmUi.lines(r, context.getString(R.string.confirm_special), footer)
        val open = PendingIntent.getActivity(context, 0,
            Intent(context, MainActivity::class.java).setData(uri("open", r.id)).putExtra(MainActivity.EXTRA_CONFIRM, r.id),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val b = NotificationCompat.Builder(context, AdbuddyApp.CHANNEL_CONFIRM)
            .setSmallIcon(android.R.drawable.ic_dialog_alert)
            .setContentTitle(context.getString(R.string.confirm_title, r.title))
            .setContentText(if (p.status == ConfirmUi.Status.OPEN) lines.first() else footer)
            .setStyle(NotificationCompat.BigTextStyle().bigText(lines.joinToString("\n")))
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setCategory(NotificationCompat.CATEGORY_CALL)
            .setContentIntent(open)
            .setTimeoutAfter(p.left)
            .setOnlyAlertOnce(true)
            .setLocalOnly(true)
        for (d in ConfirmUi.decisions(r, p.status)) b.addAction(action(context, r.id, d))
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
