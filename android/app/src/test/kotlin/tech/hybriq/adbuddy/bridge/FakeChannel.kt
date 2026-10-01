package tech.hybriq.adbuddy.bridge

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.serialization.json.JsonElement

/** A Channel for tests: records what the app sends, and `handler` plays the host. */
class FakeChannel : Channel {
    class Sent(val type: String, val payload: JsonElement, val rid: String?)

    val sent = mutableListOf<Sent>()
    val frames = mutableListOf<Triple<String, Long, ByteArray>>()

    /** Plays the host: gets each request and its callback; false means it could not be sent (request returns null). */
    var handler: (Sent, (Message?) -> Unit) -> Boolean = { _, _ -> true }

    /** Frames refused (sendFrame returns false) from this count on; -1: never. */
    var refuseFramesFrom = -1
    var queued = 0L

    /** Runs after each frame is accepted. */
    var onFrame: () -> Unit = {}
    private var next = 0
    private val _permissions = MutableStateFlow<List<tech.hybriq.adbuddy.bridge.PermissionRequest>>(emptyList())
    override val permissions: StateFlow<List<tech.hybriq.adbuddy.bridge.PermissionRequest>> = _permissions
    fun setPermissions(l: List<tech.hybriq.adbuddy.bridge.PermissionRequest>) { _permissions.value = l }
    private val _connected = MutableStateFlow(true)
    override val connected: StateFlow<Boolean> = _connected
    private val listeners = mutableMapOf<String, MutableList<(Message) -> Unit>>()
    private val frameListeners = mutableListOf<(Chunk) -> Unit>()

    fun setConnected(v: Boolean) { _connected.value = v }

    override fun request(type: String, payload: JsonElement, onResult: (Message?) -> Unit): String? {
        val s = Sent(type, payload, "r${++next}")
        sent += s
        return if (handler(s, onResult)) s.rid else null
    }

    override fun send(type: String, payload: JsonElement): Boolean { sent += Sent(type, payload, null); return true }

    override fun sendFrame(id: String, offset: Long, data: ByteArray): Boolean {
        if (refuseFramesFrom >= 0 && frames.size >= refuseFramesFrom) return false
        frames += Triple(id, offset, data)
        onFrame()
        return true
    }

    override fun queuedBytes() = queued

    override fun listen(type: String, listener: (Message) -> Unit): () -> Unit {
        listeners.getOrPut(type) { mutableListOf() } += listener
        return { listeners[type]?.remove(listener) }
    }

    override fun listenFrames(listener: (Chunk) -> Unit): () -> Unit {
        frameListeners += listener
        return { frameListeners.remove(listener) }
    }

    /** A message of the host (a stream message: seq 0). */
    fun emit(type: String, payload: String) {
        val m = reply(type, payload)
        listeners[type]?.toList()?.forEach { it(m) }
    }

    fun emitFrame(c: Chunk) { frameListeners.toList().forEach { it(c) } }

    companion object {
        fun reply(type: String, payload: String, rid: String? = null): Message {
            val r = if (rid == null) "" else ""","rid":"$rid""""
            return Protocol.decode("""{"v":1,"type":"$type","ts":"x"$r,"payload":$payload}""")!!
        }
    }
}
