package tech.hybriq.adbuddy.bridge

import java.net.Proxy
import java.util.concurrent.CompletableFuture
import java.util.concurrent.CountDownLatch
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.serialization.json.JsonObject
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import okio.ByteString.Companion.toByteString
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** The client against a fake host speaking the protocol. */
class BridgeTest {
    private val server = MockWebServer()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val token = "0123456789abcdef".repeat(4)
    private val tokenB = "fedcba9876543210".repeat(4)

    /** Short waits so the tests run fast; production uses Timing(). */
    private val fast = Timing(first = 100, reconnect = 100, max = 5_000)

    /** The host's side of each connection, in the order the app opens them. */
    private val hosts = List(4) { Host() }

    /** One connection as the host sees it. */
    private class Host : WebSocketListener() {
        val opened = CountDownLatch(1)
        val gone = CountDownLatch(1)
        val received = LinkedBlockingQueue<String>()
        val frames = LinkedBlockingQueue<ByteString>()
        @Volatile var ws: WebSocket? = null

        override fun onOpen(webSocket: WebSocket, response: Response) { ws = webSocket; opened.countDown() }
        override fun onMessage(webSocket: WebSocket, text: String) { received.add(text) }
        override fun onMessage(webSocket: WebSocket, bytes: ByteString) { frames.add(bytes) }
        override fun onClosing(webSocket: WebSocket, code: Int, reason: String) { webSocket.close(1000, null) }
        override fun onClosed(webSocket: WebSocket, code: Int, reason: String) { gone.countDown() }
        override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) { gone.countDown() }

        /** The next message from the app. */
        fun next(): Message {
            val text = received.poll(5, TimeUnit.SECONDS) ?: throw AssertionError("nothing from the app")
            return Protocol.decode(text)!!
        }

        fun send(text: String) = assertTrue("host send", ws!!.send(text))

        fun welcome(session: String = "s_1", protocol: Int = 1) =
            send("""{"v":1,"type":"welcome","ts":"x","payload":{"session":"$session","fromSeq":1,"protocol":$protocol}}""")

