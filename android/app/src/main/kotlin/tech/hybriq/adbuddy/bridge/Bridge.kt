package tech.hybriq.adbuddy.bridge

import java.net.Proxy
import java.security.SecureRandom
import java.time.Instant
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicLong
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
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import okio.ByteString.Companion.toByteString

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

    /** Listeners by type and of binary frames (protocol spec §3 and §10): listen(), listenFrames(). */
    private val listeners = ConcurrentHashMap<String, CopyOnWriteArrayList<(Message) -> Unit>>()
    private val frameListeners = CopyOnWriteArrayList<(Chunk) -> Unit>()

    /** A request waiting for its answer, with the socket it went on. */
    private class Pending(val socket: WebSocket, val onResult: (Message?) -> Unit)

    /** Requests waiting for their answer, by rid. Guarded by lock. */
    private val pending = HashMap<String, Pending>()

    /**
     * Request ids: unique in this process and unlike those of an earlier one,
     * since the host resends events with the rid they confirm (protocol spec §3).
     */
    private val ridPrefix = "r" + Frame.idString(ByteArray(4).also { SecureRandom().nextBytes(it) }) + "-"
    private val rids = AtomicLong()

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
            ws?.let { failPending(it) }
        }
        return attempt
    }

    /** The requests sent on ws get no answer now: each one fails (onResult(null)). */
    private fun failPending(ws: WebSocket) {
        val failed = synchronized(lock) {
            val ids = pending.filterValues { it.socket === ws }.keys
            ids.mapNotNull { pending.remove(it) }
        }
        failed.forEach { p -> safely { p.onResult(null) } }
    }

    /**
     * Runs a callback of another part of the app (a listener, onResult). Its
     * exception stays there: it must not cut the socket (OkHttp's reader),
     * stop the reconnection loop (failPending runs in it) nor leave the other
     * callbacks uncalled. It is not logged: the logs of this process must
     * never risk carrying a message's content. An Error (TODO(), out of
     * memory) still ends the process.
     */
    private inline fun safely(f: () -> Unit) {
        try {
            f()
        } catch (e: Exception) {
            // A bug of that callback; the channel goes on.
        }
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
            val (next, answered) = synchronized(lock) {
                if (webSocket !== socket) return
                // Only a request sent on this socket: one of an older socket already failed or is failing.
                val waiting = m.rid?.let { rid -> pending[rid]?.takeIf { it.socket === webSocket }?.let { pending.remove(rid) } }
                _state.updateAndGet { it.apply(m) } to waiting
            }
            if (next.connection == BridgeState.Connection.INCOMPATIBLE) {
                webSocket.close(1000, "host too old")
                answered?.let { p -> safely { p.onResult(null) } }
                return
            }
            if (m.type == Protocol.WELCOME) attempt.welcomed = true
            safely { onMessage(m, next) }
            answered?.let { p -> safely { p.onResult(m) } }
            listeners[m.type]?.forEach { l -> safely { l(m) } }
        }

        override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
            if (frameListeners.isEmpty()) return
            val chunk = Frame.decode(bytes.toByteArray()) ?: return
            synchronized(lock) {
                if (webSocket !== socket || _state.value.connection != BridgeState.Connection.CONNECTED) return
            }
            frameListeners.forEach { l -> safely { l(chunk) } }
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
     * The socket to send on, or null until the host has welcomed the app. The
     * socket exists from CONNECTING on, but the host reads the hello first and
     * closes with 1008 on anything else, and OkHttp would write a frame queued
     * before onOpen ahead of the hello. CONNECTED only comes from a welcome of
     * the current socket, set under this same lock, so the hello is already out.
     * Call it holding the lock.
     */
    private fun welcomedSocket(): WebSocket? =
        if (_state.value.connection == BridgeState.Connection.CONNECTED) socket else null

    /**
     * Sends the answer to a 🔴 question. Returns false when the host is not
     * reachable or has not welcomed the app yet.
     * The question stays in pending, marked as answering, until its final journal entry.
     */
    fun reply(id: String, decision: String): Boolean = synchronized(lock) {
        val ok = welcomedSocket()?.send(Protocol.encode(Protocol.CONFIRM_REPLY, ConfirmReply(id, decision), now())) ?: false
        if (ok) _state.update { it.answered(id) }
        ok
    }

    /**
     * Asks the host to pause (true) or resume (false) the agent. false when the
     * host is not reachable or has not welcomed the app yet. true only means
     * the request was queued on the socket, not that the host took it: the UI
     * shows what the host says in session.state (paused), never this result.
     */
    fun pause(paused: Boolean): Boolean = synchronized(lock) {
        welcomedSocket()?.send(Protocol.encodeElement(if (paused) Protocol.PAUSE else Protocol.RESUME, JsonObject(emptyMap()), now())) ?: false
    }

    /**
     * Calls listener with each message of type from the current socket, after
     * the state took it: for replies and stream messages, which the state does
     * not keep (protocol spec §3), and events too, repeated ones included (the
     * state skips those; a listener must not count on that). Each milestone
     * adds its own; the returned function removes this one. It runs on
     * OkHttp's reader thread, so it must be quick.
     */
    fun listen(type: String, listener: (Message) -> Unit): () -> Unit {
        val list = listeners.computeIfAbsent(type) { CopyOnWriteArrayList() }
        list.add(listener)
        return { list.remove(listener) }
    }

    /** Calls listener with each valid binary frame from the welcomed socket. The returned function removes it. */
    fun listenFrames(listener: (Chunk) -> Unit): () -> Unit {
        frameListeners.add(listener)
        return { frameListeners.remove(listener) }
    }

    /** Sends a message without a rid. false when the host is not reachable or has not welcomed the app yet. */
    fun send(type: String, payload: JsonElement): Boolean = synchronized(lock) {
        welcomedSocket()?.send(Protocol.encodeElement(type, payload, now())) ?: false
    }

    /**
     * Sends a request with a new rid and returns it, or null (and no
     * onResult) when it could not be sent. onResult gets the first message
     * from the host with that rid (a reply, an error or the event that
     * confirms it), or null when the connection ends first (protocol spec §3).
     * It is waiting before the request leaves, so no answer comes too early.
     */
    fun request(type: String, payload: JsonElement, onResult: (Message?) -> Unit): String? {
        val rid = ridPrefix + rids.incrementAndGet()
        synchronized(lock) {
            val ws = welcomedSocket() ?: return null
            pending[rid] = Pending(ws, onResult)
            if (ws.send(Protocol.encodeElement(type, payload, now(), rid))) return rid
            pending.remove(rid)
        }
        return null
    }

    /** Sends a binary frame (protocol spec §6.4). false for a bad id, offset or size, or when not welcomed. */
    fun sendFrame(id: String, offset: Long, data: ByteArray): Boolean = synchronized(lock) {
        val frame = Frame.encode(id, offset, data) ?: return false
        welcomedSocket()?.send(frame.toByteString()) ?: false
    }

    /**
     * Bytes queued on the socket and not sent yet. A sender of frames waits
     * while it is high: OkHttp closes the socket past 16 MiB.
     */
    fun queuedBytes(): Long = synchronized(lock) { socket?.queueSize() ?: 0 }

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
