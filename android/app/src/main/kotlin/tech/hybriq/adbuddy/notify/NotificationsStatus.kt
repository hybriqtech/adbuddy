package tech.hybriq.adbuddy.notify

import android.app.NotificationManager
import android.content.Context
import android.content.Intent
import android.provider.Settings
import androidx.core.app.NotificationManagerCompat
import tech.hybriq.adbuddy.AdbuddyApp

/** Whether ADBuddy can tell the user something is waiting: notifications on, and the confirm and ask channels not silenced. */
object NotificationsStatus {
    data class State(val appEnabled: Boolean, val channelBlocked: Boolean) {
        val blocked: Boolean get() = !appEnabled || channelBlocked
    }

    fun of(ctx: Context): State {
        val nm = NotificationManagerCompat.from(ctx)
        // A channel that does not exist is not blocked.
        val silenced = listOf(AdbuddyApp.CHANNEL_CONFIRM, AdbuddyApp.CHANNEL_ASK)
            .any { nm.getNotificationChannel(it)?.importance == NotificationManager.IMPORTANCE_NONE }
        return State(appEnabled = nm.areNotificationsEnabled(), channelBlocked = silenced)
    }
}

/** The app's notification settings; the general settings when the system has no such screen. */
fun openNotificationSettings(ctx: Context) {
    // No settings screen at all: nothing to open, and the app must not close.
    ctx.startFirst(
        Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, ctx.packageName),
        Intent(Settings.ACTION_SETTINGS),
    )
}
