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
import tech.hybriq.adbuddy.bridge.PermissionReply
import tech.hybriq.adbuddy.bridge.PermissionRequest
import kotlinx.coroutines.cancelChildren
import tech.hybriq.adbuddy.bridge.Protocol

class ChatControllerTest {
    private val ch = FakeChannel()
    private var n = 0
    private var now = 1_000_000L
    private val chat = ChatController(ch, clock = { now }) { "c${++n}" }

    /** Shows the cards and lets the guard of the first one pass, as the screen does. */
    private fun show(vararg ids: String) {
        chat.onPermissions(ids.map { req(it) })
        now += PermissionUi.PROMOTE_GUARD_MS
    }

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

    @Test fun aNewConversationAsksTheHostAndShowsItsRefusal() = runBlocking {
        ch.handler = { _, cb -> cb(FakeChannel.reply(Protocol.ERROR, """{"code":"failed","text":"Could not start the agent"}""", "r1")); true }
        val seen = mutableListOf<String>()
        val job = launch(Dispatchers.Unconfined, start = CoroutineStart.UNDISPATCHED) { chat.errors.collect { seen += it } }
        chat.newConversation()
        job.cancel()
        assertEquals(Protocol.SESSION_NEW, ch.sent.single().type)
        assertEquals(listOf("Could not start the agent"), seen)
    }

    @Test fun aPermissionAnswerIsAStreamMessageWithNoRid() {
        show("p_1")
        assertEquals(true, chat.answerPermission("p_1", "allow_always"))
        val sent = ch.sent.single()
        assertEquals(Protocol.PERMISSION_REPLY, sent.type)
        assertNull("it asks for no reply: the card goes with permission.resolved", sent.rid)
        assertEquals(PermissionReply("p_1", "allow_always"), Protocol.json.decodeFromJsonElement(PermissionReply.serializer(), sent.payload))
        assertEquals("a permission answer is not a message of the chat", emptyList<Outgoing>(), chat.outgoing.value)
    }

    private fun req(id: String) = PermissionRequest(id, "Bash", "touch x", "touch x", "2026-10-01T10:10:00Z", true)

    private fun answers() = ch.sent.filter { it.type == Protocol.PERMISSION_REPLY }

    @Test fun twoAnswersToTheSameIdSendOnlyOne() {
        show("p_1")
        assertEquals(true, chat.answerPermission("p_1", "allow"))
        assertEquals("a second tap is not sent", false, chat.answerPermission("p_1", "deny"))
        assertEquals(1, answers().size)
        assertEquals("allow", Protocol.json.decodeFromJsonElement(PermissionReply.serializer(), answers()[0].payload).decision)
        assertEquals(setOf("p_1"), chat.answering.value)
    }

    @Test fun anotherIdCanStillBeAnswered() {
        show("p_1")
        assertEquals(true, chat.answerPermission("p_1", "allow"))
        chat.onPermissions(listOf(req("p_2")))
        now += PermissionUi.PROMOTE_GUARD_MS
        assertEquals(true, chat.answerPermission("p_2", "deny"))
        assertEquals(2, answers().size)
    }

    @Test fun onlyTheProtocolsDecisionsAreSent() {
        show("p_1")
        for (d in listOf("", "maybe", "ALLOW", "allow ")) assertEquals(d, false, chat.answerPermission("p_1", d))
        assertEquals(0, ch.sent.size)
        assertEquals("a refused decision does not mark the card as answering", emptySet<String>(), chat.answering.value)
    }

    @Test fun anUnsentAnswerCanBeTriedAgain() {
        val c = ChatController(object : tech.hybriq.adbuddy.bridge.Channel by ch {
            override fun send(type: String, payload: kotlinx.serialization.json.JsonElement) = false
        })
        c.onPermissions(listOf(req("p_1")))
        now += 600
        assertEquals(false, c.answerPermission("p_1", "allow"))
        assertEquals(emptySet<String>(), c.answering.value)
    }

    @Test fun theResolvedOfTheHostEndsTheAnswering() {
        show("p_1")
        chat.answerPermission("p_1", "allow")
        ch.emit(Protocol.PERMISSION_RESOLVED, """{"id":"p_1","decision":"allow","by":"phone"}""")
        assertEquals(emptySet<String>(), chat.answering.value)
    }

