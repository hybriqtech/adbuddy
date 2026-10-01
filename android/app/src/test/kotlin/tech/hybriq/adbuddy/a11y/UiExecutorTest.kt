package tech.hybriq.adbuddy.a11y

import java.time.Instant
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.ErrorCode
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.UiOp
import tech.hybriq.adbuddy.bridge.UiRequest
import tech.hybriq.adbuddy.bridge.UiResponse

class UiExecutorTest {
    private class FakeScreen(
        var windows: List<UiWindow> = ScreenFixtures.windows,
        var fg: Pair<String, String> = "com.android.settings" to "com.android.settings.Settings",
        var locked: Boolean = false,
        var focused: UiNode? = null,
        var clickWorks: Boolean = true,
    ) : Screen {
        val did = mutableListOf<String>()
        override fun windows() = windows
        override fun screen() = ScreenFixtures.SCREEN
        override fun rotation() = 0
        override fun locked() = locked
        override fun foreground() = fg
        override fun click(node: UiNode): Boolean = clickWorks.also { did += "click ${node.bounds}" }
        override fun tap(x: Int, y: Int): Boolean = true.also { did += "tap $x,$y" }
        override fun focusedInput() = focused
        override fun setText(node: UiNode, text: String): Boolean = true.also { did += "setText $text" }
        override fun acting(mark: Box?) {
            did += "acting $mark"
        }
    }

    private val now = Instant.parse("2026-10-01T10:00:00Z").toEpochMilli()
    private var screen: FakeScreen? = FakeScreen()
    private val sent = mutableListOf<UiResponse>()
    private var question = false
    private var paused = false
    private val protected = mutableSetOf("com.example.bank")
    private val x = UiExecutor({ screen }, { sent += it }, { now }, { it in protected }, { question }, { paused }, { it.run() })

    private fun req(op: String, args: JsonElement = JsonObject(emptyMap()), id: String = "u_1", expires: String = "2026-10-01T10:00:10Z") =
        UiRequest(id, op, args, expires)

    private fun obj(vararg kv: Pair<String, Any>) = JsonObject(kv.associate { (k, v) -> k to if (v is Int) JsonPrimitive(v) else JsonPrimitive(v.toString()) })

    private fun error(r: UiResponse): String = r.error?.code ?: "ok"

    @Test fun dump() {
        val r = x.run(req(UiOp.DUMP))
        assertTrue(r.ok)
        assertEquals("com.android.settings", r.result!!.jsonObject["package"]!!.jsonPrimitive.content)
    }

    @Test fun clickByTextAndPoint() {
        val r = x.run(req(UiOp.CLICK, obj("text" to "Network & internet")))
        assertTrue(r.ok)
        assertEquals("action", r.result!!.jsonObject["how"]!!.jsonPrimitive.content)
        assertTrue(screen!!.did.contains("click Box(l=0, t=600, r=1344, b=690)"))
        val p = x.run(req(UiOp.CLICK, obj("x" to 540, "y" to 1200)))
        assertTrue(p.ok)
        assertTrue(screen!!.did.contains("tap 540,1200"))
    }

    @Test fun aClickThatDoesNotTakeTapsTheMiddle() {
        screen!!.clickWorks = false
        val r = x.run(req(UiOp.CLICK, obj("text" to "Network & internet")))
        assertEquals("gesture", r.result!!.jsonObject["how"]!!.jsonPrimitive.content)
        assertTrue(screen!!.did.contains("tap 672,645"))
    }

