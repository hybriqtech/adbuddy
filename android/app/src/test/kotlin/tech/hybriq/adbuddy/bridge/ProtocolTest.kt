package tech.hybriq.adbuddy.bridge

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ProtocolTest {
    // A message exactly as host/internal/bridge writes it.
    private val hostConfirm = """{"v":1,"seq":7,"type":"confirm.request","ts":"2026-09-30T01:02:03Z","payload":{"id":"j_1","level":"red","title":"Restart the phone","detail":"may restart","rawCommand":"adb shell reboot","expiresAt":"2026-09-30T01:04:03Z","special":true,"allowSession":false,"future":"ignored"}}"""

    @Test fun decodesHostMessages() {
        val m = Protocol.decode(hostConfirm)!!
        assertEquals(7L, m.seq)
        val r = Protocol.payload<ConfirmRequest>(m)!!
        assertEquals("adb shell reboot", r.rawCommand)
        assertTrue(r.special)
        assertFalse(r.allowSession)
    }

    @Test fun rejectsOtherVersionsAndGarbage() {
        assertNull(Protocol.decode("""{"v":2,"type":"notice","payload":{}}"""))
        assertNull(Protocol.decode("""{"v":1,"type":"","payload":{}}"""))
        assertNull(Protocol.decode("not json"))
    }

    @Test fun encodesHelloAndReplies() {
        val hello = Protocol.encode(Protocol.HELLO, Hello(token = "t", lastSeq = 3, session = "s1", appVersion = "0.2.0"), "2026-09-30T00:00:00Z")
        assertTrue(hello, hello.contains(""""type":"hello"""") && hello.contains(""""lastSeq":3""") && hello.contains(""""v":1"""))
        assertTrue(hello, hello.contains(""""session":"s1""""))
        val reply = Protocol.encode(Protocol.CONFIRM_REPLY, ConfirmReply("j_1", Decision.ALLOW_SESSION), "x")
        assertTrue(reply, reply.contains(""""decision":"allow_session""""))
    }
}
