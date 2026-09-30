package tech.hybriq.adbuddy.bridge

import java.time.Instant
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonObject
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener

/**
 * The app end of the channel (spec §5.2): a WebSocket client to
 * ws://127.0.0.1:<port>/v1, which `adb reverse` carries to the host. One per
 * process; BridgeService keeps the process alive while it runs.
 */
class Bridge(
    private val scope: CoroutineScope,
    private val appVersion: String,
    private val onMessage: (Message, BridgeState) -> Unit,
) {
    private val client = OkHttpClient.Builder()
        .pingInterval(20, TimeUnit.SECONDS)
        .readTimeout(0, TimeUnit.MILLISECONDS)
        .build()

    private val _state = MutableStateFlow(BridgeState())
    val state: StateFlow<BridgeState> = _state.asStateFlow()

    private var socket: WebSocket? = null
    private var loop: Job? = null
    @Volatile private var link: BridgePrefs.Link? = null

    /** Connects with link, and keeps reconnecting until stop(). */
    fun start(link: BridgePrefs.Link) {
        this.link = link
        _state.update { it.copy(hostName = link.host) }
        loop?.cancel()
        socket?.close(1000, "new link")
        loop = scope.launch {
            var backoff = 500L
            while (true) {
                val closed = CompletableDeferred<Unit>()
                connect(link, closed)
                closed.await()
                if (_state.value.connection == BridgeState.Connection.INCOMPATIBLE) break // retrying cannot help
                val wasConnected = _state.value.connection == BridgeState.Connection.CONNECTED
                _state.update { it.copy(connection = BridgeState.Connection.WAITING) }
                // Back off while the host is not there (serve not running, PC asleep).
                backoff = if (wasConnected) 1_000L else (backoff * 2).coerceAtMost(30_000L)
                delay(backoff)
            }
        }
    }

    fun stop() {
        loop?.cancel()
        socket?.close(1000, "bye")
        socket = null
        _state.update { it.copy(connection = BridgeState.Connection.WAITING) }
    }

    private fun connect(link: BridgePrefs.Link, closed: CompletableDeferred<Unit>) {
        _state.update { it.copy(connection = BridgeState.Connection.CONNECTING, outdated = BridgeState.Outdated.NONE) }
        val req = Request.Builder().url("ws://127.0.0.1:${link.port}/v1").build()
        socket = client.newWebSocket(req, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                val hello = Hello(token = link.token, lastSeq = _state.value.lastSeq, session = _state.value.session, appVersion = appVersion)
                webSocket.send(Protocol.encode(Protocol.HELLO, hello, now()))
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                val m = Protocol.decode(text) ?: return
                val next = _state.updateAndGetCompat { it.apply(m) }
                if (next.connection == BridgeState.Connection.INCOMPATIBLE) {
                    webSocket.close(1000, "host too old")
                    return
                }
                onMessage(m, next)
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                if (code == Protocol.CLOSE_INCOMPATIBLE) _state.update { it.appOutdated() }
                webSocket.close(1000, null)
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                if (code == Protocol.CLOSE_INCOMPATIBLE) _state.update { it.appOutdated() }
                closed.complete(Unit)
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                closed.complete(Unit)
            }
        })
    }

    /** Sends the answer to a 🔴 question. Returns false when the host is not reachable. */
    fun reply(id: String, decision: String): Boolean {
        val ok = socket?.send(Protocol.encode(Protocol.CONFIRM_REPLY, ConfirmReply(id, decision), now())) ?: false
        if (ok) _state.update { it.answered(id) }
        return ok
    }

    fun pause(paused: Boolean): Boolean =
        socket?.send(Protocol.encodeElement(if (paused) Protocol.PAUSE else Protocol.RESUME, JsonObject(emptyMap()), now())) ?: false

    private fun now() = Instant.now().toString()

    private inline fun MutableStateFlow<BridgeState>.updateAndGetCompat(f: (BridgeState) -> BridgeState): BridgeState {
        while (true) {
            val prev = value
            val next = f(prev)
            if (compareAndSet(prev, next)) return next
        }
    }
}
