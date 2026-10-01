package tech.hybriq.adbuddy.chat

import java.util.UUID
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.Channel
import tech.hybriq.adbuddy.bridge.ChatSend
import tech.hybriq.adbuddy.bridge.ErrorReply
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.PermissionReply
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.SessionNew

/** A confirmed message is not here any more: Bridge applies the host's chat.message to the state before it calls onResult. */
enum class SendStatus { SENDING, FAILED }

/** A message written on this phone that the chat shows until the host's copy arrives (spec M3 §7.2). */
data class Outgoing(val id: String, val text: String, val attachments: List<Attachment>, val status: SendStatus)

/**
 * Sends the person's messages (chat.send) and keeps the ones the host has not
 * confirmed. The id is the message's for good: the host drops a repeat, so
 * Retry never duplicates (protocol spec §3).
 */
class ChatController(
    private val channel: Channel,
    private val newId: () -> String = { "c" + UUID.randomUUID().toString().replace("-", "") },
) {
    private val _outgoing = MutableStateFlow<List<Outgoing>>(emptyList())
    val outgoing: StateFlow<List<Outgoing>> = _outgoing.asStateFlow()

    private val _errors = MutableSharedFlow<String>(extraBufferCapacity = 8)

    /** The host's own text of each refused message, for a snackbar. */
    val errors: SharedFlow<String> = _errors.asSharedFlow()

    /** The id of the message, or null when there was nothing to send. */
    fun send(text: String, attachments: List<Attachment> = emptyList()): String? {
        if (text.isBlank() && attachments.isEmpty()) return null
        val o = Outgoing(newId(), text, attachments, SendStatus.SENDING)
        _outgoing.update { it + o }
        post(o)
        return o.id
    }

    fun retry(id: String) {
        val o = _outgoing.value.firstOrNull { it.id == id && it.status == SendStatus.FAILED } ?: return
        set(id, SendStatus.SENDING)
        post(o)
    }

    /** Starts another conversation (session.new); the host's refusal goes to `errors`. The new state arrives as session.state. */
    fun newConversation() {
        val payload = Protocol.json.encodeToJsonElement(SessionNew.serializer(), SessionNew())
        channel.request(Protocol.SESSION_NEW, payload) { m ->
            if (m != null && m.type == Protocol.ERROR) Protocol.payload<ErrorReply>(m)?.let { _errors.tryEmit(it.text.ifEmpty { it.code }) }
        }
    }

    /**
     * Answers a question of the agent (permission.reply, M4). It has no rid and no reply: the card goes when the
     * host's permission.resolved arrives. false when it could not be sent (the card stays; the buttons are off then).
     */
    fun answerPermission(id: String, decision: String): Boolean {
        val payload = Protocol.json.encodeToJsonElement(PermissionReply.serializer(), PermissionReply(id, decision))
        return channel.send(Protocol.PERMISSION_REPLY, payload)
    }

    fun discard(id: String) = _outgoing.update { l -> l.filterNot { it.id == id } }

    private fun post(o: Outgoing) {
        val payload = Protocol.json.encodeToJsonElement(ChatSend.serializer(), ChatSend(o.id, o.text, o.attachments.map { it.id }))
        val rid = channel.request(Protocol.CHAT_SEND, payload) { m -> done(o.id, m) }
        if (rid == null) set(o.id, SendStatus.FAILED)
    }

    private fun done(id: String, m: Message?) {
        when {
            m == null -> set(id, SendStatus.FAILED) // the connection ended first
            m.type == Protocol.ERROR -> {
                set(id, SendStatus.FAILED)
                Protocol.payload<ErrorReply>(m)?.let { _errors.tryEmit(it.text.ifEmpty { it.code }) }
            }
            else -> discard(id) // the host's own chat.message is already in the state
        }
    }

    private fun set(id: String, st: SendStatus) = _outgoing.update { l -> l.map { if (it.id == id) it.copy(status = st) else it } }
}
