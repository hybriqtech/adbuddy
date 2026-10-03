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
 *   not bound"), reads again, and asks it to bind it back (requestRebind). am instrument restarts the app's process
 *   and the system binds the listener anew in it, and every new binding clears the user's mark (onListenerConnected,
 *   M5c-32 b): what this half reads as `mark` is false whatever the host did before, so it never proves that a step
 *   cleared it (the host reads the app's file or asks over the real channel for that). With `-e confirm 1` it first
 *   confirms in its own process, once the listener is bound, as the screen's button does. Only the main sources are
 *   held to "nothing but the button sets the mark" (NotificationGateTest.nothingButTheButtonSetsTheMark scans
 *   src/main): this test may call confirm().
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
        if (args.getString("confirm") == "1") {
            // am instrument restarted the process and the system bound the listener in it again, which cleared the
            // user's mark (onListenerConnected, M5c-32 b): a tap on the host's side does not reach this process. Once
            // the listener is bound (as the screen's button needs), this process confirms itself and reads.
            out.putString("$KEY.fresh.connected", awaitConnected(true, waitMs).toString())
            out.putString("$KEY.fresh.mark", NotificationConsent.of(context).confirmed().toString())
            NotificationConsent.of(context).confirm()
        }
        read(out, "", waitMs)
        if (args.getString("unbind") == "1") {
            // Allowed in the system but not bound, as after a reboot or a dead process the system has not bound
            // again yet: the listener asks to be unbound, the access stays allowed (the setting keeps it).
            val service = listener()
            assertNotNull("no bound listener to unbind", service)
            service!!.requestUnbind()
            out.putString("$KEY.unbound.disconnected", awaitConnected(false, 10_000L).toString())
            read(out, "unbound.", 0L)
            val started = SystemClock.elapsedRealtime()
            out.putString("$KEY.unbound.read", AgentNotificationListener.read()?.size?.toString() ?: "null")
            out.putString("$KEY.unbound.readMs", (SystemClock.elapsedRealtime() - started).toString())
            NotificationListenerService.requestRebind(ComponentName(context, AgentNotificationListener::class.java))
            read(out, "rebound.", waitMs)
        }
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

    @Test fun content() {
        val args = InstrumentationRegistry.getArguments()
        assumeTrue("only host/internal/emutest runs it", args.getString(ARG) == "content")
        onlyOnAnEmulator()
        val nm = context.getSystemService(NotificationManager::class.java)
        assertTrue("notifications are off for the app: the host grants POST_NOTIFICATIONS first", nm.areNotificationsEnabled())
        // After the force-stop of am instrument the system binds it again 10 s later (the binding died).
        assertTrue("the listener is not bound 30 s after the start: the host allows it first", awaitConnected(true, 30_000L))
        // Low importance: no heads-up nor sound for what this test posts.
        nm.createNotificationChannel(NotificationChannel(CHANNEL, "Listener test", NotificationManager.IMPORTANCE_LOW))
        val guarded = ProtectedApps.of(context)::isProtected
        try {
            val before = System.currentTimeMillis()
            post(nm, BIG_ID, builder().setContentTitle(BIG_TITLE).setContentText(SHORT_TEXT).setStyle(Notification.BigTextStyle().bigText(BIG_TEXT)).build())
            post(nm, LONG_ID, builder().setContentTitle(BIG_TITLE).setContentText(SHORT_TEXT).setStyle(Notification.BigTextStyle().bigText(LONG_TEXT)).build())
            // A group chat: MessagingStyle fills EXTRA_TEXT with the last message itself (Notification.MessagingStyle,
            // as T1-2 saw with `cmd notification post -S messaging`); an app that leaves it out is the case.
            val ana = Person.Builder().setName("Ana").build()
            val chat = builder().setStyle(
                Notification.MessagingStyle(Person.Builder().setName("Me").build())
                    .setConversationTitle(CHAT_TITLE)
                    .setGroupConversation(true)
                    .addMessage(CHAT_MESSAGE, before, ana),
            ).build()
            chat.extras.remove(Notification.EXTRA_TEXT)
            post(nm, CHAT_ID, chat)
            post(nm, SECRET_ID, builder().setContentTitle(SECRET_TITLE).setContentText(SECRET_TEXT).setVisibility(Notification.VISIBILITY_SECRET).build())
            post(nm, ONGOING_ID, builder().setContentTitle(ONGOING_TITLE).setContentText("Still going").setOngoing(true).build())
            post(nm, PLAIN_ID, builder().setContentTitle(PLAIN_TITLE).setContentText(PLAIN_TEXT).build())
            // The 🔴 question as the app posts it (ConfirmNotifier, the real builder), here without the PC connected.
            ConfirmNotifier.sync(context, BridgeState(pending = listOf(QUESTION)))
            val after = System.currentTimeMillis()

            val raws = awaitRead("the six notifications and the question") { r -> MINE.all { id -> mine(r, id) != null } && question(r) != null }
            val chatPosted = nm.activeNotifications.single { it.id == CHAT_ID }
            assertEquals("the chat notification still has EXTRA_TEXT once posted: the case is not tested", null,
                chatPosted.notification.extras.getCharSequence(Notification.EXTRA_TEXT))

            // BigText: the whole big text, never the short line; cut to 1024 code points in the report.
            val big = mine(raws, BIG_ID)!!
            assertFalse("a plain notification of the app's own channel reads as secret: everything would come out empty", big.secret)
            assertEquals("BigText: the listener did not read the whole big text", BIG_TEXT, big.text)
            val bigOut = asOther(big, guarded)
            assertEquals(BIG_TITLE, bigOut.title)
            assertEquals("BigText in the report: not the whole big text", BIG_TEXT, bigOut.text)
            // Longer than 1024: Android's Notification keeps its first 1024 characters (measured on API 34), the
            // same limit as the report's: the listener reads those 1024 and the report keeps them all.
            val long = mine(raws, LONG_ID)!!
            assertEquals("a big text of ${LONG_TEXT.length}: not its first ${UiLimits.MAX_NOTIFICATION_TEXT} characters", LONG_TEXT.substring(0, UiLimits.MAX_NOTIFICATION_TEXT), long.text)
            assertEquals(LONG_TEXT.substring(0, UiLimits.MAX_NOTIFICATION_TEXT), asOther(long, guarded).text)

            // MessagingStyle without EXTRA_TEXT: the text comes out empty (the messages are not read), never the message.
            val chatRead = mine(raws, CHAT_ID)!!
            assertEquals("MessagingStyle without EXTRA_TEXT: text", "", chatRead.text)
            assertFalse("the message's text reached the listener's title", chatRead.title.contains(CHAT_MESSAGE))
            assertEquals("", asOther(chatRead, guarded).text)

            // VISIBILITY_SECRET: secret in the read, and empty in the report whatever the app.
            val secret = mine(raws, SECRET_ID)!!
            assertTrue("a VISIBILITY_SECRET notification is not read as secret", secret.secret)
            val secretOut = asOther(secret, guarded)
            assertEquals("the SECRET notification's title left the phone", "", secretOut.title)
            assertEquals("the SECRET notification's text left the phone", "", secretOut.text)
            assertTrue("the SECRET notification's key (with its tag) left the phone: ${secretOut.key.length} chars", secretOut.key.startsWith("hidden-"))

            // ongoing and postedAt: the system's post time, in UTC, to the second.
            val ongoing = mine(raws, ONGOING_ID)!!
            assertTrue("an ongoing notification read as not ongoing", ongoing.ongoing)
            assertFalse("a plain notification read as ongoing", mine(raws, PLAIN_ID)!!.ongoing)
            assertTrue("ongoing in the report", asOther(ongoing, guarded).ongoing)
            for (id in MINE) {
                val n = mine(raws, id)!!
                assertTrue("notification $id: postedAt ${n.postedAtMs} is not between $before and $after (+1 s)", n.postedAtMs in before..after + 1000)
                val at = asOther(n, guarded).postedAt
                assertTrue("notification $id: postedAt $at is not UTC", at.endsWith("Z"))
                assertEquals("notification $id: postedAt $at is not its post time to the second", n.postedAtMs / 1000, Instant.parse(at).epochSecond)
            }
            val plain = asOther(mine(raws, PLAIN_ID)!!, guarded)
            assertEquals(PLAIN_TITLE, plain.title)
            assertEquals(PLAIN_TEXT, plain.text)

            // The 🔴 question has words to hide (the read has them), and the real report leaves them out, like
            // everything of ADBuddy's.
            assertTrue("the question's read has no title: nothing to hide", question(raws)!!.title.contains(QUESTION.title))
            assertTrue("these reads already pass ${UiLimits.MAX_NOTIFICATIONS}: the rest of the test needs fewer", raws.size <= UiLimits.MAX_NOTIFICATIONS)
            val report = NotificationReport.build(raws, guarded)
            assertFalse("${raws.size} notifications came out truncated", report.truncated)
            val adbuddy = report.notifications.filter { it.`package` == Packages.ADBUDDY }
            assertEquals("ADBuddy's notifications in the report", raws.count { it.pkg == Packages.ADBUDDY }, adbuddy.size)
            for (n in adbuddy) {
                assertTrue("an ADBuddy notification came out with its title, text or key", n.title.isEmpty() && n.text.isEmpty() && n.key.startsWith("hidden-"))
            }
            // The host's fillers (`cmd notification post`, com.android.shell): another app's come out with their words.
            val shell = report.notifications.filter { it.`package` == "com.android.shell" && it.title == FILLER_TITLE }
            assertTrue("no filler of the host's (cmd notification post -t '$FILLER_TITLE') in the report with its text", shell.isNotEmpty() && shell.all { it.text.startsWith("filler ") })

            // More than 50 at once: the newest 50 and truncated. A package may have 50 at most (NotificationManagerService):
            // the host's fillers make up the rest, so this app posts 49 at most.
            var next = FLOOD_ID
            var all = raws
            while (all.size <= UiLimits.MAX_NOTIFICATIONS) {
                assertTrue("50 notifications are not reached with 49 of this app's own (${all.size} now)", all.count { it.pkg == Packages.ADBUDDY } < 49)
                post(nm, next++, builder().setContentTitle("Filler of the listener test").setContentText("filler").build())
                // Under the system's rate of 5 a second per app, or it drops them.
                SystemClock.sleep(300)
                all = AgentNotificationListener.read().orEmpty()
            }
            all = awaitRead("more than ${UiLimits.MAX_NOTIFICATIONS}") { it.size > UiLimits.MAX_NOTIFICATIONS }
            val full = NotificationReport.build(all, guarded)
            assertTrue("${all.size} notifications did not come out truncated", full.truncated)
            assertEquals("${all.size} notifications: the report's size", UiLimits.MAX_NOTIFICATIONS, full.notifications.size)
            assertEquals("the report is not the newest first", full.notifications.sortedByDescending { it.postedAt }, full.notifications)
        } finally {
            nm.cancelAll()
            ConfirmNotifier.clear(context)
            nm.deleteNotificationChannel(CHANNEL)
        }
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
