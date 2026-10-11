package tech.hybriq.adbuddy.confirm

import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Decision

/** M2-8: ConfirmReceiver.handle with a fake clock, lock, scheduler and a real mayReply. */
class ConfirmReceiverHandleTest {
    private var clock = 1_000_000L
    private val arming = ConfirmArming { clock }
    private val hostNow = Instant.parse("2026-09-30T10:00:00Z").toEpochMilli()
    private var state = BridgeState(
        connection = BridgeState.Connection.CONNECTED,
        pending = listOf(ConfirmRequest(id = "j_1", title = "Restart", rawCommand = "adb shell reboot", expiresAt = "2026-09-30T10:01:00Z")),
    )

    private var locked = true
    private var sends = 0
    private var finishes = 0
    private var mayCalls = mutableListOf<Boolean>()
    private val queue = ArrayDeque<Pair<Long, () -> Unit>>()
    private var scheduled = 0
    private var failSchedule = false
    private var failSend = false
    private var failMay = false

    /** Readings of the lock, consumed one per call before falling back to [locked]. */
    private val script = ArrayDeque<Boolean>()
    private fun isLocked(): Boolean = script.removeFirstOrNull() ?: locked

    private fun schedule(d: Long, s: () -> Unit) {
        scheduled++
        if (failSchedule) throw IllegalStateException("dead handler")
        queue.addLast(d to s)
    }

    private fun run() {
        while (queue.isNotEmpty()) {
            val (d, s) = queue.removeFirst()
            clock += d
            s()
        }
    }

    private fun handle(decision: String, sdk: Int = 34, lockedAtDelivery: Boolean = locked, armedAtDelivery: Boolean = true) =
        ConfirmReceiver().handle(
        decision, lockedAtDelivery, armedAtDelivery, sdk, ::isLocked, { clock }, ::schedule,
        mayReplyNow = { l ->
            mayCalls.add(l)
            if (failMay) throw IllegalStateException("boom")
            ConfirmReceiver.mayReply(state, "j_1", decision, hostNow, l, arming)
        },
        send = {
            if (failSend) throw IllegalStateException("boom")
            sends++
            state = state.copy(answering = state.answering + "j_1")
        },
        finish = { finishes++ },
    )

    private fun armed() { arming.shown("j_1"); clock += 600 }

    @Test fun lockedThenUnlockedAt300SendsOnceWithTheFreshRead() {
        armed()
        val start = clock
        handle(Decision.ALLOW)
        assertEquals(0, sends)
        // The system unlocks 300 ms later.
        while (queue.isNotEmpty()) {
            val (d, s) = queue.removeFirst(); clock += d
            if (clock - start >= 300) locked = false
            s()
        }
        assertEquals(1, sends)
        assertEquals(listOf(false), mayCalls)
        assertEquals(1, finishes)
    }

    @Test fun neverUnlockedSendsNothing() {
        armed()
        handle(Decision.ALLOW); run()
        assertEquals(0, sends)
        assertTrue(mayCalls.none { !it })
        assertEquals(1, finishes)
    }

    @Test fun unlockedButNoLongerArmedAfterRearmSendsNothing() {
        armed()
        handle(Decision.ALLOW)
        queue.removeFirst().second.let { clock += 50; it() }
        locked = false
        arming.rearmAll() // the screen turned on during the wait
        run()
        assertEquals(0, sends)
        assertEquals(1, finishes)
    }

    @Test fun denyWhileLockedSendsAtOnceWithoutWaiting() {
        armed()
        handle(Decision.DENY)
        assertEquals(1, sends)
        assertEquals(0, scheduled)
        assertEquals(1, finishes)
    }

    @Test fun sdk30LockedNeitherSendsNorWaits() {
        armed()
        handle(Decision.ALLOW, sdk = 30)
        assertEquals(0, sends)
        assertEquals(0, scheduled)
        assertEquals(1, finishes)
    }

    @Test fun notLockedAtDeliverySendsSynchronously() {
        locked = false
        armed()
        handle(Decision.ALLOW)
        assertEquals(1, sends)
        assertEquals(0, scheduled)
        assertEquals(1, finishes)
    }

    @Test fun finishesExactlyOnceWhenSendThrows() {
        locked = false; armed(); failSend = true
        try { handle(Decision.ALLOW); fail("must rethrow") } catch (e: IllegalStateException) { }
        assertEquals(1, finishes)
    }

    @Test fun finishesExactlyOnceWhenMayReplyThrows() {
        locked = false; armed(); failMay = true
        try { handle(Decision.DENY); fail("must rethrow") } catch (e: IllegalStateException) { }
        assertEquals(1, finishes)
    }

    @Test fun finishesExactlyOnceWhenScheduleThrows() {
        armed(); failSchedule = true
        handle(Decision.ALLOW) // UnlockWait turns it into false
        assertEquals(0, sends)
        assertEquals(1, finishes)
    }

    @Test fun aSecondDeliveryOfTheSameQuestionSendsNothing() {
        armed()
        handle(Decision.ALLOW)
        handle(Decision.ALLOW)
        locked = false
        run()
        assertEquals("the second one sees the question answering", 1, sends)
        assertEquals(2, finishes)
    }

    // The wait said "unlocked" but the fresh read says locked again: nothing is sent.
    @Test fun aRelockedReadingAfterTheWaitSendsNothing() {
        armed()
        script.addAll(listOf(true, false, true)) // first poll locked, second unlocked, the fresh read locked
        handle(Decision.ALLOW); run()
        assertEquals(0, sends)
        assertEquals(listOf(true), mayCalls)
        assertEquals(1, finishes)
    }

    // The tap that came with the notification not yet armed (a stray tap as it popped up): a locked Allow is dropped at once.
    @Test fun anAllowLockedAndNotArmedAtDeliveryIsDroppedWithoutWaiting() {
        armed()
        handle(Decision.ALLOW, armedAtDelivery = false)
        assertEquals(0, sends)
        assertEquals(0, scheduled)
        assertEquals(0, mayCalls.size)
        assertEquals(1, finishes)
    }

    @Test fun aDenyNotArmedAtDeliveryStillSends() {
        armed()
        handle(Decision.DENY, armedAtDelivery = false)
        assertEquals(1, sends)
        assertEquals(1, finishes)
    }
}
