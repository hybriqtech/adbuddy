package tech.hybriq.adbuddy.chat

import org.junit.Assert.assertEquals
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertNull
import org.junit.Test
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.ChatSend
import tech.hybriq.adbuddy.bridge.FakeChannel
import tech.hybriq.adbuddy.bridge.Protocol

class ChatControllerTest {
    private val ch = FakeChannel()
    private var n = 0
    private val chat = ChatController(ch) { "c${++n}" }

    private fun sendOf(i: Int) = Protocol.json.decodeFromJsonElement(ChatSend.serializer(), ch.sent[i].payload)

    @Test fun aSentMessageWaitsForTheHost() {
        ch.handler = { _, _ -> true } // no answer yet
        val id = chat.send("hello")
        assertEquals("c1", id)
        assertEquals(Protocol.CHAT_SEND, ch.sent[0].type)
        assertEquals(ChatSend("c1", "hello", emptyList()), sendOf(0))
        assertEquals(SendStatus.SENDING, chat.outgoing.value.single().status)
    }

    @Test fun theConfirmingMessageDropsIt() {
        ch.handler = { _, cb -> cb(FakeChannel.reply(Protocol.CHAT_MESSAGE, """{"id":"c1","role":"user"}""", "r1")); true }
        chat.send("hello")
        assertEquals(emptyList<Outgoing>(), chat.outgoing.value)
    }

    @Test fun anErrorFailsItAndShowsTheHostsText() = runBlocking {
        ch.handler = { _, cb -> cb(FakeChannel.reply(Protocol.ERROR, """{"code":"busy","text":"Answer the question in the agent's terminal first"}""", "r1")); true }
        val seen = mutableListOf<String>()
        val job = launch(Dispatchers.Unconfined, start = CoroutineStart.UNDISPATCHED) { chat.errors.collect { seen += it } }
        chat.send("hello")
        job.cancel()
        assertEquals(SendStatus.FAILED, chat.outgoing.value.single().status)
        assertEquals(listOf("Answer the question in the agent's terminal first"), seen)
    }

    @Test fun aLostConnectionFailsItAndRetryKeepsTheSameId() {
        ch.handler = { _, cb -> cb(null); true }
        chat.send("hello")
        assertEquals(SendStatus.FAILED, chat.outgoing.value.single().status)
        ch.handler = { _, _ -> true }
        chat.retry("c1")
        assertEquals(SendStatus.SENDING, chat.outgoing.value.single().status)
        assertEquals("the host drops a repeated id, so retrying never duplicates", "c1", sendOf(1).id)
    }

    @Test fun nothingToSendIsNotSent() {
        assertNull(chat.send("   "))
        assertEquals(0, ch.sent.size)
        val a = Attachment("00112233445566778899aabbccddeeff", "a.png", "image/png", 3, "ab")
        chat.send("", listOf(a))
        assertEquals(listOf(a.id), sendOf(0).attachments)
    }

    @Test fun anUnsendableRequestFailsAtOnce() {
        ch.handler = { _, _ -> false }
        chat.send("hello")
        assertEquals(SendStatus.FAILED, chat.outgoing.value.single().status)
    }

    @Test fun discardRemovesIt() {
        ch.handler = { _, cb -> cb(null); true }
        chat.send("hello")
        chat.discard("c1")
        assertEquals(0, chat.outgoing.value.size)
    }
}
