package tech.hybriq.adbuddy.bridge

import android.content.Intent
import android.os.Looper
import java.time.Duration
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import tech.hybriq.adbuddy.confirm.ConfirmNotifier

/**
 * The screen turning on rearms the notification's Allow (ConfirmArming.rearmAll): a question that arrived with the
 * screen off is not allowed by the first tap on the lock screen or the shade. BridgeService listens for
 * ACTION_SCREEN_ON while it lives, and no longer once destroyed.
 */
@RunWith(RobolectricTestRunner::class)
class BridgeServiceScreenOnTest {
    private val app get() = RuntimeEnvironment.getApplication()
    private val arming get() = ConfirmNotifier.arming

    @After fun clean() = ConfirmNotifier.clear(app)

    private fun advance(ms: Long) = shadowOf(Looper.getMainLooper()).idleFor(Duration.ofMillis(ms))

    private fun screenOn() {
        app.sendBroadcast(Intent(Intent.ACTION_SCREEN_ON))
        shadowOf(Looper.getMainLooper()).idle()
    }

    @Test fun theScreenTurningOnRearmsTheNotificationsAllow() {
        val service = Robolectric.buildService(BridgeService::class.java).create()
        // After onCreate: it clears the questions of an earlier process.
        arming.shown("j_dark")
        advance(5_000)
        assertTrue(arming.isArmed("j_dark"))
        screenOn()
        assertFalse("the first tap after the screen turns on is not an answer", arming.isArmed("j_dark"))
        advance(600)
        assertTrue(arming.isArmed("j_dark"))

        service.destroy()
        arming.shown("j_after")
        advance(600)
        screenOn()
        assertTrue("a destroyed service no longer listens", arming.isArmed("j_after"))
    }
}
