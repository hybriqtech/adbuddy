package tech.hybriq.adbuddy

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Person
import android.content.ComponentName
import android.content.Context
import android.os.Build
import android.os.Bundle
import android.os.SystemClock
import android.provider.Settings
import android.service.notification.NotificationListenerService
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith
import tech.hybriq.adbuddy.a11y.Packages
import tech.hybriq.adbuddy.a11y.ProtectedApps
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.UiLimits
import tech.hybriq.adbuddy.bridge.UiNotifications
import tech.hybriq.adbuddy.confirm.ConfirmNotifier
import tech.hybriq.adbuddy.listener.AgentNotificationListener
import tech.hybriq.adbuddy.listener.NotificationAccess
import tech.hybriq.adbuddy.listener.NotificationAccessStatus
import tech.hybriq.adbuddy.listener.NotificationConsent
import tech.hybriq.adbuddy.listener.NotificationGate
import tech.hybriq.adbuddy.listener.NotificationRead
import tech.hybriq.adbuddy.listener.NotificationReport
import tech.hybriq.adbuddy.listener.RawNotification

/**
 * T6 of the manual validations (M5c-22, 23, 26, 28, 29 and 32), the app's half. host/internal/emutest
 * (listener_test.go) allows and disallows the listener with `cmd notification`, kills the process, reboots and taps
 * ADBuddy's confirm button; this test reads, inside the app's process, what the app would read then:
 * - `-e adbuddyListener gate` reads the system's state as NotificationAccessStatus does, the user's mark and
 *   NotificationGate.read with the very lambdas of UiHelpers.install (the chain `adbuddy notifications` goes through),
 *   and reports them as instrumentation status lines `adbuddy.<name>=<value>` for the host to judge. With
 *   `-e unbind 1` it then asks the system to unbind the listener (requestUnbind, the access stays allowed: "allowed but
 *   not bound"), reads again, and asks it to bind it back (requestRebind).
 * - `-e adbuddyListener content` posts notifications of its own (BigText, MessagingStyle without EXTRA_TEXT,
 *   VISIBILITY_SECRET, ongoing, a 🔴 question as ConfirmNotifier posts it, and enough to pass 50) and checks what the
 *   real listener reads of them (AgentNotificationListener.read) and what NotificationReport.build makes of that:
 *   `cmd notification post` cannot post a BigText, a MessagingStyle without text nor a visibility (T1-2). ADBuddy's
 *   own notifications always come out hidden in the report, so the content checks build it from the same reads with
 *   the package of an app that is not protected ([OTHER]). It cancels all of them whatever happens.
 * It only runs with -e adbuddyListener, which only the host passes: in the whole suite (TestInstrumentedSuite) both
 * tests are skipped.
 */
@RunWith(AndroidJUnit4::class)
class NotificationListenerOnEmulatorTest {
    private val instrumentation get() = InstrumentationRegistry.getInstrumentation()
    private val context: Context get() = instrumentation.targetContext

    @Test fun gate() {
        val args = InstrumentationRegistry.getArguments()
        assumeTrue("only host/internal/emutest runs it", args.getString(ARG) == "gate")
        onlyOnAnEmulator()
        val waitMs = args.getString("waitMs")?.toLongOrNull() ?: 25_000L
        val out = Bundle()
        read(out, "", waitMs)
        instrumentation.sendStatus(STATUS_CODE, out)
    }

    /**
     * What the app reads now, into out as `adbuddy.<prefix><name>`: the setting as the app reads it, the system's own answer, the
     * two combined (NotificationAccessStatus.listed), whether the listener is bound (waiting up to waitMs for it when
     * the system lists it), the screen's state and effective state, the mark, and the gate.
     */
    private fun read(out: Bundle, prefix: String, waitMs: Long) {
        val setting = try {
            Settings.Secure.getString(context.contentResolver, "enabled_notification_listeners").let {
                when {
                    it == null -> "null"
                    it.isEmpty() -> "empty"
                    else -> NotificationAccess.listed(it).name
                }
            }
        } catch (e: RuntimeException) {
            "exception:" + e.javaClass.simpleName
        }
        val me = ComponentName(context, AgentNotificationListener::class.java)
        val api = try {
            context.getSystemService(NotificationManager::class.java).isNotificationListenerAccessGranted(me).toString()
        } catch (e: RuntimeException) {
            "exception:" + e.javaClass.simpleName
        }
        val listed = NotificationAccessStatus.listed(context)
        if (waitMs > 0 && listed == NotificationAccess.Listed.YES) awaitConnected(true, waitMs)
        val connected = AgentNotificationListener.connected.value
        val state = NotificationAccess.state(listed, connected)
        val consent = NotificationConsent.of(context)
        val mark = consent.confirmed()
        // The gate as UiHelpers.install builds it (UiHelpers.kt): the same four lambdas.
        val gate = when (val r = NotificationGate.read(consent::confirmed, { NotificationAccessStatus.listed(context) }, consent::revoke, AgentNotificationListener::read)) {
            is NotificationRead.Ok -> "Ok:${r.items.size}"
            NotificationRead.NoAccess -> "NoAccess"
            NotificationRead.NotConfirmed -> "NotConfirmed"
        }
        out.putString("$KEY.${prefix}setting", setting)
        out.putString("$KEY.${prefix}api", api)
        out.putString("$KEY.${prefix}listed", listed?.name ?: "null")
        out.putString("$KEY.${prefix}connected", connected.toString())
        out.putString("$KEY.${prefix}state", state.name)
        out.putString("$KEY.${prefix}mark", mark.toString())
        out.putString("$KEY.${prefix}effective", NotificationAccess.effective(state, mark).name)
        out.putString("$KEY.${prefix}gate", gate)
        out.putString("$KEY.${prefix}markAfter", consent.confirmed().toString())
    }

