package tech.hybriq.adbuddy.yolo

import android.app.Notification
import android.app.NotificationManager
import android.content.Intent
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
import tech.hybriq.adbuddy.bridge.YoloState

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
    @Test fun theOffButtonIsOneActionToTheReceiver() {
        YoloNotifier.clear(app)
        YoloNotifier.sync(app, session, app.clock)
        val action = posted()!!.actions.single()
        assertEquals(app.getString(R.string.yolo_notif_off), action.title.toString())
        val shadow = shadowOf(action.actionIntent)
        assertTrue(shadow.isBroadcastIntent)
        assertEquals(YoloReceiver.ACTION_OFF, shadow.savedIntent.action)
        assertEquals(false, action.isAuthenticationRequired)
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
}