    @Test fun clickErrors() {
        assertEquals(ErrorCode.NOT_FOUND, error(x.run(req(UiOp.CLICK, obj("text" to "Nothing")))))
        assertEquals(ErrorCode.AMBIGUOUS, error(x.run(req(UiOp.CLICK, obj("text" to "Ok")))))
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("text" to "Allow")))))
        // A point on ADBuddy's window.
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("x" to 1000, "y" to 50)))))
        assertEquals(ErrorCode.INVALID, error(x.run(req(UiOp.CLICK, obj("text" to "a", "id" to "b")))))
        assertEquals(ErrorCode.INVALID, error(x.run(req(UiOp.CLICK, obj("x" to 1)))))
        assertEquals(ErrorCode.INVALID, error(x.run(req(UiOp.CLICK, obj("text" to "a", "extra" to 1)))))
        assertFalse(screen!!.did.any { it.startsWith("click") || it.startsWith("tap") })
    }

    @Test fun nothingOnAProtectedAppNorADBuddy() {
        for (pkg in listOf("com.example.bank", Packages.ADBUDDY)) {
            screen = FakeScreen(fg = pkg to "")
            for (op in listOf(req(UiOp.DUMP), req(UiOp.CLICK, obj("x" to 1, "y" to 1)), req(UiOp.SET_TEXT, obj("text" to "hi")))) {
                assertEquals("$pkg ${op.op}", ErrorCode.PROTECTED, error(x.run(op)))
            }
            assertTrue(screen!!.did.isEmpty())
            val fg = x.run(req(UiOp.FOREGROUND))
            assertEquals("true", fg.result!!.jsonObject["protected"]!!.jsonPrimitive.content)
        }
    }

    @Test fun theSystemUIWhileAQuestionWaits() {
        screen = FakeScreen(windows = ScreenFixtures.windows + UiWindow("system", 9, Box(0, 0, 1344, 400), ScreenFixtures.notification))
        question = true
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("text" to "Allow it")))))
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("x" to 1000, "y" to 350)))))
        question = false
        assertTrue(x.run(req(UiOp.CLICK, obj("text" to "Allow it"))).ok)
    }

    @Test fun setText() {
        val field = UiNode(pkg = "com.android.settings", cls = "android.widget.EditText", bounds = Box(0, 0, 10, 10), editable = true, focused = true)
        assertEquals(ErrorCode.NOT_FOUND, error(x.run(req(UiOp.SET_TEXT, obj("text" to "hi")))))
        screen!!.focused = field
        assertTrue(x.run(req(UiOp.SET_TEXT, obj("text" to "¿Qué tal? 👋"))).ok)
        assertTrue(screen!!.did.contains("setText ¿Qué tal? 👋"))
        screen!!.focused = field.copy(password = true)
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.SET_TEXT, obj("text" to "1234")))))
        assertEquals(ErrorCode.INVALID, error(x.run(req(UiOp.SET_TEXT, obj("text" to "")))))
        assertEquals(ErrorCode.INVALID, error(x.run(req(UiOp.SET_TEXT, obj("text" to "😀".repeat(4097))))))
    }

    @Test fun pausedNoTapsNorTyping() {
        paused = true
        screen!!.focused = UiNode(pkg = "com.android.settings", cls = "android.widget.EditText", bounds = Box(0, 0, 10, 10), editable = true, focused = true)
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("x" to 1, "y" to 1)))))
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.SET_TEXT, obj("text" to "hi")))))
        assertTrue(x.run(req(UiOp.DUMP)).ok)
        assertFalse(screen!!.did.any { it.startsWith("tap") || it.startsWith("setText") })
    }

    @Test fun aSplitScreenWithABank() {
        val bank = UiNode(pkg = "com.example.bank", cls = "android.widget.FrameLayout", bounds = Box(0, 1500, 1344, 2992),
            children = listOf(UiNode(pkg = "com.example.bank", cls = "android.widget.TextView", text = "Balance 1.000 €", bounds = Box(0, 1600, 600, 1700))))
        screen = FakeScreen(windows = listOf(UiWindow("app", 1, Box(0, 0, 1344, 1490), ScreenFixtures.settings), UiWindow("dialog", 1, Box(0, 1500, 1344, 2992), bank)))
        assertEquals("true", x.run(req(UiOp.FOREGROUND)).result!!.jsonObject["protected"]!!.jsonPrimitive.content)
        assertFalse(x.run(req(UiOp.DUMP)).result.toString().contains("Balance"))
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("text" to "Balance 1.000 €")))))
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("x" to 100, "y" to 1650)))))
    }

    @Test fun aWindowItCannotSee() {
        screen = FakeScreen(windows = ScreenFixtures.windows + UiWindow("app", 3, Box(0, 2000, 1344, 2992), null))
        assertEquals("true", x.run(req(UiOp.FOREGROUND)).result!!.jsonObject["protected"]!!.jsonPrimitive.content)
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("x" to 100, "y" to 2500)))))
    }

    @Test fun lockedPhone() {
        screen!!.locked = true
        assertTrue(x.run(req(UiOp.DUMP)).result!!.jsonObject["locked"]!!.jsonPrimitive.content == "true")
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("x" to 1, "y" to 1)))))
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.SET_TEXT, obj("text" to "1234")))))
    }

    @Test fun expiredUnavailableAndUnknown() {
        assertEquals(ErrorCode.EXPIRED, error(x.run(req(UiOp.DUMP, expires = "2026-10-01T09:59:59Z"))))
        assertEquals(ErrorCode.UNSUPPORTED, error(x.run(req("swipe"))))
        assertEquals(ErrorCode.UNAVAILABLE, error(x.run(req(UiOp.NOTIFICATIONS))))
        screen = null
        assertEquals(ErrorCode.UNAVAILABLE, error(x.run(req(UiOp.DUMP))))
    }

    @Test fun neverTwiceTheSameId() {
        fun message(id: String) = Message(v = Protocol.VERSION, type = Protocol.UI_REQUEST, ts = "2026-10-01T10:00:00Z", payload = Protocol.json.encodeToJsonElement(UiRequest.serializer(), req(UiOp.FOREGROUND, id = id)))
        x.onMessage(message("u_1"))
        x.onMessage(message("u_1"))
        x.onMessage(message("u_2"))
        assertEquals(listOf("u_1", "u_2"), sent.map { it.id })
    }
}

/** dumpsys activity service is 🟢 for the agent: the service must not print its state there. */
class AgentAccessibilityServiceTest {
    @Test fun doesNotOverrideDump() {
        assertFalse(AgentAccessibilityService::class.java.declaredMethods.any { it.name == "dump" })
    }
}
