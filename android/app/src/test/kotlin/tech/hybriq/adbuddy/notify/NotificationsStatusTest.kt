package tech.hybriq.adbuddy.notify

import android.app.NotificationChannel
import android.app.NotificationManager
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import tech.hybriq.adbuddy.AdbuddyApp

/** Whether the user can be told something is waiting: notifications on and the confirm and ask channels not silenced. */
@RunWith(RobolectricTestRunner::class)
class NotificationsStatusTest {
    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private val nm get() = app.getSystemService(NotificationManager::class.java)

    private fun silence(id: String) {
        // Lowering the importance of an existing channel is allowed; delete and recreate would restore the old settings.
        nm.createNotificationChannel(NotificationChannel(id, "x", NotificationManager.IMPORTANCE_NONE))
        assertEquals(NotificationManager.IMPORTANCE_NONE, nm.getNotificationChannel(id).importance)
    }

    @Test fun withNotificationsOnAndNormalChannelsNothingIsBlocked() {
        assertNotNull(nm.getNotificationChannel(AdbuddyApp.CHANNEL_CONFIRM))
        assertNotNull(nm.getNotificationChannel(AdbuddyApp.CHANNEL_ASK))
        val s = NotificationsStatus.of(app)
        assertTrue(s.appEnabled)
        assertFalse(s.channelBlocked)
        assertFalse(s.blocked)
    }

    @Test fun withNotificationsOffTheAppIsBlocked() {
        shadowOf(nm).setNotificationsEnabled(false)
        val s = NotificationsStatus.of(app)
        assertFalse(s.appEnabled)
        assertTrue(s.blocked)
    }

    @Test fun aSilencedConfirmChannelIsBlocked() {
        silence(AdbuddyApp.CHANNEL_CONFIRM)
        val s = NotificationsStatus.of(app)
        assertTrue(s.appEnabled)
        assertTrue(s.channelBlocked)
        assertTrue(s.blocked)
    }

    @Test fun aSilencedAskChannelIsBlocked() {
        silence(AdbuddyApp.CHANNEL_ASK)
        val s = NotificationsStatus.of(app)
        assertTrue(s.channelBlocked)
        assertTrue(s.blocked)
    }

    @Test fun aMissingChannelIsNotBlocked() {
        nm.deleteNotificationChannel(AdbuddyApp.CHANNEL_CONFIRM)
        nm.deleteNotificationChannel(AdbuddyApp.CHANNEL_ASK)
        val s = NotificationsStatus.of(app)
        assertFalse(s.channelBlocked)
        assertFalse(s.blocked)
    }
}
