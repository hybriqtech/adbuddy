package tech.hybriq.adbuddy

import android.app.Activity
import android.app.Application
import android.os.Bundle
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import android.content.res.Configuration
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
import tech.hybriq.adbuddy.bridge.ErrorText
import tech.hybriq.adbuddy.bridge.ErrorTexts
import tech.hybriq.adbuddy.bridge.Notice
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.chat.ChatController
import tech.hybriq.adbuddy.confirm.HostClock
import tech.hybriq.adbuddy.control.CutAccess
import tech.hybriq.adbuddy.control.CutCheck
import tech.hybriq.adbuddy.files.Captures
import tech.hybriq.adbuddy.files.Downloads
import tech.hybriq.adbuddy.files.Drafts
import tech.hybriq.adbuddy.files.FileCache
import tech.hybriq.adbuddy.files.UploadFailed
import tech.hybriq.adbuddy.files.Uploader
import tech.hybriq.adbuddy.files.UriSource
import tech.hybriq.adbuddy.net.DeviceInfoSender
import tech.hybriq.adbuddy.term.TerminalFeed
import tech.hybriq.adbuddy.ui.theme.ThemePrefs

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

    private val _cutCheck = MutableStateFlow<CutCheck?>(null)

    /**
     * What the check after a cut found (R22), kept here and not in the screen: turning the phone or going back within
     * the wait must not lose the "the PC is still connected" notice. The screen clears it once shown.
     */
    val cutCheck: StateFlow<CutCheck?> = _cutCheck.asStateFlow()

    /** Checks the cut that was just made (R22); [wasUp]: the channel was alive at the tap. */
    fun confirmCut(wasUp: Boolean) {
        scope.launch {
            _cutCheck.value = CutAccess.confirm(this@AdbuddyApp, wasUp) { CutAccess.channelUp(bridge.state.value) }
        }
    }

    fun cutCheckShown() {
        _cutCheck.value = null
    }

    /** The host's clock, which the questions' expiry is written in. */
    val clock = HostClock()

    /** What the person reads of a refusal of the host: the app's own text for its code, in the language in force (ErrorText). */
    val errorTexts: ErrorTexts = { ErrorText.text({ id -> getString(id) }, it) }

    /** Settings, "Use wallpaper colors": read once, then the activities follow it. */
    val dynamicColor by lazy { MutableStateFlow(ThemePrefs(this).dynamic) }

    private val started = AtomicInteger(0)
    private val _foreground = MutableStateFlow(false)

    /** True while at least one activity of the app is started (the bubble counts): the person is looking at the app. */
    val foreground: StateFlow<Boolean> = _foreground.asStateFlow()

    private var deviceInfo: DeviceInfoSender? = null

    override fun onCreate() {
        super.onCreate()
        bridge = Bridge(scope, BuildConfigInfo.versionName(this)) { m, _ -> clock.observe(m) }
        UiHelpers.install(this)
        deviceInfo = DeviceInfoSender(this).also { it.start() }
        channel = BridgeChannel(bridge, scope)
        channel.listen(Protocol.NOTICE) { m -> Protocol.payload<Notice>(m)?.text?.takeIf { it.isNotEmpty() }?.let(_notices::tryEmit) }
        chat = ChatController(channel, scope, errorText = errorTexts)
        UriSource.clearCache(this)
        captures = Captures(channel, errorTexts)
        drafts = Drafts(scope, Uploader(channel), captures, errorTexts)
        downloads = Downloads(channel, FileCache(File(cacheDir, "downloads")), scope, errorText = errorTexts)
        terminal = TerminalFeed(channel)
        scope.launch { channel.connected.collect { terminal.onConnectedChanged(it) } }
        createNotificationChannels()
        registerActivityLifecycleCallbacks(object : ActivityLifecycleCallbacks {
            override fun onActivityStarted(a: Activity) { started.incrementAndGet(); _foreground.value = true }
            override fun onActivityStopped(a: Activity) {
                val n = started.decrementAndGet()
                // A recreation (rotation, dark mode, language) stops the activity and starts it again at once: it stays in front.
                if (n > 0 || !a.isChangingConfigurations) _foreground.value = n > 0
            }
            override fun onActivityCreated(a: Activity, savedInstanceState: Bundle?) {}
            override fun onActivityResumed(a: Activity) {}
            override fun onActivityPaused(a: Activity) {}
            override fun onActivitySaveInstanceState(a: Activity, outState: Bundle) {}
            override fun onActivityDestroyed(a: Activity) {}
        })
    }

    /**
     * The channels carry their name in the language of the moment they are created; creating one that exists only
     * updates its name. So they are created again when the language changes (the system language or, on Android 13
     * and later, the app's own), which reaches here without the process restarting. BridgeService repaints its own
     * notification by itself (its onConfigurationChanged).
     */
    override fun onConfigurationChanged(newConfig: Configuration) {
        super.onConfigurationChanged(newConfig)
        createNotificationChannels()
    }

    private fun createNotificationChannels() {
        val nm = getSystemService(NotificationManager::class.java)
        nm.deleteNotificationChannel(LEGACY_STATUS)
        nm.createNotificationChannel(NotificationChannel(CHANNEL_CONVERSATION, getString(R.string.channel_conversation), NotificationManager.IMPORTANCE_LOW).apply {
            description = getString(R.string.channel_conversation_description)
            setAllowBubbles(true)
        })
        nm.createNotificationChannel(NotificationChannel(CHANNEL_DONE, getString(R.string.channel_done), NotificationManager.IMPORTANCE_DEFAULT).apply {
            description = getString(R.string.channel_done_description)
        })
        nm.createNotificationChannel(NotificationChannel(CHANNEL_CONFIRM, getString(R.string.channel_confirm), NotificationManager.IMPORTANCE_HIGH).apply {
            description = getString(R.string.channel_confirm_description)
        })
        nm.createNotificationChannel(NotificationChannel(CHANNEL_ASK, getString(R.string.channel_ask), NotificationManager.IMPORTANCE_HIGH).apply {
            description = getString(R.string.channel_ask_description)
            setShowBadge(false)
        })
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
                onFailed(e.display(errorTexts))
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
        const val CHANNEL_ASK = "ask"
        private const val LEGACY_STATUS = "status"
    }
}
