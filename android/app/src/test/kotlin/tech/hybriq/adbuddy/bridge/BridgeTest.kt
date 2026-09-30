package tech.hybriq.adbuddy.bridge

import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** The client against a fake host speaking the protocol. */
class BridgeTest {
    private val server = MockWebServer()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val fromApp = LinkedBlockingQueue<String>()
    private var hostSide: WebSocket? = null
    private val token = "0123456789abcdef".repeat(4)

    @Before fun setUp() {
        server.enqueue(MockResponse.Builder().webSocketUpgrade(object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) { hostSide = webSocket }
            override fun onMessage(webSocket: WebSocket, text: String) { fromApp.add(text) }
            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) { webSocket.close(1000, null) }
        }).build())
        server.start()
    }

    @After fun tearDown() {
        scope.cancel()
        server.close()
    }

    @Test fun helloQuestionAndReply() {
        val bridge = Bridge(scope, "0.2.0") { _, _ -> }
        bridge.start(BridgePrefs.Link(token, server.port, "PC"))

        val hello = Protocol.decode(fromApp.poll(5, TimeUnit.SECONDS)!!)!!
        assertEquals(Protocol.HELLO, hello.type)
        assertEquals(token, Protocol.payload<Hello>(hello)!!.token)
        assertEquals(Protocol.VERSION, Protocol.payload<Hello>(hello)!!.protocol)

        hostSide!!.send("""{"v":1,"type":"welcome","ts":"x","payload":{"session":"s_1","fromSeq":1,"protocol":1}}""")
        hostSide!!.send("""{"v":1,"seq":1,"type":"confirm.request","ts":"x","payload":{"id":"j_1","title":"Restart","rawCommand":"adb shell reboot","expiresAt":"2026-09-30T01:04:03Z"}}""")
        val deadline = System.currentTimeMillis() + 5000
        while (bridge.state.value.pending.isEmpty() && System.currentTimeMillis() < deadline) Thread.sleep(10)
        assertEquals(1, bridge.state.value.pending.size)

        assertTrue(bridge.reply("j_1", Decision.DENY))
        val reply = Protocol.decode(fromApp.poll(5, TimeUnit.SECONDS)!!)!!
        assertEquals(Protocol.CONFIRM_REPLY, reply.type)
        val r = Protocol.payload<ConfirmReply>(reply)
        assertNotNull(r)
        assertEquals("deny", r!!.decision)
        assertTrue(bridge.state.value.pending.isEmpty())
        bridge.stop()
    }

    @Test fun aHostThatSaysUpdateStopsTheRetries() {
        val bridge = Bridge(scope, "0.1.0") { _, _ -> }
        bridge.start(BridgePrefs.Link(token, server.port, "PC"))
        fromApp.poll(5, TimeUnit.SECONDS)!! // hello
        hostSide!!.close(Protocol.CLOSE_INCOMPATIBLE, "update the ADBuddy app")
        val deadline = System.currentTimeMillis() + 5000
        while (bridge.state.value.connection != BridgeState.Connection.INCOMPATIBLE && System.currentTimeMillis() < deadline) Thread.sleep(10)
        assertEquals(BridgeState.Connection.INCOMPATIBLE, bridge.state.value.connection)
        assertEquals(BridgeState.Outdated.APP, bridge.state.value.outdated)
        Thread.sleep(1500) // longer than the first retry
        assertEquals("no reconnection after an incompatible close", 1, server.requestCount)
        bridge.stop()
    }
}
