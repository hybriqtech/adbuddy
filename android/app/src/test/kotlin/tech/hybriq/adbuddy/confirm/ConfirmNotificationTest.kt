package tech.hybriq.adbuddy.confirm

import android.app.Notification
import android.app.NotificationManager
import android.os.Looper
import java.time.Duration
import java.time.Instant
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Decision

/**
 * The 🔴 notification as ConfirmNotifier really builds it (Robolectric's NotificationManager): its buttons (A4-1: a
 * special question or a command the notification cuts gets only Deny and Open, and no notification offers Allow during
 * the session), Allow asking to unlock (setAuthenticationRequired, M2-5) and Deny not, the ⚠ first, and the question kept
 * on this phone (setLocalOnly, M2-9). The emulator test (host/internal/emutest/confirm_test.go) sees the same on a real
 * system UI.
 */
@RunWith(RobolectricTestRunner::class)
class ConfirmNotificationTest {
    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private val nm get() = app.getSystemService(NotificationManager::class.java)

    @Before fun clean() = ConfirmNotifier.clear(app)

    @After fun cleanAfter() = ConfirmNotifier.clear(app)

    private fun request(id: String, raw: String = "adb shell reboot", special: Boolean = false, allowSession: Boolean = false) =
        ConfirmRequest(
            id = id, title = "Restart the phone", detail = "may restart", rawCommand = raw,
            expiresAt = Instant.ofEpochMilli(app.clock.now()).plusSeconds(600).toString(), special = special, allowSession = allowSession,
        )

    /** Posts r as the only pending question and returns its notification. */
    private fun post(r: ConfirmRequest): Notification {
        ConfirmNotifier.sync(app, BridgeState(connection = BridgeState.Connection.CONNECTED, pending = listOf(r)))
        val n = nm.activeNotifications.single { it.tag == r.id && it.id == ConfirmNotifier.NOTIFICATION_ID }
        return n.notification
    }

    private fun labels(n: Notification): List<String> = n.actions.orEmpty().map { it.title.toString() }

    private fun action(n: Notification, label: Int): Notification.Action = n.actions.single { it.title.toString() == app.getString(label) }

    private val allow get() = app.getString(R.string.action_allow)
    private val deny get() = app.getString(R.string.action_deny)
    private val session get() = app.getString(R.string.action_allow_session)
    private val open get() = app.getString(R.string.action_open)

    @Test fun aPlainQuestionOffersDenyAndAllow() {
        val n = post(request("j_plain"))
        assertEquals(listOf(deny, allow), labels(n))
        assertTrue("Allow asks to unlock", action(n, R.string.action_allow).isAuthenticationRequired)
        assertFalse("Deny does not", action(n, R.string.action_deny).isAuthenticationRequired)
        assertTrue("the question stays on this phone (FLAG_LOCAL_ONLY)", n.flags and Notification.FLAG_LOCAL_ONLY != 0)
    }

    // Allow during the session is the dialog's only: the notification offers Open to reach it (M2).
    @Test fun allowDuringTheSessionIsNeverInTheNotification() {
        val n = post(request("j_session", allowSession = true))
        assertEquals(listOf(deny, allow, open), labels(n))
        assertTrue(action(n, R.string.action_allow).isAuthenticationRequired)
    }

    @Test fun aSpecialQuestionOffersOnlyDenyAndOpenWithItsWarningFirst() {
        val n = post(request("j_special", special = true, allowSession = true))
        assertEquals(listOf(deny, open), labels(n))
        assertFalse(session in labels(n))
        val text = n.extras.getCharSequence(Notification.EXTRA_BIG_TEXT).toString()
        assertTrue("the ⚠ first: $text", text.startsWith("⚠ " + app.getString(R.string.confirm_special)))
        assertTrue(text.contains(app.getString(R.string.confirm_open_special)))
    }

    @Test fun aCommandTheNotificationCutsOffersOnlyDenyAndOpen() {
        val n = post(request("j_cut", raw = "adb shell echo " + "c".repeat(ConfirmUi.MAX_COMMAND), allowSession = true))
        assertEquals(listOf(deny, open), labels(n))
        assertTrue(n.extras.getCharSequence(Notification.EXTRA_BIG_TEXT).toString().contains(app.getString(R.string.confirm_open_cut)))
    }

    @Test fun aCommandLongerThanTheDialogOffersOnlyDenyAndOpen() {
        val n = post(request("j_long", raw = "adb shell echo " + "d".repeat(ConfirmUi.DIALOG_MAX_COMMAND)))
        assertEquals(listOf(deny, open), labels(n))
        assertTrue(n.extras.getCharSequence(Notification.EXTRA_BIG_TEXT).toString().contains(app.getString(R.string.confirm_too_long)))
    }

    // --- the arming of the notification's Allow (ConfirmNotifier.arming), in the phone's elapsed time ---

    private fun advance(ms: Long) = shadowOf(Looper.getMainLooper()).idleFor(Duration.ofMillis(ms))

    private fun connected(vararg pending: ConfirmRequest, answering: Set<String> = emptySet()) =
        BridgeState(connection = BridgeState.Connection.CONNECTED, pending = pending.toList(), answering = answering)

    private fun receiverMay(state: BridgeState, id: String, decision: String) =
        ConfirmReceiver.mayReply(state, id, decision, app.clock.now(), locked = false, arming = ConfirmNotifier.arming)

    @Test fun theNotificationsAllowIsArmedSixHundredMillisecondsAfterItIsPosted() {
        val r = request("j_arm")
        val s = connected(r)
        ConfirmNotifier.sync(app, s)
        assertFalse("a tap at once is dropped", receiverMay(s, r.id, Decision.ALLOW))
        assertTrue("Deny never waits", receiverMay(s, r.id, Decision.DENY))
        advance(300)
        assertFalse(receiverMay(s, r.id, Decision.ALLOW))
        // A change of language posts it again: the wait does not start over.
        ConfirmNotifier.repaint(app, s)
        advance(300)
        assertTrue(receiverMay(s, r.id, Decision.ALLOW))
    }

    // Its button gone (answered here: SENDING) and back (the connection dropped and came back): a new wait.
    @Test fun allowWaitsAgainWhenItsButtonComesBack() {
        val r = request("j_back")
        ConfirmNotifier.sync(app, connected(r))
        advance(1_000)
        ConfirmNotifier.sync(app, connected(r, answering = setOf(r.id)))
        assertFalse(r.id in ConfirmNotifier.arming.tracked())
        val again = connected(r)
        ConfirmNotifier.sync(app, again)
        assertFalse(receiverMay(again, r.id, Decision.ALLOW))
        advance(600)
        assertTrue(receiverMay(again, r.id, Decision.ALLOW))
    }

    @Test fun onlyAPostedAllowIsArmedAndTheGoneAreForgotten() {
        val plain = request("j_plain2")
        val special = request("j_special2", special = true)
        ConfirmNotifier.sync(app, connected(plain, special))
        assertEquals("a notification with only Deny arms nothing", setOf(plain.id), ConfirmNotifier.arming.tracked())
        ConfirmNotifier.sync(app, connected(special))
        assertEquals("the answered question is forgotten", emptySet<String>(), ConfirmNotifier.arming.tracked())
        ConfirmNotifier.sync(app, connected(plain))
        ConfirmNotifier.clear(app)
        assertEquals(emptySet<String>(), ConfirmNotifier.arming.tracked())
    }
}
