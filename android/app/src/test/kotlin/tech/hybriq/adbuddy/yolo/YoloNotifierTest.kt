package tech.hybriq.adbuddy.yolo

import android.app.Notification
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import java.time.format.FormatStyle
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.YoloState
import tech.hybriq.adbuddy.confirm.HostClock
import tech.hybriq.adbuddy.ui.Formats

@RunWith(RobolectricTestRunner::class)
class YoloNotifierTest {
    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private val nm get() = app.getSystemService(NotificationManager::class.java)
    private fun posted(): Notification? = shadowOf(nm).getNotification(null, YoloNotifier.NOTIFICATION_ID)
    private fun title(n: Notification) = n.extras.getCharSequence(Notification.EXTRA_TITLE).toString()
    private fun body(n: Notification) = n.extras.getCharSequence(Notification.EXTRA_TEXT).toString()

    private val session = BridgeState(yolo = YoloState(active = true, session = true))

    @Test fun theKeyIsNullUnlessTheModeIsOn() {
        assertNull(YoloUi.notificationKey(BridgeState()))
        assertNull(YoloUi.notificationKey(BridgeState(yolo = YoloState())))
        assertEquals("session", YoloUi.notificationKey(session))
        assertEquals("until:5000", YoloUi.notificationKey(BridgeState(yolo = YoloState(active = true, until = 5000))))
    }

    // Mutation: sync posts only while connected.
    @Test fun itIsUpWhileTheModeIsOnEvenWithTheLinkDownAndGoneWhenItIsOff() {
        YoloNotifier.clear(app)
        YoloNotifier.sync(app, session.copy(connection = BridgeState.Connection.WAITING), app.clock)
        val n = posted()
        assertNotNull(n)
        assertEquals(app.getString(R.string.yolo_notif_title), title(n!!))
        assertEquals(app.getString(R.string.yolo_notif_session), body(n))
        assertTrue("it cannot be swiped away", n.flags and Notification.FLAG_ONGOING_EVENT != 0)
        assertEquals(Notification.VISIBILITY_PUBLIC, n.visibility)
        YoloNotifier.sync(app, BridgeState(yolo = YoloState()), app.clock)
        assertNull(posted())
    }

    // Turning it off needs no unlock and no question: the action is a plain broadcast to the receiver.
    // Mutation: Intent(ctx, YoloReceiver::class.java) becomes Intent(); FLAG_IMMUTABLE becomes FLAG_MUTABLE; .setLocalOnly(true) removed.
    @Test fun theOffButtonIsOneActionToTheReceiver() {
        YoloNotifier.clear(app)
        YoloNotifier.sync(app, session, app.clock)
        val n = posted()!!
        assertEquals(AdbuddyApp.CHANNEL_YOLO, n.channelId)
        assertTrue("it stays on this phone", n.flags and Notification.FLAG_LOCAL_ONLY != 0)
        val action = n.actions.single()
        assertEquals(app.getString(R.string.yolo_notif_off), action.title.toString())
        val shadow = shadowOf(action.actionIntent)
        assertTrue(shadow.isBroadcast)
        assertEquals(YoloReceiver.ACTION_OFF, shadow.savedIntent.action)
        assertEquals(YoloReceiver::class.java.name, shadow.savedIntent.component?.className)
        assertTrue("nobody can rewrite the intent", shadow.isImmutable && shadow.flags and PendingIntent.FLAG_IMMUTABLE != 0)
        assertEquals(false, action.isAuthenticationRequired)
    }

    // Mutation: the condition `shown == null && state.yolo?.active == false` is removed from offFailed.
    @Test fun aLateFailureAfterTheModeWentOffPostsNothing() {
        YoloNotifier.clear(app)
        YoloNotifier.sync(app, session, app.clock)
        val off = BridgeState(yolo = YoloState())
        YoloNotifier.sync(app, off, app.clock)
        YoloNotifier.offFailed(app, off, app.clock)
        assertNull(posted())
    }

    // Mutation: sync returns when both keys are non-null (`if (key != null && shown != null) return`).
    @Test fun renewingTheModeRepaintsTheText() {
        val wall = 1_700_000_000_000L
        val clock = HostClock(wall = { wall }, elapsed = { 0 })
        clock.observe(Protocol.decode("""{"v":1,"seq":1,"type":"welcome","ts":"${java.time.Instant.ofEpochMilli(wall)}","payload":{}}""")!!)
        fun until(ms: Long) = BridgeState(yolo = YoloState(active = true, until = wall + ms))
        YoloNotifier.clear(app)
        YoloNotifier.sync(app, until(15 * 60_000L), clock)
        val first = body(posted()!!)
        YoloNotifier.sync(app, until(60 * 60_000L), clock)
        assertEquals(app.getString(R.string.yolo_notif_until, Formats.time(wall + 60 * 60_000L, FormatStyle.SHORT)), body(posted()!!))
        assertTrue(first != body(posted()!!))
    }

