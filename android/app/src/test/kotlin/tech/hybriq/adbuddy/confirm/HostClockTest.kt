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
    private fun msg(type: String, ts: String) =
        Protocol.decode("""{"v":1,"seq":1,"type":"$type","ts":"$ts","payload":{}}""")!!

    @Test fun theSkewIsHowFarAheadTheHostIs() {
        assertEquals(5_000L, HostClock.skewOf("2026-09-30T10:00:05Z", local))
        assertEquals(-120_000L, HostClock.skewOf("2026-09-30T09:58:00Z", local))
    }

    @Test fun anUnreadableTimeGivesNoSkew() {
        assertNull(HostClock.skewOf("", local))
        assertNull(HostClock.skewOf("yesterday", local))
    }

    @Test fun theMessageThatBringsAQuestionSetsTheSkew() {
        val c = HostClock { local }
        assertEquals(local, c.now())
        c.observe(msg(Protocol.CONFIRM_REQUEST, "2026-09-30T10:02:00Z"))
        assertEquals("host time", local + 120_000, c.now())
        c.observe(msg(Protocol.JOURNAL_ENTRY, "2026-09-30T11:00:00Z"))
        assertEquals("only messages that bring questions count", local + 120_000, c.now())
        c.observe(msg(Protocol.SNAPSHOT, "2026-09-30T09:59:00Z"))
        assertEquals(local - 60_000, c.now())
        c.observe(msg(Protocol.CONFIRM_REQUEST, "garbage"))
        assertEquals("kept", local - 60_000, c.now())
    }

    @Test fun aPhoneBehindTheHostSeesTheQuestionExpireOnTime() {
        // The host asks at 10:00:00 its time, for 60 s; the phone's clock says 09:58:00.
        val phone = Instant.parse("2026-09-30T09:58:00Z").toEpochMilli()
        var t = phone
        val c = HostClock { t }
        c.observe(msg(Protocol.CONFIRM_REQUEST, "2026-09-30T10:00:00Z"))
        val r = ConfirmRequest(id = "j_1", title = "Restart", rawCommand = "adb shell reboot", expiresAt = "2026-09-30T10:01:00Z")
        val s = BridgeState(connection = BridgeState.Connection.CONNECTED, pending = listOf(r))
        assertEquals(ConfirmUi.Status.OPEN, ConfirmUi.status(r, s, c.now()))
        t = phone + 61_000 // 09:59:01 on the phone, 10:01:01 on the host
        assertEquals(ConfirmUi.Status.EXPIRED, ConfirmUi.status(r, s, c.now()))
        t = phone
        assertEquals("the notification times out after what is left", 60_000L, ConfirmUi.plan(emptyMap(), s, c.now()).post.single().left)
        assertEquals("the expiry shown in the phone's time", phone + 60_000, c.toLocal(Instant.parse(r.expiresAt).toEpochMilli()))
    }
}
