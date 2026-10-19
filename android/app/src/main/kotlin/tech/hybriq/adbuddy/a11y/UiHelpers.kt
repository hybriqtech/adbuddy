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

    /**
     * Sends the protected apps to the host in device.info (spec §4.3). false when the host is not
     * reachable: it gets the list with the next welcome. Quick (no socket wait): safe to call from the UI.
     */
    fun publishProtectedApps(app: AdbuddyApp): Boolean = synchronized(ProtectedApps.LOCK) {
        // Read and send under the process lock that propose/confirm take: two publishes (helpers thread and screen)
        // reach the host in the order of the list they read, never an old list after a newer one. Bridge.send only
        // queues a frame (no socket wait), so holding the lock does not block the UI on the network.
        app.bridge.send(Protocol.DEVICE_INFO, Protocol.json.encodeToJsonElement(DeviceInfo.serializer(), ProtectedApps.of(app).deviceInfo()))
    }

    fun install(app: AdbuddyApp) {
        val protected = ProtectedApps.of(app)
        val consent = NotificationConsent.of(app)
        val work = Executors.newSingleThreadExecutor { Thread(it, "adbuddy-helpers") }
        val yoloOn = { YoloProtection.yoloOn(app.bridge.state.value) }
        val ui = UiExecutor(
            screen = { AgentAccessibilityService.current },
            send = { app.bridge.send(Protocol.UI_RESPONSE, UiExecutor.encode(it)) },
            now = app.clock::now,
            isProtected = YoloProtection.effective(yoloOn, protected::isProtected),
            questionWaiting = { app.bridge.state.value.pending.isNotEmpty() },
            paused = { app.bridge.state.value.sessionState.paused },
            notificationOnScreen = YoloProtection.notificationBlocks(yoloOn, AgentAccessibilityService.notifications::active),
            executor = work,
            notifications = {
                NotificationGate.read(consent::confirmed, { NotificationAccessStatus.listed(app) }, consent::revoke, AgentNotificationListener::read)
            },
        )
        fun sendList() {
            publishProtectedApps(app)
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
                val defaultSms = ProtectedApps.defaultSms(app)
                val receivers = ProtectedApps.Heuristic.receivers(defaultSms)
                val read = try {
                    ProtectedApps.launcherApps(app)
                } catch (e: RuntimeException) {
                    Log.w(TAG, "launcher apps", e)
                    null
                }
                // An empty read, or one without ADBuddy (which always has an icon), is a failure, not "no apps".
                val launcher = read?.takeIf(ProtectedApps.Heuristic::launcherUsable)
                if (read != null && launcher == null) Log.w(TAG, "launcher apps: unusable read (${read.size} apps)")
                // If the launcher cannot be read, at least what needs no launcher icon is protected (and stays so).
                val changed = if (launcher != null) {
                    protected.propose(launcher, receivers)
                } else {
                    protected.ensure(ProtectedApps.Heuristic.fallback(receivers, defaultSms))
                }
                if (changed) sendList()
            } catch (e: RuntimeException) {
                Log.w(TAG, "protected apps", e)
            }
        }
    }
}