    private fun onlyOnAnEmulator() {
        // Never on a phone, whoever starts it: the emulator's hardware is ranchu (goldfish before it).
        assertTrue("not an emulator (Build.HARDWARE=${Build.HARDWARE}): the listener's tests only run on one",
            Build.HARDWARE == "ranchu" || Build.HARDWARE == "goldfish")
    }

    private fun builder() = Notification.Builder(context, CHANNEL).setSmallIcon(android.R.drawable.ic_dialog_info)

    private fun post(nm: NotificationManager, id: Int, n: Notification) {
        nm.notify(id, n)
        SystemClock.sleep(250)
    }

    /** The read of this app's notification with id (key user|package|id|tag|uid, no tag here). */
    private fun mine(raws: List<RawNotification>, id: Int) = raws.firstOrNull { it.pkg == Packages.ADBUDDY && it.key.contains("|${Packages.ADBUDDY}|$id|null|") }

    private fun question(raws: List<RawNotification>) =
        raws.firstOrNull { it.pkg == Packages.ADBUDDY && it.key.contains("|${Packages.ADBUDDY}|${ConfirmNotifier.NOTIFICATION_ID}|${QUESTION.id}|") }

    /** What the report would say of n had another app, not protected, posted it. */
    private fun asOther(n: RawNotification, guarded: (String) -> Boolean) =
        NotificationReport.build(listOf(n.copy(pkg = OTHER)), guarded).notifications.single()

    private fun awaitRead(what: String, ok: (List<RawNotification>) -> Boolean): List<RawNotification> {
        val deadline = SystemClock.elapsedRealtime() + 20_000L
        var last: List<RawNotification>? = null
        while (SystemClock.elapsedRealtime() < deadline) {
            last = AgentNotificationListener.read()
            if (last != null && ok(last)) return last
            SystemClock.sleep(250)
        }
        throw AssertionError("the listener did not read $what in 20 s (${last?.size ?: "no"} notifications read)")
    }

    /** Waits up to ms for the listener to be bound (want) or not; whether it is as wanted. */
    private fun awaitConnected(want: Boolean, ms: Long): Boolean {
        val deadline = SystemClock.elapsedRealtime() + ms
        while (AgentNotificationListener.connected.value != want && SystemClock.elapsedRealtime() < deadline) SystemClock.sleep(100)
        return AgentNotificationListener.connected.value == want
    }

    /** The bound listener, which only the system and the listener's own package may unbind. */
    private fun listener(): NotificationListenerService? {
        // AgentNotificationListener keeps it private on purpose (nothing else may touch the service): only this test reaches it.
        val field = AgentNotificationListener::class.java.getDeclaredField("current")
        field.isAccessible = true
        return field.get(null) as NotificationListenerService?
    }

    private companion object {
        const val ARG = "adbuddyListener"
        /** The prefix of the gate's status keys, which the host reads. */
        const val KEY = "adbuddy"
        /** Status lines of the gate's reads (not a test's start, end or result: those are 1, 0 and negative). */
        const val STATUS_CODE = 2
        const val CHANNEL = "adbuddy_listener_test"
        const val OTHER = "com.example.t6.other"
        const val BIG_ID = 7401
        const val CHAT_ID = 7402
        const val SECRET_ID = 7403
        const val ONGOING_ID = 7404
        const val PLAIN_ID = 7405
        const val FLOOD_ID = 7450
        const val LONG_ID = 7406
        val MINE = listOf(BIG_ID, CHAT_ID, SECRET_ID, ONGOING_ID, PLAIN_ID, LONG_ID)
        const val BIG_TITLE = "T6 big text"
        const val SHORT_TEXT = "The short line of the big text"
        // 1000 characters, ASCII (one code point each), its end marked: far longer than the short line.
        val BIG_TEXT = (1..40).joinToString(" ") { "line%02d-abcdefghijklmn".format(it) }.padEnd(990, '.') + "END-OF-BIG"
        // 1500: longer than what Android keeps of a text and than what the report keeps.
        val LONG_TEXT = (1..60).joinToString(" ") { "long%02d-abcdefghijklmn".format(it) }.padEnd(1490, '.') + "END-OF-BIG"
        const val CHAT_TITLE = "T6 group"
        const val CHAT_MESSAGE = "The chat message 518204"
        const val SECRET_TITLE = "T6 secret title"
        const val SECRET_TEXT = "T6 secret code 640271"
        const val ONGOING_TITLE = "T6 ongoing"
        const val PLAIN_TITLE = "T6 plain"
        const val PLAIN_TEXT = "A plain notification of the listener test"
        /** What the host posts with `cmd notification post -t`: the same in listener_test.go. */
        const val FILLER_TITLE = "T6 filler"
        val QUESTION = ConfirmRequest(
            id = "j_t6",
            title = "Restart the phone",
            detail = "may restart",
            rawCommand = "adb shell reboot",
            expiresAt = Instant.now().plusSeconds(120).toString(),
        )
    }
}
