package tech.hybriq.adbuddy

import android.app.Application
import android.app.NotificationChannel
import android.app.NotificationManager
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import tech.hybriq.adbuddy.a11y.UiHelpers
import tech.hybriq.adbuddy.bridge.Bridge
import tech.hybriq.adbuddy.bridge.BridgeChannel
import tech.hybriq.adbuddy.bridge.Channel
import tech.hybriq.adbuddy.chat.ChatController
import tech.hybriq.adbuddy.confirm.HostClock

class AdbuddyApp : Application() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    /** The channel with the host, shared by the service, the receivers and the UI. */
    lateinit var bridge: Bridge
        private set

    /** The channel as the controllers see it. */
    lateinit var channel: Channel
        private set

    /** The messages written on this phone (spec M3 §7.2). */
    lateinit var chat: ChatController
        private set

    /** The host's clock, which the questions' expiry is written in. */
    val clock = HostClock()

    override fun onCreate() {
        super.onCreate()
        bridge = Bridge(scope, BuildConfigInfo.versionName(this)) { m, _ -> clock.observe(m) }
        UiHelpers.install(this)
        channel = BridgeChannel(bridge, scope)
        chat = ChatController(channel)
        val nm = getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(NotificationChannel(CHANNEL_STATUS, getString(R.string.channel_status), NotificationManager.IMPORTANCE_LOW))
        nm.createNotificationChannel(NotificationChannel(CHANNEL_CONFIRM, getString(R.string.channel_confirm), NotificationManager.IMPORTANCE_HIGH))
    }

    companion object {
        const val CHANNEL_STATUS = "status"
        const val CHANNEL_CONFIRM = "confirm"
    }
}
