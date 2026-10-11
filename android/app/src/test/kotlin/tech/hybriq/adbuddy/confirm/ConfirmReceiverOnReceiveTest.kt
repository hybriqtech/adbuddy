package tech.hybriq.adbuddy.confirm

import android.app.KeyguardManager
import android.content.Intent
import android.os.Handler
import android.os.Looper
import java.time.Duration
import java.util.concurrent.CopyOnWriteArrayList
import kotlinx.coroutines.flow.MutableStateFlow
import okhttp3.Request
import okhttp3.WebSocket
import okio.ByteString
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.bridge.Bridge
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Decision

/**
 * M2-8: ConfirmReceiver.onReceive end to end on Robolectric (the real AdbuddyApp and Bridge, a fake socket that records
 * what the app sends, ShadowKeyguardManager, the main looper under control).
 */
@RunWith(RobolectricTestRunner::class)
class ConfirmReceiverOnReceiveTest {
    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private val sent = CopyOnWriteArrayList<String>()

    private val socket = object : WebSocket {
        override fun request(): Request = Request.Builder().url("http://127.0.0.1/").build()
        override fun queueSize(): Long = 0
        override fun send(text: String): Boolean { sent.add(text); return true }
        override fun send(bytes: ByteString): Boolean = true
        override fun close(code: Int, reason: String?): Boolean = true
        override fun cancel() {}
    }

    @Before fun setUp() {
        ConfirmNotifier.clear(app)
        // The Bridge is not connected to anything: its state and socket are put by hand (it has no test seam).
        val state = BridgeState(
            connection = BridgeState.Connection.CONNECTED,
            pending = listOf(
                ConfirmRequest(
                    id = "j_1", title = "Restart", rawCommand = "adb shell reboot",
                    expiresAt = java.time.Instant.ofEpochMilli(app.clock.now()).plusSeconds(600).toString(),
                ),
            ),
        )
        field("_state").let { @Suppress("UNCHECKED_CAST") (it.get(app.bridge) as MutableStateFlow<BridgeState>).value = state }
        field("socket").set(app.bridge, socket)
    }

    @After fun clean() { ConfirmNotifier.clear(app); ScriptedKeyguard.alwaysThrow = false }

    private fun field(name: String) = Bridge::class.java.getDeclaredField(name).also { it.isAccessible = true }

    private fun lock(v: Boolean) = shadowOf(app.getSystemService(KeyguardManager::class.java)).setIsDeviceLocked(v)

    private fun advance(ms: Long) = shadowOf(Looper.getMainLooper()).idleFor(Duration.ofMillis(ms))

    private fun armed() { ConfirmNotifier.arming.shown("j_1"); advance(700) }

    private fun deliver(decision: String) = ConfirmReceiver().onReceive(
        app,
        Intent(ConfirmReceiver.ACTION_REPLY).putExtra(ConfirmReceiver.EXTRA_ID, "j_1").putExtra(ConfirmReceiver.EXTRA_DECISION, decision),
    )

    private fun replies() = sent.count { it.contains("confirm.reply") }

    @Test fun lockedThenUnlockedAfter300msSendsOneReply() {
        armed()
        lock(true)
        Handler(Looper.getMainLooper()).postDelayed({ lock(false) }, 300)
        deliver(Decision.ALLOW)
        assertEquals(0, replies())
        advance(1_000)
        assertEquals(1, replies())
    }

    @Test fun lockedAndNeverUnlockedSendsNone() {
        armed()
        lock(true)
        deliver(Decision.ALLOW)
        advance(2_500)
        assertEquals(0, replies())
        lock(false)
        advance(2_500)
        assertEquals("unlocking later by another way sends nothing", 0, replies())
    }

    @Test fun unlockedAtDeliverySendsSynchronously() {
        armed()
        lock(false)
        deliver(Decision.ALLOW)
        assertEquals(1, replies())
    }

    @Test fun denyWhileLockedSendsAtOnce() {
        armed()
        lock(true)
        deliver(Decision.DENY)
        assertEquals(1, replies())
    }

    private fun noTasksLeft() = shadowOf(Looper.getMainLooper()).nextScheduledTaskTime.let { it == Duration.ZERO || it > Duration.ofHours(1) }

    // The question is pending but its button was never shown (not armed): a locked Allow is dropped at once.
    @Test fun anAllowLockedWhoseQuestionIsNotArmedIsDroppedWithoutWaiting() {
        lock(true)
        deliver(Decision.ALLOW)
        assertEquals(0, replies())
        assertTrue("it did not wait: no task on the main looper", noTasksLeft())
        Handler(Looper.getMainLooper()).postDelayed({ lock(false) }, 100)
        advance(3_000)
        assertEquals(0, replies())
    }

    // A lock reading that throws at delivery counts as locked: nothing is sent and onReceive does not throw
    // (goAsync() is null here, so the single finish() is not observable; handle's test covers it).
    @Test @Config(shadows = [ScriptedKeyguard::class])
    fun aFailingLockReadingAtDeliverySendsNothingAndDoesNotThrow() {
        armed()
        ScriptedKeyguard.alwaysThrow = true
        deliver(Decision.ALLOW)
        advance(3_000)
        assertEquals(0, replies())
    }
}