    // Mutation: clear cancels only when shown != null.
    @Test fun clearInAColdProcessCancelsWhatIsUp() {
        YoloNotifier.clear(app) // shown == null
        nm.notify(YoloNotifier.NOTIFICATION_ID, Notification.Builder(app, AdbuddyApp.CHANNEL_YOLO).setSmallIcon(R.drawable.ic_stat_adbuddy).build())
        assertNotNull(posted())
        YoloNotifier.clear(app)
        assertNull(posted())
    }

    @Test fun anOffThatCannotBeSentSaysSoAndNamesThePcCommand() {
        YoloNotifier.clear(app)
        app.bridge.setStateForTest(session) // no socket: the request cannot go
        YoloNotifier.sync(app, session, app.clock)
        YoloReceiver().onReceive(app, Intent(YoloReceiver.ACTION_OFF))
        assertEquals(app.getString(R.string.yolo_off_failed), body(posted()!!))
    }

    @Test fun otherActionsAreIgnored() {
        YoloNotifier.clear(app)
        YoloNotifier.sync(app, session, app.clock)
        YoloReceiver().onReceive(app, Intent("tech.hybriq.adbuddy.action.SOMETHING"))
        assertEquals(app.getString(R.string.yolo_notif_session), body(posted()!!))
    }

    @Test fun clearRemovesWhatAnEarlierProcessLeft() {
        YoloNotifier.sync(app, session, app.clock)
        YoloNotifier.clear(app)
        assertNull(posted())
    }

    // Mutation: offFailed returns when nothing is shown (a cold process, the old notification still up).
    @Test fun aColdProcessStillSaysTheOffFailed() {
        YoloNotifier.clear(app)
        YoloNotifier.offFailed(app, BridgeState(), app.clock)
        assertEquals(app.getString(R.string.yolo_off_failed), body(posted()!!))
        // and the next change of the mode replaces it (here: off takes it down)
        YoloNotifier.sync(app, BridgeState(yolo = YoloState()), app.clock)
        assertNull(posted())
    }

    private fun reply(type: String) = Protocol.decode("""{"v":1,"seq":1,"type":"$type","ts":"2026-10-19T10:00:00Z","payload":{}}""")!!

    private fun offWith(answer: (((Message?) -> Unit)) -> String?) {
        YoloNotifier.clear(app)
        YoloNotifier.sync(app, session, app.clock)
        YoloReceiver.turnOff(app) { _, cb -> answer(cb) }
    }

    // Mutation: the callback ignores the reply.
    @Test fun aRefusedOrUnansweredOffSaysSoButAnAcceptedOneDoesNot() {
        offWith { cb -> cb(null); "r1" }
        assertEquals(app.getString(R.string.yolo_off_failed), body(posted()!!))
        offWith { cb -> cb(reply(Protocol.ERROR)); "r1" }
        assertEquals(app.getString(R.string.yolo_off_failed), body(posted()!!))
        offWith { cb -> cb(reply(Protocol.YOLO_STATE)); "r1" }
        assertEquals(app.getString(R.string.yolo_notif_session), body(posted()!!))
        offWith { _ -> null }
        assertEquals(app.getString(R.string.yolo_off_failed), body(posted()!!))
    }

    // Mutation: the text uses the expiry as is, without the host-to-phone clock.
    @Test fun aTimedModeSaysUntilWhenInThePhonesClock() {
        val wall = 1_700_000_000_000L
        val clock = HostClock(wall = { wall }, elapsed = { 0 })
        val hostNow = wall + 2 * 3_600_000L // the host runs two hours ahead
        clock.observe(Protocol.decode("""{"v":1,"seq":1,"type":"welcome","ts":"${java.time.Instant.ofEpochMilli(hostNow)}","payload":{}}""")!!)
        val until = hostNow + 30 * 60_000L
        YoloNotifier.clear(app)
        YoloNotifier.sync(app, BridgeState(yolo = YoloState(active = true, until = until)), clock)
        assertEquals(app.getString(R.string.yolo_notif_until, Formats.time(wall + 30 * 60_000L, FormatStyle.SHORT)), body(posted()!!))
    }

    // Mutation: sync does not compare with the key it showed.
    @Test fun theSameKeyDoesNotPostAgain() {
        YoloNotifier.clear(app)
        YoloNotifier.sync(app, session, app.clock)
        nm.cancel(YoloNotifier.NOTIFICATION_ID)
        YoloNotifier.sync(app, session, app.clock)
        assertNull(posted())
    }
}
