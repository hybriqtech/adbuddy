package tech.hybriq.adbuddy.bridge

import android.app.Notification
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.lifecycle.LifecycleService
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.launch
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.confirm.ConfirmNotifier

/**
 * Keeps the channel with the host open (spec §6: foreground service of type
 * specialUse) and shows its state in a persistent notification with Pause.
 *
 * It does not override dump(), and must not: `dumpsys activity service` is
 * 🟢 for the agent, so anything dumped here would reach it.
 */
class BridgeService : LifecycleService() {

    override fun onCreate() {
        super.onCreate()
        // specialUse exists from Android 14; before that the service has no type.
        val type = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE else 0
        ServiceCompat.startForeground(this, NOTIFICATION_ID, statusNotification(BridgeState()), type)
        val app = application as AdbuddyApp
        lifecycleScope.launch {
            app.bridge.state.collect { s ->
                getSystemService(NotificationManager::class.java).notify(NOTIFICATION_ID, statusNotification(s))
                ConfirmNotifier.sync(this@BridgeService, s)
            }
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        super.onStartCommand(intent, flags, startId)
        val app = application as AdbuddyApp
        when (intent?.action) {
            ACTION_PAUSE -> app.bridge.pause(true)
            ACTION_RESUME -> app.bridge.pause(false)
            // Bridge.start is idempotent: the same link again (the app reopened, the
            // screen rotated) keeps the connection; a new one reconnects.
            else -> BridgePrefs(this).link?.let { app.bridge.start(it) } ?: stopSelf()
        }
        return START_STICKY
    }

    override fun onDestroy() {
        (application as AdbuddyApp).bridge.stop()
        // Nobody can answer the questions without the channel.
        ConfirmNotifier.clear(this)
        super.onDestroy()
    }

    private fun statusNotification(s: BridgeState): Notification {
        val text = when {
            s.connection == BridgeState.Connection.INCOMPATIBLE && s.outdated == BridgeState.Outdated.APP -> getString(R.string.status_update_app)
            s.connection == BridgeState.Connection.INCOMPATIBLE -> getString(R.string.status_update_host)
            s.sessionState.paused -> getString(R.string.status_paused)
            s.connection == BridgeState.Connection.CONNECTED -> getString(R.string.status_connected, s.hostName)
            s.connection == BridgeState.Connection.CONNECTING -> getString(R.string.status_connecting)
            else -> getString(R.string.status_waiting)
        }
        val open = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
        val b = NotificationCompat.Builder(this, AdbuddyApp.CHANNEL_STATUS)
            .setSmallIcon(android.R.drawable.stat_notify_sync_noanim)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setContentIntent(open)
            .setOngoing(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
        if (s.connection == BridgeState.Connection.CONNECTED) {
            val paused = s.sessionState.paused
            val action = if (paused) ACTION_RESUME else ACTION_PAUSE
            val pi = PendingIntent.getService(this, if (paused) 2 else 1,
                Intent(this, BridgeService::class.java).setAction(action), PendingIntent.FLAG_IMMUTABLE)
            b.addAction(0, getString(if (paused) R.string.action_resume else R.string.action_pause), pi)
        }
        return b.build()
    }

    companion object {
        const val NOTIFICATION_ID = 1
        const val ACTION_PAUSE = "tech.hybriq.adbuddy.action.PAUSE"
        const val ACTION_RESUME = "tech.hybriq.adbuddy.action.RESUME"

        /** Starts the service, or hands it the link again (Bridge.start ignores the same link). */
        fun start(context: Context) {
            context.startForegroundService(Intent(context, BridgeService::class.java))
        }
    }
}
