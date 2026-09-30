package tech.hybriq.adbuddy.bridge

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
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
        var s = BridgeState(session = "s_1", lastSeq = 50).apply(msg(51, Protocol.CONFIRM_REQUEST, question))
        assertEquals(1, s.pending.size)
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
    private fun welcome(session: String) = msg(0, Protocol.WELCOME, """{"session":"$session","fromSeq":1,"protocol":1}""")

    // A journal record exactly as Go's encoding/json writes it (host/internal/journal.Entry): nil slices and pointers are null.
    private fun goEntry(id: String, status: String) =
        """{"id":"$id","ts":"2026-09-30T01:02:03Z","session":"s_1","serial":"192.0.2.10:5555","source":"agent","argv":null,"level":"red","rule":"reboot","description":"Restart","status":"$status","exitCode":null,"inverse":null,"backups":null,"undoneBy":null,"mac":"00ff"}"""

    @Test fun aNewSessionForgetsThePause() {
        var s = BridgeState().apply(welcome("s_1"))
        s = s.apply(msg(1, Protocol.SESSION_STATE, """{"paused":true}"""))
        assertTrue(s.sessionState.paused)
        s = s.apply(welcome("s_2"))
        assertFalse(s.sessionState.paused)
    }

    @Test fun resumingTheSameSessionKeepsItsState() {
        var s = BridgeState().apply(welcome("s_1"))
        s = s.apply(msg(1, Protocol.CONFIRM_REQUEST, question))
        s = s.apply(welcome("s_1"))
        assertEquals(1L, s.lastSeq)
        assertEquals(listOf("j_1"), s.pending.map { it.id })
    }

    @Test fun goNullsDecodeAndAnswerTheQuestion() {
        var s = BridgeState().apply(welcome("s_1"))
        s = s.apply(msg(1, Protocol.CONFIRM_REQUEST, question))
        s = s.apply(msg(2, Protocol.JOURNAL_ENTRY, goEntry("j_1", "ok")))
        assertTrue(s.pending.isEmpty())
        assertEquals(listOf("j_1"), s.journal.map { it.id })
        assertEquals(emptyList<String>(), s.journal[0].argv)
    }

    @Test fun aSnapshotWithGoNullsHasEmptyLists() {
        var s = BridgeState(session = "s_1", lastSeq = 3).apply(msg(4, Protocol.CONFIRM_REQUEST, question))
        s = s.apply(msg(5, Protocol.SNAPSHOT, """{"state":{"agent":"","state":"idle","activity":"","paused":false},"pending":null,"journal":null}"""))
        assertTrue(s.pending.isEmpty())
        assertTrue(s.journal.isEmpty())
        assertEquals(5L, s.lastSeq)
    }

    @Test fun aSnapshotWithGoJournalEntries() {
        val s = BridgeState(session = "s_1").apply(msg(5, Protocol.SNAPSHOT, """{"state":{},"pending":[],"journal":[${goEntry("j_0", "ok")}]}"""))
        assertEquals(listOf("j_0"), s.journal.map { it.id })
    }

    @Test fun anUnreadableSnapshotDropsTheQuestions() {
        var s = BridgeState(session = "s_1").apply(msg(1, Protocol.CONFIRM_REQUEST, question))
        s = s.apply(msg(2, Protocol.SNAPSHOT, """{"pending":"nope"}"""))
        assertTrue(s.pending.isEmpty())
        assertEquals(2L, s.lastSeq)
    }

    @Test fun anAnsweredQuestionStaysUntilItsFinalEntry() {
        var s = BridgeState().apply(welcome("s_1")).apply(msg(1, Protocol.CONFIRM_REQUEST, question))
        s = s.apply(msg(2, Protocol.CONFIRM_REQUEST, question.replace("j_1", "j_2")))
        s = s.answered("j_1")
        assertEquals("sending, not gone", listOf("j_1", "j_2"), s.pending.map { it.id })
        assertEquals(setOf("j_1"), s.answering)
        s = s.apply(msg(3, Protocol.JOURNAL_ENTRY, """{"id":"j_1","status":"running"}"""))
        assertEquals(setOf("j_1"), s.answering)
        s = s.apply(msg(4, Protocol.JOURNAL_ENTRY, """{"id":"j_1","status":"denied"}"""))
        assertEquals(listOf("j_2"), s.pending.map { it.id })
        assertTrue(s.answering.isEmpty())
    }

    @Test fun aDroppedConnectionMakesTheQuestionAnswerableAgain() {
        val s = BridgeState().apply(welcome("s_1")).apply(msg(1, Protocol.CONFIRM_REQUEST, question))
            .answered("j_1").disconnected()
        assertEquals(BridgeState.Connection.WAITING, s.connection)
        assertEquals(listOf("j_1"), s.pending.map { it.id })
        assertTrue(s.answering.isEmpty())
    }

    @Test fun answeringAnUnknownQuestionChangesNothing() {
        val s = BridgeState().apply(msg(1, Protocol.CONFIRM_REQUEST, question))
        assertEquals(s, s.answered("j_9"))
    }

    @Test fun answersLeaveWithTheirQuestions() {
        val s = BridgeState(session = "s_1").apply(msg(1, Protocol.CONFIRM_REQUEST, question)).answered("j_1")
        assertTrue("new session", s.apply(welcome("s_2")).answering.isEmpty())
        assertTrue("snapshot without it", s.apply(msg(2, Protocol.SNAPSHOT, """{"pending":[]}""")).answering.isEmpty())
        assertTrue("old host", s.apply(msg(0, Protocol.WELCOME, """{"session":"s_1","fromSeq":1}""")).answering.isEmpty())
        assertTrue("old app", s.appOutdated().answering.isEmpty())
    }

    @Test fun aFinalEntryOfAnotherIdKeepsTheQuestion() {
        var s = BridgeState().apply(msg(1, Protocol.CONFIRM_REQUEST, question))
        s = s.apply(msg(2, Protocol.JOURNAL_ENTRY, """{"id":"j_9","status":"ok"}"""))
        assertEquals(listOf("j_1"), s.pending.map { it.id })
    }

    @Test fun anOldHostDropsTheQuestionsAndOnlyAWelcomeGetsOut() {
        var s = BridgeState(session = "s_1").apply(msg(1, Protocol.CONFIRM_REQUEST, question))
        s = s.apply(msg(0, Protocol.WELCOME, """{"session":"s_1","fromSeq":1}"""))
        assertEquals(BridgeState.Connection.INCOMPATIBLE, s.connection)
        assertTrue(s.pending.isEmpty())
        val stuck = s
        s = s.apply(msg(2, Protocol.CONFIRM_REQUEST, question))
            .apply(msg(3, Protocol.SESSION_STATE, """{"paused":true}"""))
            .apply(msg(4, Protocol.JOURNAL_ENTRY, """{"id":"j_1","status":"ok"}"""))
            .apply(msg(5, Protocol.SNAPSHOT, """{"state":{},"pending":[$question],"journal":[]}"""))
        assertEquals(stuck, s)
        s = s.apply(welcome("s_1"))
        assertEquals(BridgeState.Connection.CONNECTED, s.connection)
        assertEquals(BridgeState.Outdated.NONE, s.outdated)
    }

    @Test fun anOutdatedAppIgnoresEverythingButAWelcome() {
        val s = BridgeState(session = "s_1", lastSeq = 1).appOutdated()
        assertEquals(s, s.apply(msg(2, Protocol.CONFIRM_REQUEST, question)))
    }

    @Test fun anUnreadableWelcomeMeansAnOldHost() {
        val s = BridgeState(session = "s_1").apply(msg(1, Protocol.CONFIRM_REQUEST, question))
            .apply(msg(0, Protocol.WELCOME, """{"session":7}"""))
        assertEquals(BridgeState.Connection.INCOMPATIBLE, s.connection)
        assertEquals(BridgeState.Outdated.HOST, s.outdated)
        assertTrue(s.pending.isEmpty())
    }

    @Test fun aReplacedEntryKeepsItsPlace() {
        var s = BridgeState().apply(msg(1, Protocol.JOURNAL_ENTRY, """{"id":"j_1","status":"running"}"""))
        s = s.apply(msg(2, Protocol.JOURNAL_ENTRY, """{"id":"j_2","status":"ok"}"""))
        s = s.apply(msg(3, Protocol.JOURNAL_ENTRY, """{"id":"j_1","status":"ok"}"""))
        assertEquals(listOf("j_1", "j_2"), s.journal.map { it.id })
        assertEquals("ok", s.journal[0].status)
    }

    @Test fun repeatedEventsAreIgnored() {
        var s = BridgeState().apply(welcome("s_1"))
        s = s.apply(msg(1, Protocol.CONFIRM_REQUEST, question))
        s = s.apply(msg(2, Protocol.JOURNAL_ENTRY, """{"id":"j_1","status":"denied"}"""))
        val before = s
        s = s.apply(msg(1, Protocol.CONFIRM_REQUEST, question)).apply(msg(2, Protocol.JOURNAL_ENTRY, """{"id":"j_1","status":"running"}"""))
        assertEquals(before, s)
        assertTrue(s.pending.isEmpty())
    }
}
