package tech.hybriq.adbuddy.a11y

import java.time.Instant
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ErrorCode
import tech.hybriq.adbuddy.bridge.UiOp
import tech.hybriq.adbuddy.bridge.UiRequest
import tech.hybriq.adbuddy.bridge.UiResponse
import tech.hybriq.adbuddy.bridge.YoloState
import tech.hybriq.adbuddy.confirm.HostClock
import tech.hybriq.adbuddy.listener.NotificationRead
import tech.hybriq.adbuddy.listener.RawNotification

class YoloProtectionTest {
    private val connected = BridgeState.Connection.CONNECTED

    private val hostNow = Instant.parse("2026-10-01T10:00:00Z").toEpochMilli()
    private val hostClock = HostClock(wall = { hostNow }, elapsed = { 0 })

    // Mutation: drop the connection from yoloOn.
    @Test fun theModeCountsOnlyWhenKnownActiveAndTheLinkIsUp() {
        assertTrue(YoloProtection.yoloOn(BridgeState(connection = connected, yolo = YoloState(active = true, session = true)), hostClock))
        assertFalse("link down: the state may be stale", YoloProtection.yoloOn(BridgeState(connection = BridgeState.Connection.WAITING, yolo = YoloState(active = true, session = true)), hostClock))
        assertFalse("off", YoloProtection.yoloOn(BridgeState(connection = connected, yolo = YoloState()), hostClock))
        assertFalse("unknown", YoloProtection.yoloOn(BridgeState(connection = connected, yolo = null), hostClock))
    }

    // Mutation: drop the clock check from yoloOn (an old state kept over a reconnection would read as on).
    @Test fun aTimedModeThatExpiredOnTheHostsClockIsNotOn() {
        val timed = { until: Long -> BridgeState(connection = connected, yolo = YoloState(active = true, until = until)) }
        assertFalse("expired", YoloProtection.yoloOn(timed(hostNow - 1), hostClock))
        assertFalse("expires right now", YoloProtection.yoloOn(timed(hostNow), hostClock))
        assertTrue("still running", YoloProtection.yoloOn(timed(hostNow + 60_000), hostClock))
        assertTrue("until the session ends has no expiry", YoloProtection.yoloOn(BridgeState(connection = connected, yolo = YoloState(active = true, until = 0, session = true)), hostClock))
    }

    // Mutation: capture the state once in `predicates` (or wire UiHelpers to the user's list alone).
    @Test fun thePredicatesReadTheLiveStateOnEveryCall() {
        var state = BridgeState(connection = connected, yolo = YoloState(active = true, session = true))
        var shade = true
        val p = YoloProtection.predicates({ state }, hostClock, { it == "com.example.bank" }, { shade })
        assertFalse("YOLO up: the user's app is not protected", p.isProtected("com.example.bank"))
        assertFalse("YOLO up: no notification block", p.notificationOnScreen())
        assertTrue("ADBuddy always", p.isProtected(Packages.ADBUDDY))
        state = BridgeState(connection = BridgeState.Connection.WAITING, yolo = YoloState(active = true, session = true))
        assertTrue("link down: protects as before", p.isProtected("com.example.bank"))
        assertTrue(p.notificationOnScreen())
        state = BridgeState(connection = connected, yolo = YoloState(active = true, session = true))
        assertFalse("link back up", p.isProtected("com.example.bank"))
        state = BridgeState(connection = connected, yolo = YoloState())
        assertTrue("turned off", p.isProtected("com.example.bank"))
        assertTrue(p.notificationOnScreen())
        shade = false
        assertFalse("nothing on screen", p.notificationOnScreen())
    }

    // The executor, with the predicate the app gives it.
    private class FakeScreen(var fg: String, var wins: List<UiWindow> = ScreenFixtures.windows) : Screen {
        override fun windows() = wins
        override fun screen() = ScreenFixtures.SCREEN
        override fun rotation() = 0
        override fun locked() = false
        override fun foreground() = fg to "$fg.Main"
        override fun click(node: UiNode) = true
        override fun tap(x: Int, y: Int) = true
        var focused: UiNode? = null
        var canShow = true
        override fun focusedInput(): UiNode? = focused
        override fun setText(node: UiNode, text: String) = true
        override fun acting(mark: Box?) = canShow
        override fun hideNotice(ms: Long) = true
    }

