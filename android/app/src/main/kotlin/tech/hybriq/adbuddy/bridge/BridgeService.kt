package tech.hybriq.adbuddy.bridge

import android.app.KeyguardManager
import android.app.Notification
import android.app.NotificationManager
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.util.Log
import android.widget.Toast
import androidx.annotation.StringRes
import androidx.core.app.RemoteInput
import androidx.core.app.ServiceCompat
import androidx.lifecycle.LifecycleService
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.launch
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.confirm.ConfirmNotifier
import tech.hybriq.adbuddy.control.CutAccess
import tech.hybriq.adbuddy.notify.ConversationNotifier
import tech.hybriq.adbuddy.notify.ConversationView
import tech.hybriq.adbuddy.notify.DoneDetector
import tech.hybriq.adbuddy.notify.Notify

/**
 * Keeps the channel with the host open (spec §6: foreground service of type
 * specialUse) and shows the conversation in a persistent notification (spec M3
 * §7.4) with Reply, Send screenshot, Pause and Cut access.
 *
 * It does not override dump(), and must not: `dumpsys activity service` is
 * 🟢 for the agent, so anything dumped here would reach it.
 */
class BridgeService : LifecycleService() {
    private var foreground = false
    private var shown: ConversationView? = null
    private var lastId: String? = null
    private var lastTs = 0L
    private val main = Handler(Looper.getMainLooper())

    override fun onCreate() {
        super.onCreate()
        val first = ConversationView.of(BridgeState(), CutAccess.isCut(contentResolver))
        foreground = goForeground(conversationNotification(first))
        if (!foreground) {
            stopSelf()
            return
        }
        shown = first
        // Questions left by an earlier process: nobody can answer them now; the host sends them again.
        ConfirmNotifier.clear(this)
        val app = application as AdbuddyApp
        lifecycleScope.launch {
            app.bridge.state.collect { s -> ConfirmNotifier.sync(this@BridgeService, s) }
        }
        lifecycleScope.launch {
            combine(app.bridge.state, CutAccess.observe(this@BridgeService)) { s, cut -> ConversationView.of(s, cut) }.collect { v ->
                // Posted again only when what it shows changes, not on every journal entry.
                if (v != shown) post(v)
            }
        }
        lifecycleScope.launch {
            val detector = DoneDetector()
            app.bridge.state.collect { s ->
                if (s.connection != BridgeState.Connection.CONNECTED) {
                    detector.reset()
                } else if (detector.onState(s.sessionState, SystemClock.elapsedRealtime())) {
                    ConversationNotifier.done(this@BridgeService, Notify.doneLine(s.chat, s.sessionState.conversation))
                }
            }
        }
    }

    private fun post(v: ConversationView) {
        shown = v
        getSystemService(NotificationManager::class.java).notify(NOTIFICATION_ID, conversationNotification(v))
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
            // Reply and capture put something in the agent's hands: never with the phone locked (API 30 has no
            // setAuthenticationRequired, and the lock screen must not be a way in on any version).
            ACTION_REPLY -> {
                if (!locked()) {
                    val text = RemoteInput.getResultsFromIntent(intent)?.getCharSequence(ConversationNotifier.KEY_REPLY)?.toString()
                    if (!text.isNullOrBlank()) app.chat.send(text)
                }
                // The "sending" wheel of the direct reply stays until the notification is posted again (also when it was ignored).
                shown?.let { post(it) }
            }
            ACTION_CAPTURE -> if (!locked()) {
                app.captures.take { a, text ->
                    if (a != null) app.chat.send("", listOf(a)) else toast(text)
                }
            }
            // Cutting needs no unlock: it only narrows what the agent can do.
            ACTION_CUT -> if (!CutAccess.cut(this)) toast(R.string.notif_cut_failed)
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

    private fun locked() = getSystemService(KeyguardManager::class.java).isDeviceLocked

    private fun toast(@StringRes res: Int) = toast(getString(res))

    /** Toasts go through the main thread. The text is the host's or ours, shown as is. */
    private fun toast(text: String?) {
        val t = text?.takeIf { it.isNotEmpty() } ?: getString(R.string.notif_capture_failed)
        main.post { Toast.makeText(applicationContext, t, Toast.LENGTH_LONG).show() }
    }

    private fun conversationNotification(v: ConversationView): Notification {
        val id = v.last?.id
        if (id != lastId) {
            lastId = id
            lastTs = System.currentTimeMillis()
        }
        return ConversationNotifier.build(this, v, lastTs)
    }

    companion object {
        const val NOTIFICATION_ID = 1
        const val ACTION_PAUSE = "tech.hybriq.adbuddy.action.PAUSE"
        const val ACTION_RESUME = "tech.hybriq.adbuddy.action.RESUME"
        const val ACTION_REPLY = "tech.hybriq.adbuddy.action.REPLY"
        const val ACTION_CAPTURE = "tech.hybriq.adbuddy.action.CAPTURE"
        const val ACTION_CUT = "tech.hybriq.adbuddy.action.CUT"
        private const val TAG = "ADBuddy"

        /** Starts the service, or hands it the link again (Bridge.start ignores the same link). */
        fun start(context: Context) {
            context.startForegroundService(Intent(context, BridgeService::class.java))
        }
    }
}
