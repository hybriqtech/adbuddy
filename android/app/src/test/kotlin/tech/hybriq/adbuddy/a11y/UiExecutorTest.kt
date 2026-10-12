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
import tech.hybriq.adbuddy.bridge.UiLimits
import tech.hybriq.adbuddy.bridge.UiOp
import tech.hybriq.adbuddy.bridge.UiRequest
import tech.hybriq.adbuddy.bridge.UiResponse
import tech.hybriq.adbuddy.listener.NotificationRead
import tech.hybriq.adbuddy.listener.RawNotification

class UiExecutorTest {
    private class FakeScreen(
        var windows: List<UiWindow> = ScreenFixtures.windows,
        var fg: Pair<String, String> = "com.android.settings" to "com.android.settings.Settings",
        var locked: Boolean = false,
        var focused: UiNode? = null,
        var clickWorks: Boolean = true,
        var tapWorks: Boolean = true,
        var afterClick: (() -> Unit)? = null,
        var actingWorks: Boolean = true,
        var hideWorks: Boolean = true,
    ) : Screen {
        val did = mutableListOf<String>()
        var windowsRead = false
        override fun windows() = windows.also { windowsRead = true }
        override fun screen() = ScreenFixtures.SCREEN
        override fun rotation() = 0
        override fun locked() = locked
        override fun foreground() = fg
        override fun click(node: UiNode): Boolean = clickWorks.also { did += "click ${node.bounds}"; afterClick?.invoke() }
        override fun tap(x: Int, y: Int): Boolean = tapWorks.also { did += "tap $x,$y" }
        override fun focusedInput() = focused
        override fun setText(node: UiNode, text: String): Boolean = true.also { did += "setText $text" }
        override fun acting(mark: Box?): Boolean {
            did += "acting $mark"
            return actingWorks
        }
        override fun hideNotice(ms: Long): Boolean {
            did += "hideNotice $ms"
            return hideWorks
        }
    }

    private val now = Instant.parse("2026-10-01T10:00:00Z").toEpochMilli()
    private var screen: FakeScreen? = FakeScreen()
    private val sent = mutableListOf<UiResponse>()
    private var question = false
    private var paused = false
    private var notification = false
    private var notificationAfter = Int.MAX_VALUE // checks that pass before a heads-up appears
    private var checks = 0
    private val protected = mutableSetOf("com.example.bank")
    private var notes: List<RawNotification>? = emptyList() // null: no access
    private var userConfirmed = true
    private var notesThrow = false
    private val x = UiExecutor(
        { screen }, { sent += it }, { now }, { it in protected }, { question }, { paused }, { notification || ++checks > notificationAfter }, { it.run() },
        notifications = {
            when {
                notesThrow -> throw IllegalStateException("boom")
                !userConfirmed -> NotificationRead.NotConfirmed
                else -> notes?.let { NotificationRead.Ok(it) } ?: NotificationRead.NoAccess
            }
        },
    )

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

