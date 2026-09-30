package tech.hybriq.adbuddy.confirm

import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Protocol

class HostClockTest {
    private val local = Instant.parse("2026-09-30T10:00:00Z").toEpochMilli()
    private fun ms(ts: String) = Instant.parse(ts).toEpochMilli()
    private fun msg(type: String, ts: String) =
        Protocol.decode("""{"v":1,"seq":1,"type":"$type","ts":"$ts","payload":{}}""")!!

    /** A phone whose wall clock and elapsed time are set by the test. */
    private class Phone(var wall: Long, var elapsed: Long = 1_000_000) {
        val clock = HostClock(wall = { wall }, elapsed = { elapsed })
        fun advance(millis: Long) {
            wall += millis
            elapsed += millis
        }
    }

    @Test fun theSkewIsHowFarAheadTheHostIs() {
        assertEquals(5_000L, HostClock.skewOf("2026-09-30T10:00:05Z", local))
        assertEquals(-120_000L, HostClock.skewOf("2026-09-30T09:58:00Z", local))
    }

    @Test fun anUnreadableTimeGivesNoSkew() {
        assertNull(HostClock.skewOf("", local))
        assertNull(HostClock.skewOf("yesterday", local))
    }

    @Test fun beforeAnyWelcomeItIsThePhonesTime() {
        val p = Phone(local)
        assertEquals(local, p.clock.now())
        assertEquals(local + 5, p.clock.toLocal(local + 5))
    }

    @Test fun theWelcomeSetsTheHostsTime() {
        val p = Phone(local)
        p.clock.observe(msg(Protocol.WELCOME, "2026-09-30T10:02:00Z"))
        assertEquals(ms("2026-09-30T10:02:00Z"), p.clock.now())
        p.advance(30_000)
        assertEquals(ms("2026-09-30T10:02:30Z"), p.clock.now())
        assertEquals("shown in the phone's time", local + 30_000, p.clock.toLocal(ms("2026-09-30T10:02:30Z")))
    }

    @Test fun theBacklogDoesNotMoveTheClock() {
        // Reconnecting, the host sends the welcome (now) and then the buffered
        // messages with the time they were first sent.
        val p = Phone(local)
        p.clock.observe(msg(Protocol.WELCOME, "2026-09-30T10:00:00Z"))
        p.clock.observe(msg(Protocol.CONFIRM_REQUEST, "2026-09-30T09:55:00Z"))
        p.clock.observe(msg(Protocol.SNAPSHOT, "2026-09-30T09:50:00Z"))
        p.clock.observe(msg(Protocol.JOURNAL_ENTRY, "2026-09-30T11:00:00Z"))
        assertEquals(local, p.clock.now())
    }

    @Test fun anOldQuestionFromTheBacklogIsExpired() {
        // Asked at 09:55 for 60 s, sent again after a reconnection at 10:00.
        val p = Phone(local)
        p.clock.observe(msg(Protocol.WELCOME, "2026-09-30T10:00:00Z"))
        p.clock.observe(msg(Protocol.CONFIRM_REQUEST, "2026-09-30T09:55:00Z"))
        val r = ConfirmRequest(id = "j_1", title = "Restart", rawCommand = "adb shell reboot", expiresAt = "2026-09-30T09:56:00Z")
        val s = BridgeState(connection = BridgeState.Connection.CONNECTED, pending = listOf(r))
        assertEquals(ConfirmUi.Status.EXPIRED, ConfirmUi.status(r, s, p.clock.now()))
    }

    @Test fun anUnreadableWelcomeTimeKeepsTheLastOne() {
        val p = Phone(local)
        p.clock.observe(msg(Protocol.WELCOME, "2026-09-30T10:01:00Z"))
        p.clock.observe(msg(Protocol.WELCOME, "garbage"))
        assertEquals(ms("2026-09-30T10:01:00Z"), p.clock.now())
    }

    @Test fun changingThePhonesTimeDoesNotMoveTheHostsClock() {
        val p = Phone(local)
        p.clock.observe(msg(Protocol.WELCOME, "2026-09-30T10:00:00Z"))
        p.wall += 3_600_000 // the person sets the clock an hour ahead
        p.elapsed += 10_000
        assertEquals(ms("2026-09-30T10:00:10Z"), p.clock.now())
    }

    @Test fun aPhoneBehindTheHostSeesTheQuestionExpireOnTime() {
        // The host welcomes at 10:00:00 its time and asks for 60 s; the phone's clock says 09:58:00.
        val p = Phone(ms("2026-09-30T09:58:00Z"))
        p.clock.observe(msg(Protocol.WELCOME, "2026-09-30T10:00:00Z"))
        val r = ConfirmRequest(id = "j_1", title = "Restart", rawCommand = "adb shell reboot", expiresAt = "2026-09-30T10:01:00Z")
        val s = BridgeState(connection = BridgeState.Connection.CONNECTED, pending = listOf(r))
        assertEquals(ConfirmUi.Status.OPEN, ConfirmUi.status(r, s, p.clock.now()))
        assertEquals("the notification times out after what is left", 60_000L, ConfirmUi.plan(emptyMap(), s, p.clock.now()).post.single().left)
        assertEquals("the expiry shown in the phone's time", ms("2026-09-30T09:59:00Z"), p.clock.toLocal(ms(r.expiresAt)))
        p.advance(61_000)
        assertEquals(ConfirmUi.Status.EXPIRED, ConfirmUi.status(r, s, p.clock.now()))
    }
}
