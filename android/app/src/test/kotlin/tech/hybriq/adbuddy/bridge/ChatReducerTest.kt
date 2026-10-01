package tech.hybriq.adbuddy.bridge

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ChatReducerTest {
    private fun msg(seq: Long, type: String, payload: String) =
        Protocol.decode("""{"v":1,"seq":$seq,"type":"$type","ts":"x","payload":$payload}""")!!

    private fun chat(id: String, text: String, extra: String = "") =
        """{"id":"$id","role":"user","text":"$text","conversation":"c_1"$extra}"""

    @Test fun messagesAreUpsertedById() {
        var s = BridgeState()
        s = s.apply(msg(1, Protocol.CHAT_MESSAGE, chat("a", "one")))
        s = s.apply(msg(2, Protocol.CHAT_MESSAGE, chat("b", "two")))
        s = s.apply(msg(3, Protocol.CHAT_MESSAGE, chat("a", "one, edited")))
        assertEquals(listOf("a", "b"), s.chat.messages.map { it.id })
        assertEquals("one, edited", s.chat.messages[0].text)
        assertEquals(3L, s.lastSeq)
    }

    @Test fun onlyTheLast100AreKept() {
        var s = BridgeState()
        for (i in 1..130) s = s.apply(msg(i.toLong(), Protocol.CHAT_MESSAGE, chat("m$i", "x")))
        assertEquals(ChatReducer.MAX_MESSAGES, s.chat.messages.size)
        assertEquals("m31", s.chat.messages.first().id)
        assertEquals("m130", s.chat.messages.last().id)
    }

    @Test fun nullAttachmentsAndBadPayloadsNeverThrow() {
        var s = BridgeState()
        s = s.apply(msg(1, Protocol.CHAT_MESSAGE, chat("a", "x", ""","attachments":null""")))
        assertTrue(s.chat.messages[0].attachments.isEmpty())
        s = s.apply(msg(2, Protocol.CHAT_MESSAGE, """{"id":5}"""))
        s = s.apply(msg(3, Protocol.CHAT_MESSAGE, """{"role":"user"}"""))
        s = s.apply(msg(4, Protocol.CHAT_MESSAGE, "[]"))
        assertEquals(listOf("a"), s.chat.messages.map { it.id })
        assertEquals(4L, s.lastSeq)
    }

    @Test fun snapshotReplacesTheMessagesAndSkipsTheBadOnes() {
        var s = BridgeState(session = "s_1", lastSeq = 5)
        s = s.apply(msg(6, Protocol.CHAT_MESSAGE, chat("old", "old")))
        val snap = """{"state":{},"chat":[${chat("a", "one")},{"id":7},${chat("b", "two")},${chat("a", "one again")}]}"""
        s = s.apply(msg(7, Protocol.SNAPSHOT, snap))
        assertEquals(listOf("b", "a"), s.chat.messages.map { it.id })
        assertEquals("one again", s.chat.messages.last().text)
    }

    @Test fun aNewSessionForgetsTheChatButAReconnectKeepsIt() {
        var s = BridgeState(session = "s_1", lastSeq = 5).apply(msg(6, Protocol.CHAT_MESSAGE, chat("a", "x")))
        assertEquals(1, s.disconnected().chat.messages.size)
        s = s.apply(msg(0, Protocol.WELCOME, """{"session":"s_2","fromSeq":1,"protocol":1}"""))
        assertTrue(s.chat.messages.isEmpty())
    }
}