    @Test fun aCardGoneFromTheListEndsItsAnswering() {
        show("p_1", "p_2")
        chat.answerPermission("p_1", "allow")
        chat.onPermissions(listOf(req("p_2")))
        assertEquals(emptySet<String>(), chat.answering.value)
    }

    @Test fun aFreshlyPromotedCardCannotBeAnsweredYet() {
        show("p_1", "p_2")
        assertEquals(true, chat.answerPermission("p_1", "allow"))
        // p_1 resolved: p_2 takes its place under the thumb
        chat.onPermissions(listOf(req("p_2")))
        now += 599
        assertEquals(false, chat.canAnswer("p_2"))
        assertEquals(1L, chat.guardLeftMs("p_2"))
        assertEquals("the second tap does not grant p_2", false, chat.answerPermission("p_2", "allow"))
        assertEquals(1, answers().size)
        now += 1
        assertEquals(0L, chat.guardLeftMs("p_2"))
        assertEquals(true, chat.answerPermission("p_2", "allow"))
        assertEquals(2, answers().size)
    }

    @Test fun aCardThatIsNotTheFirstCannotBeAnswered() {
        show("p_1", "p_2")
        assertEquals(false, chat.answerPermission("p_2", "allow"))
        assertEquals(0, answers().size)
    }

    @Test fun afterALostConnectionTheCardCanBeAnsweredAgain() = runBlocking {
        val c = ChatController(ch, scope = this, clock = { now }) { "c" }
        ch.setPermissions(listOf(req("p_1")))
        kotlinx.coroutines.yield()
        now += 600
        c.answerPermission("p_1", "allow")
        assertEquals(false, c.answerPermission("p_1", "allow"))
        ch.setConnected(false)
        kotlinx.coroutines.yield()
        assertEquals(emptySet<String>(), c.answering.value)
        ch.setConnected(true)
        kotlinx.coroutines.yield()
        assertEquals("a reconnection rearms the guard", false, c.answerPermission("p_1", "allow"))
        now += 600
        assertEquals(true, c.answerPermission("p_1", "allow"))
        assertEquals(2, answers().size)
        coroutineContext.cancelChildren()
    }

    @Test fun theGuardEpochChangesWhenTheGuardRestarts() = runBlocking {
        val c = ChatController(ch, scope = this, clock = { now }) { "c" }
        val e0 = c.guardEpoch.value
        ch.setPermissions(listOf(req("p_1")))
        kotlinx.coroutines.yield()
        val e1 = c.guardEpoch.value
        assertEquals("a new card", true, e1 != e0)
        ch.setConnected(false)
        kotlinx.coroutines.yield()
        ch.setConnected(true)
        kotlinx.coroutines.yield()
        assertEquals("a reconnection, after the guard was rearmed", true, c.guardEpoch.value != e1)
        assertEquals(PermissionUi.PROMOTE_GUARD_MS, c.guardLeftMs("p_1"))
        coroutineContext.cancelChildren()
    }

    @Test fun nothingObservedMeansNothingCanBeAnswered() {
        now += 10_000
        assertEquals(false, chat.canAnswer("p_1"))
        assertEquals(false, chat.answerPermission("p_1", "allow"))
        assertEquals(0, ch.sent.size)
    }

    @Test fun anEmptyListThenANewCardGuardsTheNewCard() {
        chat.onPermissions(emptyList())
        now += 10_000
        assertEquals("a card nobody listed", false, chat.answerPermission("p_1", "allow"))
        chat.onPermissions(listOf(req("p_1")))
        assertEquals(false, chat.answerPermission("p_1", "allow"))
        now += 599
        assertEquals(false, chat.answerPermission("p_1", "allow"))
        now += 1
        assertEquals(true, chat.answerPermission("p_1", "allow"))
    }

    @Test fun theControllerFollowsTheBridgesPermissionsByItself() = runBlocking {
        val c = ChatController(ch, scope = this, clock = { now }) { "c" }
        ch.setPermissions(listOf(req("p_1")))
        kotlinx.coroutines.yield()
        assertEquals(false, c.answerPermission("p_1", "allow"))
        now += 600
        assertEquals(true, c.answerPermission("p_1", "allow"))
        ch.setPermissions(emptyList())
        kotlinx.coroutines.yield()
        assertEquals("the card went: its answering too", emptySet<String>(), c.answering.value)
        coroutineContext.cancelChildren()
    }
}