        fun isGone(): Boolean = gone.await(5, TimeUnit.SECONDS)
    }

    @Before fun setUp() {
        for (h in hosts) server.enqueue(MockResponse.Builder().webSocketUpgrade(h).build())
        server.start()
    }

    @After fun tearDown() {
        scope.cancel()
        server.close()
    }

    private fun link(t: String = token) = BridgePrefs.Link(t, server.port, "PC")

    private fun waitFor(what: String, cond: () -> Boolean) {
        val deadline = System.currentTimeMillis() + 5000
        while (!cond() && System.currentTimeMillis() < deadline) Thread.sleep(10)
        assertTrue(what, cond())
    }

    /** Reads the hello on h, welcomes the app and waits until it is connected. */
    private fun connect(bridge: Bridge, h: Host): Hello {
        val hello = h.next()
        assertEquals(Protocol.HELLO, hello.type)
        h.welcome()
        waitFor("connected") { bridge.state.value.connection == BridgeState.Connection.CONNECTED }
        return Protocol.payload<Hello>(hello)!!
    }

    @Test fun helloQuestionAndReply() {
        val bridge = Bridge(scope, "0.2.0") { _, _ -> }
        bridge.start(link())

        val hello = hosts[0].next()
        assertEquals(Protocol.HELLO, hello.type)
        assertEquals(token, Protocol.payload<Hello>(hello)!!.token)
        assertEquals(Protocol.VERSION, Protocol.payload<Hello>(hello)!!.protocol)

        hosts[0].welcome()
        hosts[0].send("""{"v":1,"seq":1,"type":"confirm.request","ts":"x","payload":{"id":"j_1","title":"Restart","rawCommand":"adb shell reboot","expiresAt":"2026-09-30T01:04:03Z"}}""")
        waitFor("the question arrives") { bridge.state.value.pending.isNotEmpty() }
        assertEquals(1, bridge.state.value.pending.size)

        assertTrue(bridge.reply("j_1", Decision.DENY))
        val reply = hosts[0].next()
        assertEquals(Protocol.CONFIRM_REPLY, reply.type)
        val r = Protocol.payload<ConfirmReply>(reply)
        assertNotNull(r)
        assertEquals("deny", r!!.decision)
        // Answered but not gone: it leaves with its final journal entry.
        assertEquals(listOf("j_1"), bridge.state.value.pending.map { it.id })
        assertEquals(setOf("j_1"), bridge.state.value.answering)

        hosts[0].send("""{"v":1,"seq":2,"type":"journal.entry","ts":"x","payload":{"id":"j_1","status":"denied","level":"red"}}""")
        waitFor("the final entry removes it") { bridge.state.value.pending.isEmpty() }
        assertTrue(bridge.state.value.answering.isEmpty())
        bridge.stop()
    }

    @Test fun aHostThatSaysUpdateStopsTheRetries() {
        val bridge = Bridge(scope, "0.1.0", fast) { _, _ -> }
        bridge.start(link())
        hosts[0].next() // hello
        hosts[0].ws!!.close(Protocol.CLOSE_INCOMPATIBLE, "update the ADBuddy app")
        waitFor("incompatible") { bridge.state.value.connection == BridgeState.Connection.INCOMPATIBLE }
        assertEquals(BridgeState.Outdated.APP, bridge.state.value.outdated)
        Thread.sleep(1000) // ten times the first wait
        assertEquals("no reconnection after an incompatible close", 1, server.requestCount)
        bridge.stop()
    }

    @Test fun anOldHostsWelcomeStopsTheRetries() {
        val bridge = Bridge(scope, "0.2.0", fast) { _, _ -> }
        bridge.start(link())
        hosts[0].next() // hello
        hosts[0].welcome(protocol = 0)
        waitFor("incompatible") { bridge.state.value.connection == BridgeState.Connection.INCOMPATIBLE }
        assertEquals(BridgeState.Outdated.HOST, bridge.state.value.outdated)
        assertTrue("the app closes the old host's socket", hosts[0].isGone())
        Thread.sleep(1000)
        assertEquals("no reconnection to an old host", 1, server.requestCount)
        bridge.stop()
    }

    @Test fun reconnectsWithTheSessionAndTheLastSeq() {
        val bridge = Bridge(scope, "0.2.0", fast) { _, _ -> }
        bridge.start(link())
        connect(bridge, hosts[0])
        hosts[0].send("""{"v":1,"seq":1,"type":"session.state","ts":"x","payload":{"state":"working"}}""")
        hosts[0].send("""{"v":1,"seq":2,"type":"session.state","ts":"x","payload":{"state":"idle"}}""")
        waitFor("seq 2 applied") { bridge.state.value.lastSeq == 2L }

        hosts[0].ws!!.close(1000, "restart")
        val hello = Protocol.payload<Hello>(hosts[1].next())!!
        assertEquals(token, hello.token)
        assertEquals("s_1", hello.session)
        assertEquals(2L, hello.lastSeq)
        assertEquals(2, server.requestCount)
        bridge.stop()
    }

    @Test fun aNewLinkWhileWaitingConnectsAtOnceWithIt() {
        // Waits long enough that only the new link can explain a quick second connection.
        val bridge = Bridge(scope, "0.2.0", Timing(first = 10_000, reconnect = 10_000, max = 10_000)) { _, _ -> }
        bridge.start(link())
        connect(bridge, hosts[0])
        hosts[0].ws!!.close(1000, "restart")
        waitFor("waiting") { bridge.state.value.connection == BridgeState.Connection.WAITING }
        assertTrue(hosts[0].isGone())

        bridge.start(link(tokenB))
        assertEquals(tokenB, Protocol.payload<Hello>(hosts[1].next())!!.token)
        Thread.sleep(500)
        assertEquals(2, server.requestCount)
        bridge.stop()
    }

    @Test fun aNewLinkWhileConnectedLeavesNoOldSocket() {
        val bridge = Bridge(scope, "0.2.0", fast) { _, _ -> }
        bridge.start(link())
        connect(bridge, hosts[0])

        bridge.start(link(tokenB))
        assertTrue("the old socket is closed", hosts[0].isGone())
        assertEquals(tokenB, Protocol.payload<Hello>(hosts[1].next())!!.token)
        Thread.sleep(500)
        assertEquals(2, server.requestCount)
        assertEquals("no third socket", 1L, hosts[2].opened.count)
        bridge.stop()
    }

    @Test fun startingTwiceWithTheSameLinkOpensOneConnection() {
        val bridge = Bridge(scope, "0.2.0", fast) { _, _ -> }
        bridge.start(link())
        bridge.start(link())
        connect(bridge, hosts[0])
        Thread.sleep(500)
        assertEquals(1, server.requestCount)
        assertEquals("the first socket is still open", 1L, hosts[0].gone.count)
        bridge.stop()
    }

    @Test fun stopEndsTheConnectionAndTheRetries() {
        val bridge = Bridge(scope, "0.2.0", fast) { _, _ -> }
        bridge.start(link())
        connect(bridge, hosts[0])
        bridge.stop()
        assertTrue("the socket is closed", hosts[0].isGone())
        Thread.sleep(1000) // ten times the wait before a retry
        assertEquals(1, server.requestCount)
        assertEquals(BridgeState.Connection.WAITING, bridge.state.value.connection)
        assertFalse(bridge.reply("j_1", Decision.ALLOW))
    }

    @Test fun stopWhileWaitingEndsTheRetries() {
        // A one-second wait leaves room to stop well before it ends, even on a slow machine.
        val bridge = Bridge(scope, "0.2.0", Timing(first = 1_000, reconnect = 1_000, max = 1_000)) { _, _ -> }
        bridge.start(link())
        connect(bridge, hosts[0])
        hosts[0].ws!!.close(1000, "restart")
        waitFor("waiting") { bridge.state.value.connection == BridgeState.Connection.WAITING }
        bridge.stop()
        Thread.sleep(2_000) // twice the wait: a retry would have come by now
        assertEquals(1, server.requestCount)
    }

    @Test fun aRejectedTokenWaitsTheLongestUntilANewLink() {
        val bridge = Bridge(scope, "0.2.0", fast) { _, _ -> }
        bridge.start(link())
        hosts[0].next() // hello
        hosts[0].ws!!.close(1008, "unauthorized")
        assertTrue(hosts[0].isGone())
        Thread.sleep(1000) // ten times the first wait, a fifth of the longest
        assertEquals("no quick retry with a rejected token", 1, server.requestCount)

        bridge.start(link(tokenB))
        assertEquals(tokenB, Protocol.payload<Hello>(hosts[1].next())!!.token)
        bridge.stop()
    }

    @Test fun pauseAndResume() {
        val bridge = Bridge(scope, "0.2.0", fast) { _, _ -> }
        bridge.start(link())
        connect(bridge, hosts[0])
        assertTrue(bridge.pause(true))
        assertEquals(Protocol.PAUSE, hosts[0].next().type)
        assertTrue(bridge.pause(false))
        assertEquals(Protocol.RESUME, hosts[0].next().type)
        bridge.stop()
    }

    @Test fun nothingGoesBeforeTheHelloOrBeforeTheWelcome() {
        // The host reads the hello first and closes with 1008 on anything else,
        // so reply and pause must refuse until the host has welcomed the app.
        // The upgrade answers late, so the socket exists before it opens.
        val slow = MockWebServer()
        val h = Host()
        slow.enqueue(MockResponse.Builder().headersDelay(500, TimeUnit.MILLISECONDS).webSocketUpgrade(h).build())
        slow.start()
        val bridge = Bridge(scope, "0.2.0", fast) { _, _ -> }
        try {
            bridge.start(BridgePrefs.Link(token, slow.port, "PC"))
            waitFor("connecting") { bridge.state.value.connection == BridgeState.Connection.CONNECTING }
            assertFalse("no reply before the socket opens", bridge.reply("j_1", Decision.ALLOW))
            assertFalse("no pause before the socket opens", bridge.pause(true))

            assertEquals("the hello goes first", Protocol.HELLO, h.next().type)
            // Hello sent, no welcome yet: still connecting.
            assertEquals(BridgeState.Connection.CONNECTING, bridge.state.value.connection)
            assertFalse("no reply before the welcome", bridge.reply("j_1", Decision.ALLOW))
            assertFalse("no pause before the welcome", bridge.pause(false))
            assertTrue(bridge.state.value.answering.isEmpty())

            h.welcome()
            waitFor("connected") { bridge.state.value.connection == BridgeState.Connection.CONNECTED }
            assertTrue(bridge.pause(true))
            assertEquals("nothing refused was sent", Protocol.PAUSE, h.next().type)
        } finally {
            bridge.stop()
            slow.close()
        }
    }

    @Test fun withoutAConnectionNothingIsSent() {
        val bridge = Bridge(scope, "0.2.0") { _, _ -> }
        assertFalse(bridge.reply("j_1", Decision.ALLOW))
        assertFalse(bridge.pause(true))
        assertTrue(bridge.state.value.answering.isEmpty())
    }

    @Test fun theClientUsesNoProxy() {
        // A proxy would receive the hello, token included.
        assertEquals(Proxy.NO_PROXY, Bridge(scope, "0.2.0") { _, _ -> }.client.proxy)
    }

    @Test fun theLinkHidesItsToken() {
        val nonce = "fedcba9876543210".repeat(2)
        val s = BridgePrefs.Link(token, 1234, "PC", nonce).toString()
        assertFalse(s, s.contains(token))
        assertFalse(s, s.contains(nonce))
        assertTrue(s, s.contains("token=<redacted>"))
        assertTrue(s, s.contains("port=1234"))
    }

    @Test fun backoffDoublesFromHalfASecondToThirty() {
        val b = Backoff(Timing())
        assertEquals(listOf(500L, 1_000L, 2_000L, 4_000L, 8_000L, 16_000L, 30_000L, 30_000L), List(8) { b.after(welcomed = false, rejected = false) })
        // Back to one second after a connection, and doubling from there.
        assertEquals(1_000L, b.after(welcomed = true, rejected = false))
        assertEquals(2_000L, b.after(welcomed = false, rejected = false))
    }

    @Test fun backoffAfterARejectedTokenStaysAtTheLongest() {
        val b = Backoff(Timing())
        assertEquals(30_000L, b.after(welcomed = false, rejected = true))
        assertEquals(30_000L, b.after(welcomed = false, rejected = false))
        assertEquals(30_000L, b.after(welcomed = true, rejected = true))
        // Only a welcome (the host took the token) brings the short waits back.
        assertEquals(1_000L, b.after(welcomed = true, rejected = false))
    }

    /**
     * A request carries a new rid; its first answer reaches onResult, and every
     * listener of the answer's type gets it too (protocol spec §3).
     */
    @Test fun requestsAndTheirReplies() {
        val bridge = Bridge(scope, "0.3.0", fast) { _, _ -> }
        val errors = LinkedBlockingQueue<Message>()
        val others = LinkedBlockingQueue<Message>()
        bridge.listen(Protocol.ERROR) { errors.add(it) }
        val remove = bridge.listen(Protocol.ERROR) { others.add(it) }
        assertEquals("not welcomed yet", null, bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { })
        bridge.start(link())
        connect(bridge, hosts[0])

        val result = CompletableFuture<Message?>()
        val rid = bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { result.complete(it) }
        assertNotNull(rid)
        assertTrue("a valid rid", Protocol.validRid(rid!!))
        val sent = hosts[0].next()
        assertEquals(Protocol.CAPTURE_REQUEST, sent.type)
        assertEquals(rid, sent.rid)
        val second = bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { }
        assertTrue("rids differ", second != null && second != rid)
        assertEquals(second, hosts[0].next().rid)

        hosts[0].send("""{"v":1,"rid":"$rid","type":"error","ts":"x","payload":{"code":"unsupported","text":"no"}}""")
        val got = result.get(5, TimeUnit.SECONDS)!!
        assertEquals(ErrorCode.UNSUPPORTED, Protocol.payload<ErrorReply>(got)!!.code)
        assertEquals(rid, errors.poll(5, TimeUnit.SECONDS)!!.rid)
        assertEquals(rid, others.poll(5, TimeUnit.SECONDS)!!.rid)
        assertEquals("a reply does not move lastSeq", 0L, bridge.state.value.lastSeq)

        remove()
        hosts[0].send("""{"v":1,"type":"error","ts":"x","payload":{"code":"failed","text":"x"}}""")
        assertNotNull(errors.poll(5, TimeUnit.SECONDS))
        assertEquals("a removed listener hears nothing", null, others.poll(200, TimeUnit.MILLISECONDS))

        assertTrue(bridge.send(Protocol.TERMINAL_UNWATCH, JsonObject(emptyMap())))
        val plain = hosts[0].next()
        assertEquals(Protocol.TERMINAL_UNWATCH, plain.type)
        assertEquals(null, plain.rid)
        bridge.stop()
    }

    /** A request still waiting when its connection ends fails with null. */
    @Test fun aDroppedConnectionFailsItsRequests() {
        val bridge = Bridge(scope, "0.3.0", fast) { _, _ -> }
        bridge.start(link())
        connect(bridge, hosts[0])
        val result = CompletableFuture<Message?>()
        assertNotNull(bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { result.complete(it) })
        hosts[0].next()
        hosts[0].ws!!.close(1000, "restart")
        assertEquals(null, result.get(5, TimeUnit.SECONDS))
        bridge.stop()
    }

    /** Binary frames both ways, through Frame; malformed ones never reach the listener. */
    @Test fun binaryFrames() {
        val bridge = Bridge(scope, "0.3.0", fast) { _, _ -> }
        val frames = LinkedBlockingQueue<Chunk>()
        bridge.listenFrames { frames.add(it) }
        bridge.start(link())
        connect(bridge, hosts[0])
        val id = "00112233445566778899aabbccddeeff"

        assertTrue(bridge.sendFrame(id, 7, "hello".toByteArray()))
        assertEquals(Frame.encode(id, 7, "hello".toByteArray())!!.toByteString(), hosts[0].frames.poll(5, TimeUnit.SECONDS))
        assertFalse("a bad id is not sent", bridge.sendFrame("XYZ", 0, "x".toByteArray()))
        assertFalse("an empty frame is not sent", bridge.sendFrame(id, 0, ByteArray(0)))
        assertTrue(bridge.queuedBytes() >= 0)

        assertTrue(hosts[0].ws!!.send(ByteArray(4).toByteString())) // shorter than an id
        assertTrue(hosts[0].ws!!.send(Frame.encode(id, 3, "back".toByteArray())!!.toByteString()))
        val chunk = frames.poll(5, TimeUnit.SECONDS)!!
        assertEquals(id, chunk.id)
        assertEquals(3L, chunk.offset)
        assertEquals("back", String(chunk.data))
        assertEquals(null, frames.poll(200, TimeUnit.MILLISECONDS))
        bridge.stop()
    }
}
