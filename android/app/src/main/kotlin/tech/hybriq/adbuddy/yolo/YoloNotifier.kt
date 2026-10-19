package tech.hybriq.adbuddy.yolo

import android.app.Notification
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import java.time.format.FormatStyle
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.confirm.HostClock
import tech.hybriq.adbuddy.ui.Formats

/**
 * The persistent notification of the YOLO mode (spec 2026-10-18): up while it is on, with the link up or down (the mode
 * lives on the host), local to this phone, visible on the lock screen, with one action, Turn off, that needs no unlock
 * (turning it off is always safe). Nothing of the agent's goes in it. It is apart from the conversation's notification:
 * its life is the mode's, not the service's.
 */
object YoloNotifier {
    const val NOTIFICATION_ID = 8
    private const val OFF_FAILED = "off-failed"

    /** The key of what is up (YoloUi.notificationKey), null when nothing is. */
    private var shown: String? = null

    @Synchronized
    fun sync(context: Context, state: BridgeState, clock: HostClock) {
        val nm = context.getSystemService(NotificationManager::class.java)
        val key = YoloUi.notificationKey(state)
        if (key == shown) return
        shown = key
        if (key == null) nm.cancel(NOTIFICATION_ID) else nm.notify(NOTIFICATION_ID, build(context, state, clock, offFailed = false))
    }

    /** The Turn off could not be sent: the same notification, saying so. The next change of the mode puts the normal one back. */
    @Synchronized
    fun offFailed(context: Context, state: BridgeState, clock: HostClock) {
        // The mode is known to be off and nothing is up: a late reply must not bring the ongoing notification back.
        if (shown == null && state.yolo?.active == false) return
        // A cold process (the old notification outlived it, yolo == null) has shown == null: it still must say so; the marker makes the next sync replace or cancel it.
        if (shown == null) shown = OFF_FAILED
        context.getSystemService(NotificationManager::class.java).notify(NOTIFICATION_ID, build(context, state, clock, offFailed = true))
    }

    /** Takes down what an earlier process left: nobody can say if the mode is still on until the host does. */
    @Synchronized
    fun clear(context: Context) {
        context.getSystemService(NotificationManager::class.java).cancel(NOTIFICATION_ID)
        shown = null
    }

    private fun build(ctx: Context, state: BridgeState, clock: HostClock, offFailed: Boolean): Notification {
        val y = state.yolo
        val text = when {
            offFailed -> ctx.getString(R.string.yolo_off_failed)
            y == null || y.session -> ctx.getString(R.string.yolo_notif_session)
            else -> ctx.getString(R.string.yolo_notif_until, Formats.time(clock.toLocal(y.until), FormatStyle.SHORT))
        }
        val off = PendingIntent.getBroadcast(
            ctx, 8, Intent(ctx, YoloReceiver::class.java).setAction(YoloReceiver.ACTION_OFF), PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val open = PendingIntent.getActivity(ctx, 9, Intent(ctx, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
        return NotificationCompat.Builder(ctx, AdbuddyApp.CHANNEL_YOLO)
            .setSmallIcon(R.drawable.ic_stat_adbuddy)
            .setContentTitle(ctx.getString(R.string.yolo_notif_title))
            .setContentText(text)
            .setCategory(NotificationCompat.CATEGORY_STATUS)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setLocalOnly(true)
            .setShowWhen(false)
            .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
            .setContentIntent(open)
            // No authentication: turning it off is safe at any time, locked or not.
            .addAction(NotificationCompat.Action.Builder(0, ctx.getString(R.string.yolo_notif_off), off).setAuthenticationRequired(false).build())
            .build()
    }
}
