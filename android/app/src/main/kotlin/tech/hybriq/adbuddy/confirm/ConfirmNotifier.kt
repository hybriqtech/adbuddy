package tech.hybriq.adbuddy.confirm

import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
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
 * Allowing needs the phone unlocked (setAuthenticationRequired, Android 12+).
 * Questions answered elsewhere or expired disappear; one answered here shows
 * "Sending…" without buttons until the host records it (ConfirmUi).
 */
object ConfirmNotifier {
    /** What is on screen: id → posted as answering. */
    private var shown = emptyMap<String, Boolean>()

    @Synchronized
    fun sync(context: Context, state: BridgeState) {
        val nm = context.getSystemService(NotificationManager::class.java)
        val plan = ConfirmUi.plan(shown, state, System.currentTimeMillis())
        for (id in plan.cancel) nm.cancel(id, NOTIFICATION_ID)
        for (p in plan.post) nm.notify(p.request.id, NOTIFICATION_ID, build(context, p))
        shown = plan.shown
    }

    /** Removes every question: nobody can answer them once the channel is gone for good. */
    @Synchronized
    fun clear(context: Context) {
        val nm = context.getSystemService(NotificationManager::class.java)
        for (id in shown.keys) nm.cancel(id, NOTIFICATION_ID)
        shown = emptyMap()
    }

    private fun build(context: Context, p: ConfirmUi.Post): android.app.Notification {
        val r = p.request
        val time = DateTimeFormatter.ofPattern("HH:mm:ss").withZone(ZoneId.systemDefault()).format(p.expires)
        val body = buildString {
            append(r.rawCommand)
            if (r.detail.isNotEmpty()) append("\n").append(r.detail)
            if (r.special) append("\n⚠ ").append(context.getString(R.string.confirm_special))
            append("\n").append(
                if (p.answering) context.getString(R.string.confirm_sending) else context.getString(R.string.confirm_expires, time),
            )
        }
        val open = PendingIntent.getActivity(context, r.id.hashCode(),
            Intent(context, MainActivity::class.java).putExtra(MainActivity.EXTRA_CONFIRM, r.id),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val b = NotificationCompat.Builder(context, AdbuddyApp.CHANNEL_CONFIRM)
            .setSmallIcon(android.R.drawable.ic_dialog_alert)
            .setContentTitle(context.getString(R.string.confirm_title, r.title))
            .setContentText(if (p.answering) context.getString(R.string.confirm_sending) else r.rawCommand)
            .setStyle(NotificationCompat.BigTextStyle().bigText(body))
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setCategory(NotificationCompat.CATEGORY_CALL)
            .setContentIntent(open)
            .setTimeoutAfter(p.left)
            .setOnlyAlertOnce(true)
        // Posted only before the expiry, and it times out at the expiry: never answerable late.
        val status = if (p.answering) ConfirmUi.Status.SENDING else ConfirmUi.Status.OPEN
        for (d in ConfirmUi.decisions(r, status)) b.addAction(action(context, r.id, d))
        return b.build()
    }

    private fun action(context: Context, id: String, decision: String): NotificationCompat.Action {
        val label = when (decision) {
            Decision.ALLOW -> R.string.action_allow
            Decision.ALLOW_SESSION -> R.string.action_allow_session
            else -> R.string.action_deny
        }
        val intent = Intent(context, ConfirmReceiver::class.java)
            .setAction(ConfirmReceiver.ACTION_REPLY)
            .putExtra(ConfirmReceiver.EXTRA_ID, id)
            .putExtra(ConfirmReceiver.EXTRA_DECISION, decision)
        val pi = PendingIntent.getBroadcast(context, (id + decision).hashCode(), intent,
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        return NotificationCompat.Action.Builder(0, context.getString(label), pi)
            .setAuthenticationRequired(decision != Decision.DENY) // unlocking is required to allow (Android 12+)
            .build()
    }

    const val NOTIFICATION_ID = 2
}
