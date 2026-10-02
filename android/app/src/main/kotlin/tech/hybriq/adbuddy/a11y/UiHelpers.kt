package tech.hybriq.adbuddy.a11y

import android.util.Log
import java.util.concurrent.Executors
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.bridge.DeviceInfo
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.listener.AgentNotificationListener
import tech.hybriq.adbuddy.listener.NotificationAccessStatus
import tech.hybriq.adbuddy.listener.NotificationConsent
import tech.hybriq.adbuddy.listener.NotificationGate

/**
 * Connects the agent's helpers to the channel with the host (M5 spec §2):
 * ui.request goes to UiExecutor, and on each welcome the app sends the
 * protected apps in device.info (spec §4.3). The work runs on one thread of
 * its own, never on the socket's reader nor the main one.
 */
object UiHelpers {
    private const val TAG = "ADBuddyHelpers"

    /**
     * The WELCOME listener: the list goes out on work, and whatever sendList
     * throws is logged (an uncaught exception on the helpers thread would kill the app).
     */
    internal fun welcomeHandler(work: java.util.concurrent.Executor, sendList: () -> Unit, log: (RuntimeException) -> Unit): (tech.hybriq.adbuddy.bridge.Message) -> Unit = {
        work.execute {
            try {
                sendList()
            } catch (e: RuntimeException) {
                log(e)
            }
        }
    }

    fun install(app: AdbuddyApp) {
        val protected = ProtectedApps.of(app)
        val consent = NotificationConsent.of(app)
        val work = Executors.newSingleThreadExecutor { Thread(it, "adbuddy-helpers") }
        val ui = UiExecutor(
            screen = { AgentAccessibilityService.current },
            send = { app.bridge.send(Protocol.UI_RESPONSE, UiExecutor.encode(it)) },
            now = app.clock::now,
            isProtected = protected::isProtected,
            questionWaiting = { app.bridge.state.value.pending.isNotEmpty() },
            paused = { app.bridge.state.value.sessionState.paused },
            notificationOnScreen = AgentAccessibilityService.notifications::active,
            executor = work,
            notifications = {
                NotificationGate.read(consent::confirmed, { NotificationAccessStatus.listed(app) }, consent::revoke, AgentNotificationListener::read)
            },
        )
        fun sendList() {
            val info = DeviceInfo(protectedApps = protected.list().sorted())
            app.bridge.send(Protocol.DEVICE_INFO, Protocol.json.encodeToJsonElement(DeviceInfo.serializer(), info))
        }
        app.bridge.listen(Protocol.UI_REQUEST) { m ->
            try {
                ui.onMessage(m)
            } catch (e: RuntimeException) {
                Log.w(TAG, "ui.request", e)
            }
        }
        app.bridge.listen(Protocol.WELCOME, welcomeHandler(work, ::sendList) { Log.w(TAG, "welcome", it) })
        // The first proposal (and a new app that looks like a bank) reaches the
        // host at once if it is connected; otherwise with the next welcome.
        work.execute {
            try {
                if (protected.propose(ProtectedApps.launcherApps(app))) sendList()
            } catch (e: RuntimeException) {
                Log.w(TAG, "protected apps", e)
            }
        }
    }
}
