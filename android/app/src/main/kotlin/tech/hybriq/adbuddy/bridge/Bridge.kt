package tech.hybriq.adbuddy.bridge

import java.net.Proxy
import java.time.Instant
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.delay
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.flow.updateAndGet
import kotlinx.coroutines.job
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
 *
 * start() and stop() may come from any thread (the UI, the provider, the
 * service). Only one socket is ever current: each connection attempt closes
 * its own socket when it ends, and whatever an older socket still delivers is
 * ignored.
 */
class Bridge internal constructor(
    private val scope: CoroutineScope,
    private val appVersion: String,
    private val timing: Timing,
    private val onMessage: (Message, BridgeState) -> Unit,
) {
    constructor(scope: CoroutineScope, appVersion: String, onMessage: (Message, BridgeState) -> Unit) :
        this(scope, appVersion, Timing(), onMessage)

    // No proxy, ever: the channel only goes to 127.0.0.1 (adb reverse), and a
    // proxy set on the phone or by a VPN app would receive the hello, token included.
    internal val client: OkHttpClient = OkHttpClient.Builder()
        .proxy(Proxy.NO_PROXY)
        .pingInterval(20, TimeUnit.SECONDS)
        .readTimeout(0, TimeUnit.MILLISECONDS)
        .build()

    private val _state = MutableStateFlow(BridgeState())
    val state: StateFlow<BridgeState> = _state.asStateFlow()

    /** Guards socket, loop and link, and every state change that depends on which socket is current. */
    private val lock = Any()
    @Volatile private var socket: WebSocket? = null
    @Volatile private var loop: Job? = null
    @Volatile private var link: BridgePrefs.Link? = null

    /** Connects with link, and keeps reconnecting until stop(). The same link again changes nothing. */
    fun start(link: BridgePrefs.Link) {
        synchronized(lock) {
            if (link == this.link && loop?.isActive == true) return
            this.link = link
            loop?.cancel()
            socket?.close(1000, "new link")
            socket = null
            _state.update { it.copy(hostName = link.host) }
            loop = scope.launch { run(link) }
        }
    }

    fun stop() {
        synchronized(lock) {
            link = null
            loop?.cancel()
            loop = null
            socket?.close(1000, "bye")
            socket = null
            _state.update { it.disconnected() }
        }
    }

    private suspend fun run(link: BridgePrefs.Link) {
        val job = currentCoroutineContext().job
        val backoff = Backoff(timing)
        while (true) {
            val attempt = connect(link, job)
            synchronized(lock) {
                job.ensureActive()
                if (_state.value.connection == BridgeState.Connection.INCOMPATIBLE) return // retrying cannot help
                _state.update { it.disconnected() }
            }
            // Back off while the host is not there (serve not running, PC asleep) or refuses the token.
            delay(backoff.after(attempt.welcomed, attempt.code == CLOSE_REJECTED))
        }
    }

    /** How one connection went. */
    private class Attempt {
        val closed = CompletableDeferred<Unit>()
        @Volatile var welcomed = false
        @Volatile var code = 0
    }

    /** One connection, from opening to closing; its socket is always gone when this returns. */
    private suspend fun connect(link: BridgePrefs.Link, job: Job): Attempt {
        val attempt = Attempt()
        val req = Request.Builder().url("ws://127.0.0.1:${link.port}/v1").build()
        var ws: WebSocket? = null
        try {
            synchronized(lock) {
                // A start() or stop() may have replaced this loop meanwhile: then it opens nothing.
                job.ensureActive()
                _state.update { it.copy(connection = BridgeState.Connection.CONNECTING, outdated = BridgeState.Outdated.NONE) }
                ws = client.newWebSocket(req, Listener(link, attempt))
                socket = ws
            }
            attempt.closed.await()
        } finally {
            synchronized(lock) { if (socket === ws) socket = null }
            ws?.cancel()
        }
        return attempt
    }

    /** Callbacks of one socket. Only the current socket changes the state; any socket completes its own attempt. */
    private inner class Listener(private val link: BridgePrefs.Link, private val attempt: Attempt) : WebSocketListener() {
        override fun onOpen(webSocket: WebSocket, response: Response) {
            synchronized(lock) {
                if (webSocket !== socket) return
                val s = _state.value
                val hello = Hello(token = link.token, lastSeq = s.lastSeq, session = s.session, appVersion = appVersion)
                webSocket.send(Protocol.encode(Protocol.HELLO, hello, now()))
            }
        }

        override fun onMessage(webSocket: WebSocket, text: String) {
            val m = Protocol.decode(text) ?: return
            val next = synchronized(lock) {
                if (webSocket !== socket) return
                _state.updateAndGet { it.apply(m) }
            }
            if (next.connection == BridgeState.Connection.INCOMPATIBLE) {
                webSocket.close(1000, "host too old")
                return
            }
            if (m.type == Protocol.WELCOME) attempt.welcomed = true
            onMessage(m, next)
        }

        override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
            attempt.code = code
            outdatedIf(webSocket, code)
            webSocket.close(1000, null)
        }

        override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
            if (attempt.code == 0) attempt.code = code
            outdatedIf(webSocket, code)
            attempt.closed.complete(Unit)
        }

        override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
            attempt.closed.complete(Unit)
        }

        private fun outdatedIf(webSocket: WebSocket, code: Int) {
            if (code != Protocol.CLOSE_INCOMPATIBLE) return
            synchronized(lock) {
                if (webSocket === socket) _state.update { it.appOutdated() }
            }
        }
    }

    /**
     * Sends the answer to a 🔴 question. Returns false when the host is not reachable.
     * The question stays in pending, marked as answering, until its final journal entry.
     */
    fun reply(id: String, decision: String): Boolean = synchronized(lock) {
        val ok = socket?.send(Protocol.encode(Protocol.CONFIRM_REPLY, ConfirmReply(id, decision), now())) ?: false
        if (ok) _state.update { it.answered(id) }
        ok
    }

    /**
     * Asks the host to pause (true) or resume (false) the agent. true only means
     * the request was queued on the socket, not that the host took it: the UI
     * shows what the host says in session.state (paused), never this result.
     */
    fun pause(paused: Boolean): Boolean = synchronized(lock) {
        socket?.send(Protocol.encodeElement(if (paused) Protocol.PAUSE else Protocol.RESUME, JsonObject(emptyMap()), now())) ?: false
    }

    private fun now() = Instant.now().toString()

    private companion object {
        /** The host closes with 1008 (policy violation) when it refuses the token. */
        const val CLOSE_REJECTED = 1008
    }
}

/** The waits between connection attempts. Tests shorten them; production uses the defaults. */
internal data class Timing(val first: Long = 500, val reconnect: Long = 1_000, val max: Long = 30_000)

/**
 * The wait after each attempt: from `first` doubling up to `max` while the host
 * is not there (0.5, 1, 2, 4, 8, 16, 30 s), back to `reconnect` after a
 * connection, and `max` from a refused token until the host welcomes the app
 * again (a new link starts a new loop, and a new Backoff).
 */
internal class Backoff(private val t: Timing) {
    private var next = t.first
    private var rejected = false

    fun after(welcomed: Boolean, rejected: Boolean): Long {
        if (rejected) this.rejected = true else if (welcomed) this.rejected = false
        if (this.rejected) return t.max
        val wait = if (welcomed) t.reconnect else next
        next = (wait * 2).coerceAtMost(t.max)
        return wait
    }
}
