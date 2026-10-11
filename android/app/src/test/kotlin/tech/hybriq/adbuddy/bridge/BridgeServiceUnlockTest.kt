package tech.hybriq.adbuddy.bridge

import android.app.KeyguardManager
import android.app.NotificationManager
import android.content.Intent
import android.os.Bundle
import android.os.Looper
import androidx.core.app.RemoteInput
import java.time.Duration
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNotSame
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.Shadows.shadowOf
import org.robolectric.shadows.ShadowToast
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.confirm.ConfirmNotifier
import tech.hybriq.adbuddy.confirm.ScriptedKeyguard
import tech.hybriq.adbuddy.notify.ConversationNotifier

/**
 * M2-8: the Reply and Capture buttons delivered with the phone locked wait for the PIN (UnlockWait) instead of being
 * dropped; destroyed meanwhile, nothing happens. API 34 only (the SDK is a parameter of UnlockWait, tested there).
 */
@RunWith(RobolectricTestRunner::class)
class BridgeServiceUnlockTest {
    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private val nm get() = shadowOf(app.getSystemService(NotificationManager::class.java))

    @After fun clean() { ConfirmNotifier.clear(app); ScriptedKeyguard.script.clear(); ScriptedKeyguard.alwaysThrow = false }

    private fun lock(v: Boolean) = shadowOf(app.getSystemService(KeyguardManager::class.java)).setIsDeviceLocked(v)

    private fun advance(ms: Long) = shadowOf(Looper.getMainLooper()).idleFor(Duration.ofMillis(ms))

    private fun reply(text: String): Intent {
        val i = Intent(BridgeService.ACTION_REPLY)
        val results = Bundle().apply { putCharSequence(ConversationNotifier.KEY_REPLY, text) }
        RemoteInput.addResultsToIntent(arrayOf(RemoteInput.Builder(ConversationNotifier.KEY_REPLY).build()), i, results)
        return i
    }

    private fun posted() = nm.getNotification(BridgeService.NOTIFICATION_ID)

    private fun start() = Robolectric.buildService(BridgeService::class.java).create().also { shadowOf(Looper.getMainLooper()).idle() }

    @Test fun aReplyDeliveredLockedIsSentOnlyOnceTheAppReadsUnlocked() {
        lock(true)
        val s = start()
        val before = posted()
        s.get().onStartCommand(reply("hola"), 0, 1)
        advance(100)
        assertEquals(0, app.chat.outgoing.value.size)
        lock(false)
        advance(100)
        assertEquals(listOf("hola"), app.chat.outgoing.value.map { it.text })
        assertNotSame("the notification is posted again", before, posted())
    }

    @Test fun anEmptyReplyAfterTheUnlockSendsNothingButRepostsTheNotification() {
        lock(true)
        val s = start()
        val before = posted()
        s.get().onStartCommand(reply("  "), 0, 1)
        lock(false)
        advance(100)
        assertEquals(0, app.chat.outgoing.value.size)
        assertNotSame(before, posted())
    }

    @Test fun neverUnlockedDropsTheReplyAndRepostsTheNotification() {
        lock(true)
        val s = start()
        val before = posted()
        s.get().onStartCommand(reply("hola"), 0, 1)
        advance(3_000)
        assertEquals(0, app.chat.outgoing.value.size)
        assertNotSame(before, posted())
        lock(false)
        advance(3_000)
        assertEquals("unlocking later by another way sends nothing", 0, app.chat.outgoing.value.size)
    }

    @Test fun notLockedAtDeliveryReplyIsSynchronous() {
        lock(false)
        val s = start()
        s.get().onStartCommand(reply("hola"), 0, 1)
        assertEquals(listOf("hola"), app.chat.outgoing.value.map { it.text })
    }

    @Test fun aCaptureDeliveredLockedRunsOnlyAfterTheUnlock() {
        lock(true)
        val s = start()
        s.get().onStartCommand(Intent(BridgeService.ACTION_CAPTURE), 0, 1)
        advance(100)
        assertNull("nothing yet: no capture was asked", ShadowToast.getLatestToast())
        lock(false)
        advance(100)
        assertNotNull("the capture was asked (the host is offline: it fails with a toast)", ShadowToast.getLatestToast())
    }

    @Test fun aCaptureNeverUnlockedIsDropped() {
        lock(true)
        val s = start()
        s.get().onStartCommand(Intent(BridgeService.ACTION_CAPTURE), 0, 1)
        advance(3_000)
        lock(false)
        advance(3_000)
        assertNull(ShadowToast.getLatestToast())
    }

    @Test fun aServiceDestroyedDuringTheWaitDoesNothing() {
        lock(true)
        val s = start()
        s.get().onStartCommand(reply("hola"), 0, 1)
        s.get().onStartCommand(Intent(BridgeService.ACTION_CAPTURE), 0, 1)
        s.destroy()
        assertTrue(
            "no task of the wait is left on the main looper (removeCallbacksAndMessages)",
            shadowOf(Looper.getMainLooper()).nextScheduledTaskTime.let { it == Duration.ZERO || it > Duration.ofHours(1) },
        )
        val after = posted()
        lock(false)
        advance(3_000)
        assertEquals(0, app.chat.outgoing.value.size)
        assertNull(ShadowToast.getLatestToast())
        assertSame("no notification posted after the destroy", after, posted())
    }

    // UnlockWait says unlocked, the fresh read says locked again (a relock): nothing happens (scripted readings:
    // delivery locked, poll unlocked, fresh read locked).
    @Test @Config(shadows = [ScriptedKeyguard::class])
    fun aReplyWhoseFreshReadIsLockedAgainSendsNothingButRepostsTheNotification() {
        lock(true)
        val s = start()
        val before = posted()
        ScriptedKeyguard.script.addAll(listOf(true, false, true))
        s.get().onStartCommand(reply("hola"), 0, 1)
        advance(100)
        assertEquals(0, app.chat.outgoing.value.size)
        assertNotSame(before, posted())
    }

    @Test @Config(shadows = [ScriptedKeyguard::class])
    fun aCaptureWhoseFreshReadIsLockedAgainIsDropped() {
        lock(true)
        val s = start()
        ScriptedKeyguard.script.addAll(listOf(true, false, true))
        s.get().onStartCommand(Intent(BridgeService.ACTION_CAPTURE), 0, 1)
        advance(100)
        assertNull(ShadowToast.getLatestToast())
    }

    // A reading of the lock that throws counts as locked: it neither sends nor breaks onStartCommand.
    @Test @Config(shadows = [ScriptedKeyguard::class])
    fun aFailingLockReadingAtDeliveryCountsAsLockedForReply() {
        lock(false)
        val s = start()
        ScriptedKeyguard.alwaysThrow = true
        s.get().onStartCommand(reply("hola"), 0, 1)
        advance(3_000)
        assertEquals(0, app.chat.outgoing.value.size)
    }

    @Test @Config(shadows = [ScriptedKeyguard::class])
    fun aFailingLockReadingAtDeliveryCountsAsLockedForCapture() {
        lock(false)
        val s = start()
        ScriptedKeyguard.alwaysThrow = true
        s.get().onStartCommand(Intent(BridgeService.ACTION_CAPTURE), 0, 1)
        advance(3_000)
        assertNull(ShadowToast.getLatestToast())
    }
}
