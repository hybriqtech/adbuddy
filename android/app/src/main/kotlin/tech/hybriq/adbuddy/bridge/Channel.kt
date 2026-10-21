package tech.hybriq.adbuddy.bridge

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.serialization.json.JsonElement

/**
 * What the chat, the files, the terminal and the capture need from the
 * channel with the host. Bridge implements it through BridgeChannel; the tests
 * use FakeChannel, so none of them needs a network.
 */
interface Channel {
    /** Bridge.request: the new rid, or null when it could not be sent (and onResult is never called). */
    fun request(type: String, payload: JsonElement, onResult: (Message?) -> Unit): String?
    fun send(type: String, payload: JsonElement): Boolean
    fun sendFrame(id: String, offset: Long, data: ByteArray): Boolean
    fun queuedBytes(): Long
    fun listen(type: String, listener: (Message) -> Unit): () -> Unit
    fun listenFrames(listener: (Chunk) -> Unit): () -> Unit

    /** True while the host has welcomed the app. */
    val connected: StateFlow<Boolean>

    /** The agent's permission questions the host has pending, oldest first (ChatState.permissions). */
    val permissions: StateFlow<List<PermissionRequest>>
}

class BridgeChannel(private val bridge: Bridge, scope: CoroutineScope) : Channel {
    override fun request(type: String, payload: JsonElement, onResult: (Message?) -> Unit) = bridge.request(type, payload, onResult)
    override fun send(type: String, payload: JsonElement) = bridge.send(type, payload)
    override fun sendFrame(id: String, offset: Long, data: ByteArray) = bridge.sendFrame(id, offset, data)
    override fun queuedBytes() = bridge.queuedBytes()
    override fun listen(type: String, listener: (Message) -> Unit) = bridge.listen(type, listener)
    override fun listenFrames(listener: (Chunk) -> Unit) = bridge.listenFrames(listener)
    override val connected: StateFlow<Boolean> =
        bridge.state.map { it.isConnected }.stateIn(scope, SharingStarted.Eagerly, false)
    override val permissions: StateFlow<List<PermissionRequest>> =
        bridge.state.map { it.chat.permissions }.distinctUntilChanged().stateIn(scope, SharingStarted.Eagerly, bridge.state.value.chat.permissions)
}
