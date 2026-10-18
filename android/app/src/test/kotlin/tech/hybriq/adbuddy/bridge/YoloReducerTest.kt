package tech.hybriq.adbuddy.bridge

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class YoloReducerTest {
    private fun msg(seq: Long, type: String, payload: String) =
        Protocol.decode("""{"v":1,"seq":$seq,"type":"$type","ts":"x","payload":$payload}""")!!

    private val on = """{"active":true,"until":1790849700000,"session":false}"""
    private val onState = YoloState(active = true, until = 1790849700000, session = false)

    @Test fun startsUnknown() {
        assertNull(BridgeState().yolo)
    }

    @Test fun anEventSetsItAndAnOffIsKnown() {
        var s = BridgeState(session = "s_1").apply(msg(1, Protocol.YOLO_STATE, on))
        assertEquals(onState, s.yolo)
        s = s.apply(msg(2, Protocol.YOLO_STATE, """{"active":false,"until":0,"session":false}"""))
        assertEquals(YoloState(), s.yolo)
    }

    // Unreadable is unknown, never off: a false off would hide the band of a mode that is on.
    // Mutation: make read() return YoloState() instead of null.
    @Test fun anUnreadablePayloadIsUnknownNeverOff() {
        for (bad in listOf("{}", """{"active":"yes"}""", """{"active":null}""", """{"active":true,"until":"x"}""", "\"x\"", "[]", "5")) {
            val s = BridgeState(session = "s_1", yolo = onState).apply(msg(1, Protocol.YOLO_STATE, bad))
            assertNull(bad, s.yolo)
        }
    }

    @Test fun aSnapshotSetsItAndWithoutItMakesItUnknown() {
        var s = BridgeState(session = "s_1").apply(msg(1, Protocol.SNAPSHOT, """{"yolo":$on}"""))
        assertEquals(onState, s.yolo)
        s = s.apply(msg(2, Protocol.SNAPSHOT, "{}"))
        assertNull(s.yolo)
    }

    @Test fun aMalformedYoloPartOfASnapshotIsUnknownAndKeepsTheQuestions() {
        val question = """{"id":"j_1","title":"Restart","rawCommand":"adb shell reboot","expiresAt":"2026-09-30T01:04:03Z"}"""
        val s = BridgeState(session = "s_1", yolo = onState).apply(msg(1, Protocol.SNAPSHOT, """{"pending":[$question],"yolo":"x"}"""))
        assertNull(s.yolo)
        assertEquals(1, s.pending.size)
    }

    // The mode outlives the app's connection: the band stays while the link is down.
    // Mutation: make reset() return null for DISCONNECTED.
    @Test fun aDroppedConnectionKeepsIt() {
        val s = BridgeState(session = "s_1", yolo = onState).disconnected()
        assertEquals(onState, s.yolo)
        assertEquals(onState, BridgeState(session = "s_1", yolo = onState).appOutdated().yolo)
        // A welcome of a host too old to talk to (protocol below the minimum) keeps it too.
        val old = BridgeState(session = "s_1", yolo = onState).apply(msg(0, Protocol.WELCOME, """{"session":"s_1","fromSeq":1,"protocol":0}"""))
        assertEquals(BridgeState.Connection.INCOMPATIBLE, old.connection)
        assertEquals(onState, old.yolo)
    }

    @Test fun aNewSessionOrAnUnreadableSnapshotForgetsIt() {
        val newSession = BridgeState(session = "s_1", yolo = onState).apply(msg(0, Protocol.WELCOME, """{"session":"s_2","fromSeq":1,"protocol":1}"""))
        assertNull(newSession.yolo)
        val unreadable = BridgeState(session = "s_1", yolo = onState).apply(msg(1, Protocol.SNAPSHOT, "\"x\""))
        assertNull(unreadable.yolo)
        // The same session after a reconnection keeps it.
        val same = BridgeState(session = "s_1", yolo = onState).apply(msg(0, Protocol.WELCOME, """{"session":"s_1","fromSeq":1,"protocol":1}"""))
        assertTrue(same.yolo?.active == true)
    }
}
