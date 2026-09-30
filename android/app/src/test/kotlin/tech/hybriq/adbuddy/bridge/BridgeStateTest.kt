package tech.hybriq.adbuddy.bridge

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class BridgeStateTest {
    private fun msg(seq: Long, type: String, payload: String) =
        Protocol.decode("""{"v":1,"seq":$seq,"type":"$type","ts":"x","payload":$payload}""")!!

    private val question = """{"id":"j_1","title":"Restart","rawCommand":"adb shell reboot","expiresAt":"2026-09-30T01:04:03Z"}"""

    @Test fun questionsLeaveWhenTheirEntryIsFinal() {
        var s = BridgeState()
        s = s.apply(msg(0, Protocol.WELCOME, """{"session":"s_1","fromSeq":1,"protocol":1}"""))
        assertEquals(BridgeState.Connection.CONNECTED, s.connection)
        s = s.apply(msg(1, Protocol.CONFIRM_REQUEST, question))
        assertEquals(1, s.pending.size)
        s = s.apply(msg(2, Protocol.JOURNAL_ENTRY, """{"id":"j_1","status":"running","level":"red"}"""))
        assertEquals("a running record keeps the question", 1, s.pending.size)
        s = s.apply(msg(3, Protocol.JOURNAL_ENTRY, """{"id":"j_1","status":"denied","level":"red"}"""))
        assertTrue(s.pending.isEmpty())
        assertEquals(1, s.journal.size)
        assertEquals("denied", s.journal[0].status)
        assertEquals(3L, s.lastSeq)
    }

    @Test fun snapshotReplacesEverything() {
        var s = BridgeState(session = "s_1", lastSeq = 9)
        s = s.apply(msg(10, Protocol.SNAPSHOT, """{"state":{"paused":true},"pending":[$question],"journal":[{"id":"j_0","status":"ok"}]}"""))
        assertTrue(s.sessionState.paused)
        assertEquals(listOf("j_1"), s.pending.map { it.id })
        assertEquals(listOf("j_0"), s.journal.map { it.id })
        assertEquals(10L, s.lastSeq)
    }

    @Test fun anotherSessionStartsOver() {
        var s = BridgeState(session = "s_1", lastSeq = 50).apply(msg(1, Protocol.CONFIRM_REQUEST, question))
        s = s.apply(msg(0, Protocol.WELCOME, """{"session":"s_2","fromSeq":1,"protocol":1}"""))
        assertEquals(0L, s.lastSeq)
        assertTrue(s.pending.isEmpty())
    }

    @Test fun badPayloadsAreIgnored() {
        val s = BridgeState().apply(msg(4, Protocol.CONFIRM_REQUEST, """{"id":5}"""))
        assertTrue(s.pending.isEmpty())
        assertEquals(4L, s.lastSeq)
    }

    @Test fun anOldHostIsIncompatible() {
        val s = BridgeState().apply(msg(0, Protocol.WELCOME, """{"session":"s_1","fromSeq":1}"""))
        assertEquals(BridgeState.Connection.INCOMPATIBLE, s.connection)
        assertEquals(BridgeState.Outdated.HOST, s.outdated)
    }

    @Test fun anOldAppIsIncompatible() {
        val s = BridgeState(pending = listOf(Protocol.json.decodeFromString(ConfirmRequest.serializer(), question))).appOutdated()
        assertEquals(BridgeState.Connection.INCOMPATIBLE, s.connection)
        assertEquals(BridgeState.Outdated.APP, s.outdated)
        assertTrue(s.pending.isEmpty())
    }
}