    private val now = Instant.parse("2026-10-01T10:00:00Z").toEpochMilli()
    private val sent = mutableListOf<UiResponse>()
    private var yolo = false
    private var headsUp = false
    private val screen = FakeScreen("com.example.bank")
    private val userList = setOf("com.example.bank")
    private val x = UiExecutor(
        { screen }, { sent += it }, { now },
        YoloProtection.effective({ yolo }, { it in userList }),
        { false }, { false },
        YoloProtection.notificationBlocks({ yolo }, { headsUp }),
        { it.run() },
        notifications = { NotificationRead.Ok(listOf(RawNotification("0|com.example.bank|1|null|1", "com.example.bank", "Bank", "Your code is 482913", now, ongoing = false, secret = false))) },
    )

    private fun req(op: String, id: String = "u_1") = UiRequest(id, op, JsonObject(emptyMap()), "2026-10-01T10:00:10Z")
    private fun code(r: UiResponse) = r.error?.code ?: "ok"

    // Mutation: make effective ignore yoloActive().
    @Test fun aProtectedAppIsReadOnlyUnderYolo() {
        assertEquals(ErrorCode.PROTECTED, code(x.run(req(UiOp.DUMP, "u_1"))))
        yolo = true
        assertEquals("ok", code(x.run(req(UiOp.DUMP, "u_2"))))
        yolo = false
        assertEquals(ErrorCode.PROTECTED, code(x.run(req(UiOp.DUMP, "u_3"))))
    }

    @Test fun aProtectedAppCanBeTouchedUnderYolo() {
        val click = { id: String ->
            UiRequest(id, UiOp.CLICK, kotlinx.serialization.json.buildJsonObject {
                put("x", kotlinx.serialization.json.JsonPrimitive(540))
                put("y", kotlinx.serialization.json.JsonPrimitive(1200))
            }, "2026-10-01T10:00:10Z")
        }
        assertEquals(ErrorCode.PROTECTED, code(x.run(click("u_1"))))
        yolo = true
        assertEquals("ok", code(x.run(click("u_2"))))
    }

    // Mutation: let effective lift ADBuddy too.
    @Test fun adbuddyItselfIsNeverLifted() {
        screen.fg = Packages.ADBUDDY
        yolo = true
        assertEquals(ErrorCode.PROTECTED, code(x.run(req(UiOp.DUMP))))
        assertTrue(YoloProtection.effective({ true }, { false })(Packages.ADBUDDY))
        assertFalse(YoloProtection.effective({ true }, { true })("com.example.bank"))
    }

    // Mutation: make `effective` ignore yoloActive() for the notifications op's guard.
    @Test fun theNotificationsOpShowsAListedAppsContentOnlyUnderYolo() {
        val shown = { r: UiResponse -> r.result!!.toString().contains("482913") }
        assertFalse(shown(x.run(req(UiOp.NOTIFICATIONS, "u_1"))))
        yolo = true
        assertTrue(shown(x.run(req(UiOp.NOTIFICATIONS, "u_2"))))
        yolo = false
        assertFalse(shown(x.run(req(UiOp.NOTIFICATIONS, "u_3"))))
    }

    // Mutation: in UiExecutor.guardedOnScreen, skip the windows of ADBuddy (the split-screen half).
    @Test fun adbuddyInSplitScreenStillMarksTheForegroundProtectedUnderYolo() {
        yolo = true
        val split = FakeScreen("com.android.settings", ScreenFixtures.windows + UiWindow("app", 3, Box(0, 1500, 1344, 2992), ScreenFixtures.adbuddy))
        val y = UiExecutor(
            { split }, { sent += it }, { now },
            YoloProtection.effective({ yolo }, { it in userList }),
            { false }, { false }, YoloProtection.notificationBlocks({ yolo }, { headsUp }), { it.run() },
        )
        val flag = { r: UiResponse -> r.result!!.jsonObject["protected"]!!.jsonPrimitive.content }
        // Windows: settings and, in the other half, ADBuddy itself as an app window.
        assertEquals("true", flag(y.run(req(UiOp.FOREGROUND))))
    }

    @Test fun aHeadsUpNotificationStopsTheHelpersOnlyWithoutYolo() {
        screen.fg = "com.android.settings"
        headsUp = true
        assertEquals(ErrorCode.PROTECTED, code(x.run(req(UiOp.DUMP, "u_1"))))
        yolo = true
        assertEquals("ok", code(x.run(req(UiOp.DUMP, "u_2"))))
    }

