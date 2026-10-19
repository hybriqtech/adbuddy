package tech.hybriq.adbuddy.a11y

import java.time.Instant
import kotlinx.serialization.json.JsonObject
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

class YoloProtectionTest {
    private val connected = BridgeState.Connection.CONNECTED

    // Mutation: drop the connection from yoloOn.
    @Test fun theModeCountsOnlyWhenKnownActiveAndTheLinkIsUp() {
        assertTrue(YoloProtection.yoloOn(BridgeState(connection = connected, yolo = YoloState(active = true, session = true))))
        assertFalse("link down: the state may be stale", YoloProtection.yoloOn(BridgeState(connection = BridgeState.Connection.WAITING, yolo = YoloState(active = true, session = true))))
        assertFalse("off", YoloProtection.yoloOn(BridgeState(connection = connected, yolo = YoloState())))
        assertFalse("unknown", YoloProtection.yoloOn(BridgeState(connection = connected, yolo = null)))
    }

    // The executor, with the predicate the app gives it.
    private class FakeScreen(var fg: String) : Screen {
        override fun windows() = ScreenFixtures.windows
        override fun screen() = ScreenFixtures.SCREEN
        override fun rotation() = 0
        override fun locked() = false
        override fun foreground() = fg to "$fg.Main"
        override fun click(node: UiNode) = true
        override fun tap(x: Int, y: Int) = true
        override fun focusedInput(): UiNode? = null
        override fun setText(node: UiNode, text: String) = true
        override fun acting(mark: Box?) = true
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

    @Test fun aHeadsUpNotificationStopsTheHelpersOnlyWithoutYolo() {
        screen.fg = "com.android.settings"
        headsUp = true
        assertTrue(code(x.run(req(UiOp.DUMP, "u_1"))) != "ok")
        yolo = true
        assertEquals("ok", code(x.run(req(UiOp.DUMP, "u_2"))))
    }
}
