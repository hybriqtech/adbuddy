package tech.hybriq.adbuddy.chat

import org.junit.Assert.assertEquals
import org.junit.Test
import tech.hybriq.adbuddy.bridge.ChatMessage
import tech.hybriq.adbuddy.bridge.ChatState

class TimelineTest {
    private fun m(id: String, conv: String) = ChatMessage(id, "user", "t", conversation = conv)
    private fun out(id: String, st: SendStatus = SendStatus.SENDING) = Outgoing(id, "t", emptyList(), st)

    @Test fun onlyTheCurrentConversationShows() {
        val chat = ChatState(messages = listOf(m("a", "c_1"), m("b", "c_2"), m("c", "")))
        assertEquals(listOf("ha", "hc"), Timeline.of(chat, "c_1", emptyList()).map { it.key })
        assertEquals("no conversation known yet: all show", listOf("ha", "hb", "hc"), Timeline.of(chat, "", emptyList()).map { it.key })
    }

    @Test fun myMessagesGoAfterAndDisappearWhenTheHostHasThem() {
        val chat = ChatState(messages = listOf(m("a", "c_1")))
        val items = Timeline.of(chat, "c_1", listOf(out("a", SendStatus.SENDING), out("x", SendStatus.FAILED)))
        assertEquals(listOf("ha", "mx"), items.map { it.key })
    }
}
