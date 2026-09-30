package tech.hybriq.adbuddy.bridge

import android.app.Notification
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.lifecycle.LifecycleService
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.launch
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.confirm.ConfirmNotifier
import tech.hybriq.adbuddy.control.CutAccess
import tech.hybriq.adbuddy.ui.LinkStatus
import tech.hybriq.adbuddy.ui.PauseButton
import tech.hybriq.adbuddy.ui.StatusView

/**
 * Keeps the channel with the host open (spec §6: foreground service of type
 * specialUse) and shows its state in a persistent notification with Pause.
 *
 * It does not override dump(), and must not: `dumpsys activity service` is
 * 🟢 for the agent, so anything dumped here would reach it.
 */
class BridgeService : LifecycleService() {
    private var foreground = false
    private var shownStatus: StatusView? = null

    override fun onCreate() {
        super.onCreate()
        val first = StatusView.of(BridgeState(), CutAccess.isCut(contentResolver))
        foreground = goForeground(statusNotification(first))
        if (!foreground) {
            stopSelf()
            return
        }
        shownStatus = first
        // Questions left by an earlier process: nobody can answer them now; the host sends them again.
        ConfirmNotifier.clear(this)
        val app = application as AdbuddyApp
        lifecycleScope.launch {
            app.bridge.state.collect { s -> ConfirmNotifier.sync(this@BridgeService, s) }
        }
        lifecycleScope.launch {
            combine(app.bridge.state, CutAccess.observe(this@BridgeService)) { s, cut -> StatusView.of(s, cut) }.collect { v ->
                // Posted again only when what it shows changes, not on every journal entry.
                if (v != shownStatus) {
                    shownStatus = v
                    getSystemService(NotificationManager::class.java).notify(NOTIFICATION_ID, statusNotification(v))
                }
            }
        }
    }

    /**
     * startForeground, or false when Android refuses it (from the background,
     * Android 12+: ForegroundServiceStartNotAllowedException, an
     * IllegalStateException): then the service stops, and opening the app
     * starts it again.
     */
    private fun goForeground(n: Notification): Boolean = try {
        // specialUse exists from Android 14; before that the service has no type.
        val type = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE else 0
        ServiceCompat.startForeground(this, NOTIFICATION_ID, n, type)
        true
    } catch (e: IllegalStateException) {
        Log.w(TAG, "could not go to the foreground", e)
        false
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        super.onStartCommand(intent, flags, startId)
        if (!foreground) return START_NOT_STICKY
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
        if (foreground) {
            (application as AdbuddyApp).bridge.stop()
            // Nobody can answer the questions without the channel.
            ConfirmNotifier.clear(this)
        }
        super.onDestroy()
    }

    private fun statusNotification(v: StatusView): Notification {
        val text = if (v.status == LinkStatus.CONNECTED) getString(v.status.text, v.host) else getString(v.status.text)
        val open = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
        val b = NotificationCompat.Builder(this, AdbuddyApp.CHANNEL_STATUS)
            .setSmallIcon(android.R.drawable.stat_notify_sync_noanim)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setContentIntent(open)
            .setOngoing(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
        v.button?.let { button ->
            val action = if (button == PauseButton.RESUME) ACTION_RESUME else ACTION_PAUSE
            val pi = PendingIntent.getService(this, if (button == PauseButton.RESUME) 2 else 1,
                Intent(this, BridgeService::class.java).setAction(action), PendingIntent.FLAG_IMMUTABLE)
            b.addAction(0, getString(button.text), pi)
        }
        return b.build()
    }

    companion object {
        const val NOTIFICATION_ID = 1
        const val ACTION_PAUSE = "tech.hybriq.adbuddy.action.PAUSE"
        const val ACTION_RESUME = "tech.hybriq.adbuddy.action.RESUME"
        private const val TAG = "ADBuddy"

        /** Starts the service, or hands it the link again (Bridge.start ignores the same link). */
        fun start(context: Context) {
            context.startForegroundService(Intent(context, BridgeService::class.java))
        }
    }
}
