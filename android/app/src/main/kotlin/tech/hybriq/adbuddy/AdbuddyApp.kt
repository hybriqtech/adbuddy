package tech.hybriq.adbuddy

import android.app.Application
import android.app.NotificationChannel
import android.app.NotificationManager
import android.net.Uri
import java.io.File
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import tech.hybriq.adbuddy.a11y.UiHelpers
import kotlinx.coroutines.launch
import tech.hybriq.adbuddy.bridge.Bridge
import tech.hybriq.adbuddy.bridge.BridgeChannel
import tech.hybriq.adbuddy.bridge.Channel
import tech.hybriq.adbuddy.bridge.Notice
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.chat.ChatController
import tech.hybriq.adbuddy.confirm.HostClock
import tech.hybriq.adbuddy.files.Captures
import tech.hybriq.adbuddy.files.Downloads
import tech.hybriq.adbuddy.files.Drafts
import tech.hybriq.adbuddy.files.FileCache
import tech.hybriq.adbuddy.files.UploadFailed
import tech.hybriq.adbuddy.files.Uploader
import tech.hybriq.adbuddy.files.UriSource
import tech.hybriq.adbuddy.net.DeviceInfoSender
import tech.hybriq.adbuddy.term.TerminalFeed

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

    /** Screenshots of this phone, taken by the host. */
    lateinit var captures: Captures
        private set

    /** The attachments of the message being written. */
    lateinit var drafts: Drafts
        private set

    /** The attachments downloaded from the host. */
    lateinit var downloads: Downloads
        private set

    /** The terminal's stream, one per process (the model survives a rotation). */
    lateinit var terminal: TerminalFeed
        private set

    private val _notices = MutableSharedFlow<String>(extraBufferCapacity = 8)

    /** The text of each `notice` of the host (spec M2 section 4), for a snackbar: it is shown as the host wrote it. */
    val notices: SharedFlow<String> = _notices.asSharedFlow()

    /** The host's clock, which the questions' expiry is written in. */
    val clock = HostClock()

    private var deviceInfo: DeviceInfoSender? = null

    override fun onCreate() {
        super.onCreate()
        bridge = Bridge(scope, BuildConfigInfo.versionName(this)) { m, _ -> clock.observe(m) }
        UiHelpers.install(this)
        deviceInfo = DeviceInfoSender(this).also { it.start() }
        channel = BridgeChannel(bridge, scope)
        channel.listen(Protocol.NOTICE) { m -> Protocol.payload<Notice>(m)?.text?.takeIf { it.isNotEmpty() }?.let(_notices::tryEmit) }
        chat = ChatController(channel, scope)
        UriSource.clearCache(this)
        captures = Captures(channel)
        drafts = Drafts(scope, Uploader(channel), captures)
        downloads = Downloads(channel, FileCache(File(cacheDir, "downloads")), scope)
        terminal = TerminalFeed(channel)
        scope.launch { channel.connected.collect { terminal.onConnectedChanged(it) } }
        val nm = getSystemService(NotificationManager::class.java)
        nm.deleteNotificationChannel(LEGACY_STATUS)
        nm.createNotificationChannel(NotificationChannel(CHANNEL_CONVERSATION, getString(R.string.channel_conversation), NotificationManager.IMPORTANCE_LOW).apply { setAllowBubbles(true) })
        nm.createNotificationChannel(NotificationChannel(CHANNEL_DONE, getString(R.string.channel_done), NotificationManager.IMPORTANCE_DEFAULT))
        nm.createNotificationChannel(NotificationChannel(CHANNEL_CONFIRM, getString(R.string.channel_confirm), NotificationManager.IMPORTANCE_HIGH))
    }

    override fun onTerminate() {
        deviceInfo?.stop()
        super.onTerminate()
    }

    /**
     * A file or photo picked on the phone: copied, hashed and uploaded as a draft, off the main thread.
     * onFailed gets the host's text, or "" (the screen then says it in its own words); it runs off the main thread too.
     */
    fun attachUri(uri: Uri, onFailed: (String) -> Unit) {
        // The draft shows (and blocks Send) while the file is copied and hashed, not only once it uploads.
        val key = drafts.reserve()
        scope.launch {
            try {
                drafts.bind(key, UriSource.copy(this@AdbuddyApp, uri))
            } catch (e: UploadFailed) {
                drafts.remove(key)
                onFailed(e.text)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) { // a provider that fails (Drive offline, permission revoked) must not end the process
                drafts.remove(key)
                onFailed("")
            }
        }
    }

    companion object {
        /** The conversation notification (replaces "status"): bubbles allowed, low importance, never a sound. */
        const val CHANNEL_CONVERSATION = "conversation"
        const val CHANNEL_DONE = "done"
        const val CHANNEL_CONFIRM = "confirm"
        private const val LEGACY_STATUS = "status"
    }
}
