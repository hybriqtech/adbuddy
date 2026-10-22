package tech.hybriq.adbuddy.notify

import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R

internal val Context.notificationManager: NotificationManager
    get() = getSystemService(NotificationManager::class.java)

/** Tapping a notification opens the app. */
internal fun openAppIntent(ctx: Context, requestCode: Int): PendingIntent =
    PendingIntent.getActivity(ctx, requestCode, Intent(ctx, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)

/** The builder every notification of the app starts from: its icon, and (when [color]) its accent color. */
internal fun baseBuilder(ctx: Context, channel: String, color: Boolean = true): NotificationCompat.Builder {
    val b = NotificationCompat.Builder(ctx, channel).setSmallIcon(R.drawable.ic_stat_adbuddy)
    if (color) b.setColor(ContextCompat.getColor(ctx, R.color.ic_launcher_background))
    return b
}
