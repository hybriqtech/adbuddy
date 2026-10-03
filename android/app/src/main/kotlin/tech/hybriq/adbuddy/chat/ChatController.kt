package tech.hybriq.adbuddy.chat

import java.util.UUID
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.Channel
import tech.hybriq.adbuddy.bridge.ChatSend
import tech.hybriq.adbuddy.bridge.ErrorReply
import tech.hybriq.adbuddy.bridge.ErrorText
import tech.hybriq.adbuddy.bridge.ErrorTexts
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.PermissionReply
import tech.hybriq.adbuddy.bridge.PermissionRequest
import tech.hybriq.adbuddy.bridge.PermissionResolved
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
    /** With a scope, a lost connection frees the answered cards (the answer may never have arrived). */
    scope: CoroutineScope? = null,
    clock: () -> Long = { System.nanoTime() / 1_000_000 },
    /** How the host's refusals read on screen (ErrorText); the tests' default is the host's own text. */
    private val errorText: ErrorTexts = ErrorText.plain,
    private val newId: () -> String = { "c" + UUID.randomUUID().toString().replace("-", "") },
) {
    private val _outgoing = MutableStateFlow<List<Outgoing>>(emptyList())
    val outgoing: StateFlow<List<Outgoing>> = _outgoing.asStateFlow()

    private val guard = PermissionGuard(clock)
    private val _answering = MutableStateFlow<Set<String>>(emptySet())

    /** The permission cards answered here whose permission.resolved has not come: their buttons stay off. */
    val answering: StateFlow<Set<String>> = _answering.asStateFlow()

    private val _guardEpoch = MutableStateFlow(0)

    /** Changes each time the promote guard is (re)started (a new first card, a reconnection): the screen re-reads [guardLeftMs] and [canAnswer] then. */
    val guardEpoch: StateFlow<Int> = _guardEpoch.asStateFlow()

    init {
        channel.listen(Protocol.PERMISSION_RESOLVED) { m -> Protocol.payload<PermissionResolved>(m)?.let { r -> _answering.update { it - r.id } } }
        // An answer sent on a socket that dropped may never have reached the host: the card can be answered again.
        scope?.launch { channel.connected.collect { if (!it) _answering.value = emptySet() else { guard.rearm(); _guardEpoch.update { it + 1 } } } }
        // The cards come from the bridge's state: the screen does not have to tell the controller.
        scope?.launch { channel.permissions.collect { onPermissions(it) } }
    }

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
            if (m != null && m.type == Protocol.ERROR) Protocol.payload<ErrorReply>(m)?.let { _errors.tryEmit(errorText(it)) }
        }
    }

    /** The cards, oldest first (the bridge's list, each time it changes; with a scope the constructor follows it by itself): starts the promote guard and forgets the answered ones that went. */
    fun onPermissions(permissions: List<PermissionRequest>) {
        guard.observe(permissions.map { it.id })
        _guardEpoch.update { it + 1 }
        val ids = permissions.map { it.id }.toSet()
        _answering.update { a -> a.filter { it in ids }.toSet() }
    }

    /** RED stub. */
    fun rearmGuard() {}

    /** Milliseconds before the card may be answered (its promote guard); 0 when it may, or when it never may (then [canAnswer] says no). Finite: fit for PermissionUi.nextTickMs. */
    fun guardLeftMs(id: String): Long = guard.leftMs(id) ?: 0L

    /** Whether the card may be answered now, apart from the connection and its time (PermissionUi.canAnswer): not answered yet, the first one, and past its guard. */
    fun canAnswer(id: String): Boolean = id !in _answering.value && guard.leftMs(id) == 0L

    /**
     * Answers a question of the agent (permission.reply, M4). It has no rid and no reply: the card goes when the
     * host's permission.resolved arrives, and until then (or a lost connection) it is answering and takes no other
     * answer: one id, one reply. false when nothing was sent: a decision that is not the protocol's, a card already
     * answered, not the one on screen or just promoted, or the host unreachable (then it can be answered again).
     */
    fun answerPermission(id: String, decision: String): Boolean {
        if (!PermissionUi.isDecision(decision) || !canAnswer(id)) return false
        var first = false
        _answering.update { a -> first = id !in a; a + id }
        if (!first) return false
        val payload = Protocol.json.encodeToJsonElement(PermissionReply.serializer(), PermissionUi.reply(id, decision))
        val ok = channel.send(Protocol.PERMISSION_REPLY, payload)
        if (!ok) _answering.update { it - id }
        return ok
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
                Protocol.payload<ErrorReply>(m)?.let { _errors.tryEmit(errorText(it)) }
            }
            else -> discard(id) // the host's own chat.message is already in the state
        }
    }

    private fun set(id: String, st: SendStatus) = _outgoing.update { l -> l.map { if (it.id == id) it.copy(status = st) else it } }
}