    // The app's own notice: under YOLO it is not required (R1 extended), the rest of the guards stay.
    private var live = BridgeState(connection = connected, yolo = YoloState(active = true, session = true))
    private var paused = false
    private val mute = FakeScreen("com.android.settings").also {
        it.canShow = false
        it.focused = UiNode(pkg = "com.android.settings", cls = "android.widget.EditText", bounds = Box(0, 0, 10, 10), editable = true, focused = true)
    }
    private val noticeX: UiExecutor = YoloProtection.predicates({ live }, hostClock, { it in userList }, { false }).let { p ->
        UiExecutor({ mute }, { sent += it }, { now }, p.isProtected, { false }, { paused }, p.notificationOnScreen, { it.run() }, noticeOptional = p.noticeOptional)
    }

    private fun obj(vararg kv: Pair<String, Any>) = kotlinx.serialization.json.buildJsonObject {
        for ((k, v) in kv) put(k, if (v is Int) kotlinx.serialization.json.JsonPrimitive(v) else kotlinx.serialization.json.JsonPrimitive(v as String))
    }

    private fun fourOps(tag: String) = listOf(
        UiRequest("${tag}_d", UiOp.DUMP, JsonObject(emptyMap()), "2026-10-01T10:00:10Z"),
        UiRequest("${tag}_c", UiOp.CLICK, obj("x" to 540, "y" to 1200), "2026-10-01T10:00:10Z"),
        UiRequest("${tag}_t", UiOp.SET_TEXT, obj("text" to "hi"), "2026-10-01T10:00:10Z"),
        UiRequest("${tag}_n", UiOp.NOTICE, JsonObject(emptyMap()), "2026-10-01T10:00:10Z"),
    ).map { code(noticeX.run(it)) }

    // Mutation: noticeOptional ignores the state (always true).
    @Test fun withoutYoloAnUndrawableNoticeStillStopsEveryOp() {
        live = BridgeState(connection = connected, yolo = YoloState())
        assertEquals(List(4) { ErrorCode.FAILED }, fourOps("a"))
    }

    // Mutation: remove the check from notice (use s.acting(null) alone), or from any op.
    @Test fun underYoloAnUndrawableNoticeDoesNotStopDumpClickSetTextOrNotice() {
        assertEquals(List(4) { "ok" }, fourOps("b"))
    }

    @Test fun theNoticeIsStillAskedForUnderYolo() {
        var asked = 0
        val s = object : Screen by mute { override fun acting(mark: Box?): Boolean { asked++; return false } }
        val p = YoloProtection.predicates({ live }, hostClock, { false }, { false })
        val e = UiExecutor({ s }, { sent += it }, { now }, p.isProtected, { false }, { false }, p.notificationOnScreen, { it.run() }, noticeOptional = p.noticeOptional)
        assertEquals("ok", code(e.run(UiRequest("n1", UiOp.NOTICE, JsonObject(emptyMap()), "2026-10-01T10:00:10Z"))))
        assertEquals(1, asked)
    }

    @Test fun pauseStillWinsUnderYolo() {
        paused = true
        assertEquals(List(4) { ErrorCode.PROTECTED }, fourOps("p"))
    }

    // Mutation: noticeOptional ignores the connection.
    @Test fun withTheLinkDownOrTheModeExpiredTheNoticeIsRequiredAgain() {
        live = BridgeState(connection = BridgeState.Connection.WAITING, yolo = YoloState(active = true, session = true))
        assertEquals(List(4) { ErrorCode.FAILED }, fourOps("l"))
        live = BridgeState(connection = connected, yolo = YoloState(active = true, until = hostNow - 1))
        assertEquals(List(4) { ErrorCode.FAILED }, fourOps("e"))
    }

    @Test fun adbuddyInFrontIsStillProtectedUnderYolo() {
        mute.fg = Packages.ADBUDDY
        val r = fourOps("z")
        assertEquals(listOf(ErrorCode.PROTECTED, ErrorCode.PROTECTED, ErrorCode.PROTECTED), r.take(3))
    }

    @Test fun thePredicatesNoticeOptionalReadsTheLiveState() {
        val p = YoloProtection.predicates({ live }, hostClock, { false }, { false })
        assertTrue(p.noticeOptional())
        live = BridgeState(connection = connected, yolo = YoloState())
        assertFalse(p.noticeOptional())
    }
}