    @Test fun theFallbackTapChecksWhatIsOnTopNow() {
        // Clean centre: the tap happens.
        screen!!.clickWorks = false
        assertTrue(x.run(req(UiOp.CLICK, obj("text" to "Network & internet"))).ok)
        assertTrue(screen!!.did.contains("tap 672,645"))
        // The system UI covers the centre after the click failed, while a question waits.
        val s = FakeScreen(clickWorks = false)
        screen = s
        question = true
        val cover = UiWindow("system", 9, Box(0, 600, 1344, 690), ScreenFixtures.notification)
        s.afterClick = { s.windows = ScreenFixtures.windows + cover }
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("text" to "Network & internet")))))
        assertFalse(s.did.any { it.startsWith("tap") })
        // A protected app covers it.
        question = false
        val bank = UiWindow("app", 9, Box(0, 600, 1344, 690), UiNode(pkg = "com.example.bank", cls = "android.widget.FrameLayout", bounds = Box(0, 600, 1344, 690)))
        val s2 = FakeScreen(clickWorks = false)
        screen = s2
        s2.afterClick = { s2.windows = ScreenFixtures.windows + bank }
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("text" to "Network & internet")))))
        assertFalse(s2.did.any { it.startsWith("tap") })
    }

    @Test fun aTapThatDoesNotHappenFails() {
        screen!!.tapWorks = false
        assertEquals(ErrorCode.FAILED, error(x.run(req(UiOp.CLICK, obj("x" to 540, "y" to 1200)))))
        screen!!.clickWorks = false
        assertEquals(ErrorCode.FAILED, error(x.run(req(UiOp.CLICK, obj("text" to "Network & internet")))))
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

    @Test fun aRecentNotificationIsAProtectedScreen() {
        notification = true
        screen!!.focused = UiNode(pkg = "com.android.settings", cls = "android.widget.EditText", bounds = Box(0, 0, 10, 10), editable = true, focused = true)
        assertEquals("true", x.run(req(UiOp.FOREGROUND)).result!!.jsonObject["protected"]!!.jsonPrimitive.content)
        val d = x.run(req(UiOp.DUMP))
        assertEquals(ErrorCode.PROTECTED, error(d))
        assertEquals("A notification is on screen: ask the user to do this part.", d.error!!.text)
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("x" to 540, "y" to 1200)))))
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("text" to "Network & internet")))))
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.SET_TEXT, obj("text" to "hi")))))
        assertTrue(screen!!.did.isEmpty())
        notification = false
        assertEquals("false", x.run(req(UiOp.FOREGROUND)).result!!.jsonObject["protected"]!!.jsonPrimitive.content)
        assertTrue(x.run(req(UiOp.DUMP)).ok)
        assertTrue(x.run(req(UiOp.CLICK, obj("x" to 540, "y" to 1200))).ok)
        assertTrue(x.run(req(UiOp.SET_TEXT, obj("text" to "hi"))).ok)
    }

    @Test fun aNotificationThatAppearsMidRequestStopsIt() {
        val field = UiNode(pkg = "com.android.settings", cls = "android.widget.EditText", bounds = Box(0, 0, 10, 10), editable = true, focused = true)
        fun acts() = screen!!.did.filter { it.startsWith("tap") || it.startsWith("click") || it.startsWith("setText") || (it.startsWith("acting") && it != "acting null") }
        // dump: the first check passes, the one after reading the tree does not.
        notificationAfter = 1
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.DUMP))))
        // click at a point and set text: the late check just before acting.
        for (op in listOf(req(UiOp.CLICK, obj("x" to 540, "y" to 1200)), req(UiOp.SET_TEXT, obj("text" to "hi")), req(UiOp.CLICK, obj("text" to "Network & internet")))) {
            checks = 0
            screen!!.focused = field
            assertEquals(op.op, ErrorCode.PROTECTED, error(x.run(op)))
        }
        // the fallback tap after a failed ACTION_CLICK: the third check.
        screen!!.clickWorks = false
        notificationAfter = 2
        checks = 0
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("text" to "Network & internet")))))
        assertTrue(acts().toString(), acts().none { it.startsWith("tap") || it.startsWith("setText") })
        // without a notification everything runs.
        notificationAfter = Int.MAX_VALUE
        assertTrue(x.run(req(UiOp.DUMP)).ok)
    }

    @Test fun pausedNoTapsNorTyping() {
        paused = true
        screen!!.focused = UiNode(pkg = "com.android.settings", cls = "android.widget.EditText", bounds = Box(0, 0, 10, 10), editable = true, focused = true)
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.CLICK, obj("x" to 1, "y" to 1)))))
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.SET_TEXT, obj("text" to "hi")))))
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.DUMP))))
        assertFalse(screen!!.did.any { it.startsWith("tap") || it.startsWith("setText") || (it.startsWith("acting") && it != "acting null") })
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

    @Test fun lockedForegroundIsUnknown() {
        screen!!.locked = true
        val f = x.run(req(UiOp.FOREGROUND)).result!!.jsonObject
        assertEquals("", f["package"]!!.jsonPrimitive.content)
        assertEquals("", f["activity"]!!.jsonPrimitive.content)
        assertEquals("false", f["protected"]!!.jsonPrimitive.content)
        val d = x.run(req(UiOp.DUMP)).result!!.jsonObject
        assertEquals("", d["package"]!!.jsonPrimitive.content)
        assertEquals("", d["activity"]!!.jsonPrimitive.content)
        // Unlocked: unchanged.
        screen!!.locked = false
        val u = x.run(req(UiOp.FOREGROUND)).result!!.jsonObject
        assertEquals("com.android.settings", u["package"]!!.jsonPrimitive.content)
        assertEquals("false", u["protected"]!!.jsonPrimitive.content)
    }

    @Test fun expiredUnavailableAndUnknown() {
        assertEquals(ErrorCode.EXPIRED, error(x.run(req(UiOp.DUMP, expires = "2026-10-01T09:59:59Z"))))
        assertEquals(ErrorCode.UNSUPPORTED, error(x.run(req("swipe"))))
        screen = null
        assertEquals(ErrorCode.UNAVAILABLE, error(x.run(req(UiOp.DUMP))))
    }

    private fun raw(json: String) = Message(v = Protocol.VERSION, type = Protocol.UI_REQUEST, ts = "2026-10-01T10:00:00Z", payload = Protocol.json.parseToJsonElement(json))

    @Test fun anUnknownFieldIsAnsweredInvalid() {
        x.onMessage(raw("""{"id":"u_1","op":"dump","args":{},"expiresAt":"2026-10-01T10:00:10Z","future":1}"""))
        assertEquals(1, sent.size)
        assertEquals("u_1", sent[0].id)
        assertFalse(sent[0].ok)
        assertEquals(ErrorCode.INVALID, error(sent[0]))
        assertTrue(screen!!.did.isEmpty())
    }

    @Test fun aMalformedFieldWithAnIdIsAnsweredInvalid() {
        x.onMessage(raw("""{"id":"u_2","op":7,"args":{},"expiresAt":"2026-10-01T10:00:10Z"}"""))
        x.onMessage(raw("""{"id":"u_3","op":"dump","args":{}}"""))
        assertEquals(listOf("u_2" to ErrorCode.INVALID, "u_3" to ErrorCode.INVALID), sent.map { it.id to error(it) })
    }

    @Test fun withoutAReadableIdThereIsNobodyToAnswer() {
        x.onMessage(raw("""{"op":"dump","args":{},"expiresAt":"2026-10-01T10:00:10Z"}"""))
        x.onMessage(raw("""{"id":5,"op":"dump","args":{},"expiresAt":"2026-10-01T10:00:10Z"}"""))
        x.onMessage(raw("""{"id":"","op":"dump","args":{},"expiresAt":"2026-10-01T10:00:10Z","future":1}"""))
        x.onMessage(raw("""[1,2]"""))
        assertTrue(sent.isEmpty())
    }

    @Test fun anInvalidIdIsNotAnsweredTwice() {
        val m = raw("""{"id":"u_9","op":"dump","args":{},"expiresAt":"2026-10-01T10:00:10Z","future":1}""")
        x.onMessage(m)
        x.onMessage(m)
        assertEquals(1, sent.size)
    }

    @Test fun lockedDumpNeverNamesTheProtectedApp() {
        screen = FakeScreen(fg = "com.example.bank" to "com.example.bank.Main", locked = true)
        val r = x.run(req(UiOp.DUMP))
        assertTrue(r.ok)
        val d = r.result!!.jsonObject
        assertEquals("true", d["locked"]!!.jsonPrimitive.content)
        assertEquals("", d["package"]!!.jsonPrimitive.content)
        assertEquals("", d["activity"]!!.jsonPrimitive.content)
        assertFalse(r.result.toString().contains("com.example.bank"))
    }

    @Test fun anExpiresAtOutOfRangeOrGarbageIsInvalid() {
        assertEquals(ErrorCode.INVALID, error(x.run(req(UiOp.DUMP, expires = "+999999999-01-01T00:00:00Z"))))
        assertEquals(ErrorCode.INVALID, error(x.run(req(UiOp.DUMP, expires = "-999999999-01-01T00:00:00Z"))))
        assertEquals(ErrorCode.INVALID, error(x.run(req(UiOp.DUMP, expires = "garbage"))))
        val sentBefore = sent.size
        x.onMessage(raw("""{"id":"u_5","op":"dump","args":{},"expiresAt":"+999999999-01-01T00:00:00Z"}"""))
        assertEquals(sentBefore + 1, sent.size)
        assertEquals(ErrorCode.INVALID, error(sent.last()))
    }

    @Test fun withoutTheNotificationsParameterTheExecutorIsClosed() {
        // The default of `notifications` is closed: an executor wired without the gate never reads any.
        val bare = UiExecutor({ screen }, { sent += it }, { now }, { false }, { false }, { false }, { false }, { it.run() })
        val r = bare.run(req(UiOp.NOTIFICATIONS))
        assertFalse(r.ok)
        assertEquals(ErrorCode.UNAVAILABLE, error(r))
        assertEquals(UiExecutor.NO_NOTIFICATION_ACCESS, r.error?.text)
    }

    @Test fun aSendThatThrowsDoesNotEscapeTheExecutor() {
        val failing = UiExecutor({ screen }, { throw IllegalStateException("socket closed") }, { now }, { it in protected }, { question }, { paused }, { false }, { it.run() })
        failing.onMessage(raw("""{"id":"u_6","op":"foreground","args":{},"expiresAt":"2026-10-01T10:00:10Z"}"""))
        failing.onMessage(raw("""{"id":"u_7","op":"dump","args":{},"expiresAt":"2026-10-01T10:00:10Z","future":1}"""))
    }

    @Test fun aRunThatThrowsAnswersFailed() {
        val broken = UiExecutor({ screen }, { sent += it }, { throw IllegalStateException("clock") }, { false }, { false }, { false }, { false }, { it.run() })
        broken.onMessage(raw("""{"id":"u_8","op":"dump","args":{},"expiresAt":"2026-10-01T10:00:10Z"}"""))
        assertEquals(listOf("u_8" to ErrorCode.FAILED), sent.map { it.id to error(it) })
    }

    @Test fun anIdThatIsTooLongIsNotAnswered() {
        val ok = "a".repeat(UiExecutor.MAX_ID)
        x.onMessage(raw("""{"id":"$ok","op":"dump","args":{},"expiresAt":"2026-10-01T10:00:10Z","future":1}"""))
        assertEquals(listOf(ok), sent.map { it.id })
        x.onMessage(raw("""{"id":"${ok}a","op":"dump","args":{},"expiresAt":"2026-10-01T10:00:10Z","future":1}"""))
        assertEquals(1, sent.size)
    }

    @Test fun withoutTheNoticeClickAndSetTextDoNotAct() {
        val msg = "ADBuddy can't show the on-screen notice, so it won't act. Ask the user to do this part."
        val field = UiNode(pkg = "com.android.settings", cls = "android.widget.EditText", bounds = Box(0, 0, 10, 10), editable = true, focused = true)
        val s = FakeScreen(actingWorks = false, focused = field)
        screen = s
        for (r in listOf(req(UiOp.CLICK, obj("text" to "Network & internet")), req(UiOp.CLICK, obj("x" to 540, "y" to 1200)), req(UiOp.SET_TEXT, obj("text" to "hi")))) {
            val a = x.run(r)
            assertEquals(ErrorCode.FAILED, error(a))
            assertEquals(msg, a.error!!.text)
        }
        assertFalse(s.did.any { it.startsWith("click") || it.startsWith("tap") || it.startsWith("setText") })
        assertTrue(x.run(req(UiOp.FOREGROUND)).ok)
    }

    @Test fun withoutTheNoticeDumpDoesNotRead() {
        val s = FakeScreen(actingWorks = false)
        screen = s
        val a = x.run(req(UiOp.DUMP))
        assertEquals(ErrorCode.FAILED, error(a))
        assertEquals("ADBuddy can't show the on-screen notice, so it won't read the screen. Ask the user to do this part.", a.error!!.text)
        // The tree is not read at all: the check comes before s.windows().
        assertFalse(s.windowsRead)
        // Foreground only reads the name of the app, not what it shows: it still answers.
        assertTrue(x.run(req(UiOp.FOREGROUND)).ok)
    }

    @Test fun aProtectedAppIsRefusedBeforeTheNotice() {
        screen = FakeScreen(actingWorks = false, fg = "com.example.bank" to "com.example.bank.Main")
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.DUMP))))
    }

    @Test fun pausedDumpIsRefused() {
        // Paused: AgentAccessibilityService.acting() is true without drawing, so the dump must not rely on it
        // (the pause can arrive after the host's doors looked).
        paused = true
        val s = FakeScreen()
        screen = s
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.DUMP))))
        assertFalse(s.windowsRead)
    }

    @Test fun noticeOpShowsTheOverlay() {
        val s = FakeScreen()
        screen = s
        val a = x.run(req(UiOp.NOTICE))
        assertTrue(a.ok)
        assertTrue(s.did.contains("acting null"))
        assertFalse(s.windowsRead)
    }

    @Test fun noticeOpFailsClosed() {
        val s = FakeScreen(actingWorks = false)
        screen = s
        assertEquals(ErrorCode.FAILED, error(x.run(req(UiOp.NOTICE))))
    }

    @Test fun noticeOpRefusedWhilePaused() {
        paused = true
        val s = FakeScreen()
        screen = s
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.NOTICE))))
        assertFalse(s.did.any { it.startsWith("acting") })
    }

    @Test fun hideNoticeTakesTheOverlayOffForTheCapture() {
        val s = FakeScreen()
        screen = s
        val r = x.run(req(UiOp.HIDE_NOTICE, obj("ms" to 2000)))
        assertTrue(r.ok)
        assertEquals(listOf("hideNotice 2000"), s.did)
        assertFalse(s.windowsRead)
    }

    @Test fun hideNoticeFailsWhenTheOverlayCannotBeTakenOff() {
        val s = FakeScreen(hideWorks = false)
        screen = s
        assertEquals(ErrorCode.FAILED, error(x.run(req(UiOp.HIDE_NOTICE, obj("ms" to 2000)))))
    }

    @Test fun hideNoticeIsRefusedWhilePaused() {
        paused = true
        val s = FakeScreen()
        screen = s
        assertEquals(ErrorCode.PROTECTED, error(x.run(req(UiOp.HIDE_NOTICE, obj("ms" to 2000)))))
        assertTrue(s.did.isEmpty())
    }

    @Test fun hideNoticeNeedsATimeWithinTheLimit() {
        val s = FakeScreen()
        screen = s
        for (ms in listOf(0, -1, UiLimits.MAX_HIDE_NOTICE_MS + 1)) {
            assertEquals("ms=$ms", ErrorCode.INVALID, error(x.run(req(UiOp.HIDE_NOTICE, obj("ms" to ms)))))
        }
        assertEquals(ErrorCode.INVALID, error(x.run(req(UiOp.HIDE_NOTICE))))
        assertEquals(ErrorCode.INVALID, error(x.run(req(UiOp.HIDE_NOTICE, obj("ms" to 2000, "extra" to 1)))))
        assertTrue(s.did.isEmpty())
        assertTrue(x.run(req(UiOp.HIDE_NOTICE, obj("ms" to 1))).ok)
        assertTrue(x.run(req(UiOp.HIDE_NOTICE, obj("ms" to UiLimits.MAX_HIDE_NOTICE_MS))).ok)
    }

    @Test fun hideNoticeWithoutTheServiceIsUnavailable() {
        screen = null
        assertEquals(ErrorCode.UNAVAILABLE, error(x.run(req(UiOp.HIDE_NOTICE, obj("ms" to 2000)))))
    }

    @Test fun neverTwiceTheSameId() {
        fun message(id: String) = Message(v = Protocol.VERSION, type = Protocol.UI_REQUEST, ts = "2026-10-01T10:00:00Z", payload = Protocol.json.encodeToJsonElement(UiRequest.serializer(), req(UiOp.FOREGROUND, id = id)))
        x.onMessage(message("u_1"))
        x.onMessage(message("u_1"))
        x.onMessage(message("u_2"))
        assertEquals(listOf("u_1", "u_2"), sent.map { it.id })
    }

    // --- notifications (M5c) ---

    private fun note(pkg: String, title: String, text: String, at: Long = now, secret: Boolean = false) =
        RawNotification("0|$pkg|1|null|1", pkg, title, text, at, ongoing = false, secret = secret)

    private fun notificationsOf(r: UiResponse) = r.result!!.jsonObject["notifications"]!!.toString()

    @Test fun notificationsHideWhatAProtectedAppAndAdbuddySay() {
        notes = listOf(
            note("com.whatsapp", "Ana", "¿Comemos?"),
            note("com.example.bank", "Bank", "Your code is 482913"),
            note("tech.hybriq.adbuddy", "The agent wants to", "adb shell reboot"),
        )
        val r = x.run(req(UiOp.NOTIFICATIONS))
        assertTrue(r.ok)
        val out = notificationsOf(r)
        assertTrue(out.contains("¿Comemos?"))
        for (secret in listOf("482913", "Bank", "reboot", "The agent wants")) assertFalse("$secret in $out", out.contains(secret))
        assertTrue(out.contains("com.example.bank")) // the package is still said
        assertEquals("false", r.result!!.jsonObject["truncated"]!!.toString())
    }

    @Test fun notificationsFollowTheProtectedListAsItChanges() {
        notes = listOf(note("com.new.bank", "Bank", "482913"))
        assertTrue(notificationsOf(x.run(req(UiOp.NOTIFICATIONS, id = "u_1"))).contains("482913"))
        protected += "com.new.bank"
        assertFalse(notificationsOf(x.run(req(UiOp.NOTIFICATIONS, id = "u_2"))).contains("482913"))
    }

    @Test fun notificationsNeedNoAccessibilityServiceAndNeverTouchTheScreen() {
        screen = null // the service is off
        notes = listOf(note("com.whatsapp", "Ana", "hola"))
        val r = x.run(req(UiOp.NOTIFICATIONS))
        assertTrue(r.ok)
        assertFalse(notificationsOf(r).isEmpty())
    }

    @Test fun notificationsWithoutAccessAreUnavailableWithHowToGrantIt() {
        notes = null
        val r = x.run(req(UiOp.NOTIFICATIONS))
        assertEquals(ErrorCode.UNAVAILABLE, error(r))
        assertTrue(r.error!!.text.contains("Notification access"))
        assertTrue(r.error!!.text.contains("Device & app notifications"))
    }

    @Test fun notificationsNeedTheUsersConfirmationAndSayWhereToGiveIt() {
        notes = listOf(note("com.whatsapp", "Ana", "hola"))
        userConfirmed = false
        val r = x.run(req(UiOp.NOTIFICATIONS))
        assertEquals(ErrorCode.UNAVAILABLE, error(r))
        assertTrue(r.error!!.text.contains("confirm it in the ADBuddy app"))
        assertFalse(r.error!!.text.contains("whatsapp"))
        userConfirmed = true
        assertTrue(x.run(req(UiOp.NOTIFICATIONS, id = "u_2")).ok)
    }

    @Test fun notificationsChecksItsArgsTheExpiryAndNeverThrows() {
        assertEquals(ErrorCode.INVALID, error(x.run(req(UiOp.NOTIFICATIONS, obj("all" to 1)))))
        assertEquals(ErrorCode.INVALID, error(x.run(req(UiOp.NOTIFICATIONS, JsonPrimitive("x")))))
        assertEquals(ErrorCode.EXPIRED, error(x.run(req(UiOp.NOTIFICATIONS, expires = "2026-10-01T09:59:59Z"))))
        notesThrow = true
        assertEquals(ErrorCode.FAILED, error(x.run(req(UiOp.NOTIFICATIONS))))
        notesThrow = false
        assertTrue(x.run(req(UiOp.NOTIFICATIONS)).ok)
    }

    @Test fun readingNotificationsDoesNotNeedADishonestHeadsUpOrAPause() {
        // It reads no screen: neither a heads-up window nor a pause (the host's doors handle it) stops it.
        notes = listOf(note("com.whatsapp", "Ana", "hola"))
        notification = true
        paused = true
        assertTrue(x.run(req(UiOp.NOTIFICATIONS)).ok)
    }
}

/** dumpsys activity service is 🟢 for the agent: the service must not print its state there. */
class AgentAccessibilityServiceTest {
    @Test fun doesNotOverrideDump() {
        assertFalse(AgentAccessibilityService::class.java.declaredMethods.any { it.name == "dump" })
    }
}
