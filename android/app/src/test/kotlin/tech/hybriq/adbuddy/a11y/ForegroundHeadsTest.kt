package tech.hybriq.adbuddy.a11y

import android.view.accessibility.AccessibilityWindowInfo
import java.time.Instant
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.UiOp
import tech.hybriq.adbuddy.bridge.UiRequest
import tech.hybriq.adbuddy.bridge.UiResponse

/** P5: `foreground` reads only each window's type and root package (Screen.windowHeads), never the tree. */
class ForegroundHeadsTest {
    /** A screen whose windows() blows up: foreground must decide from windowHeads() alone. */
    private class HeadsScreen(var heads: List<UiWindowHead>, var fg: String = "com.android.settings") : Screen {
        override fun windows(): List<UiWindow> = throw IllegalStateException("the tree must not be read")
        override fun windowHeads() = heads
        override fun screen() = ScreenFixtures.SCREEN
        override fun rotation() = 0
        override fun locked() = false
        override fun foreground() = fg to "$fg.Main"
        override fun click(node: UiNode) = false
        override fun tap(x: Int, y: Int) = false
        override fun focusedInput(): UiNode? = null
        override fun setText(node: UiNode, text: String) = false
        override fun acting(mark: Box?) = true
    }

    private val now = Instant.parse("2026-10-01T10:00:00Z").toEpochMilli()
    private fun run(s: Screen): UiResponse =
        UiExecutor({ s }, {}, { now }, { it == "com.example.bank" }, { false }, { false }, { false }, { it.run() })
            .run(UiRequest("u_1", UiOp.FOREGROUND, JsonObject(emptyMap()), "2026-10-01T10:00:10Z"))

    private fun protectedOf(heads: List<UiWindowHead>): Boolean {
        val r = run(HeadsScreen(heads))
        assertTrue("answer: ${r.error}", r.ok)
        return r.result!!.jsonObject["protected"]!!.jsonPrimitive.boolean
    }

    @Test fun headsDecideWithoutReadingTheTree() {
        // (b) a window with content hidden: app, empty package -> protected
        assertEquals(true, protectedOf(listOf(UiWindowHead("app", ""))))
        // Settings + system UI -> not protected
        assertEquals(false, protectedOf(listOf(UiWindowHead("app", "com.android.settings"), UiWindowHead("system", "com.android.systemui"))))
    }

    @Test fun moreCases() {
        // (c) keyboard and a contentless system window are not app windows
        assertEquals(false, protectedOf(listOf(UiWindowHead("app", "com.android.settings"), UiWindowHead("ime", "com.google.android.inputmethod"), UiWindowHead("system", ""))))
        // ADBuddy's own window not at the front still counts
        assertEquals(true, protectedOf(listOf(UiWindowHead("app", "com.android.settings"), UiWindowHead("dialog", Packages.ADBUDDY))))
        // a guarded app in the second half
        assertEquals(true, protectedOf(listOf(UiWindowHead("app", "com.android.settings"), UiWindowHead("dialog", "com.example.bank"))))
        // a dialog window without content hides what it is
        assertEquals(true, protectedOf(listOf(UiWindowHead("app", "com.android.settings"), UiWindowHead("dialog", ""))))
    }

    @Test fun typeOfHasItsFourBranches() {
        val svc = AgentAccessibilityService.Companion
        assertEquals("app", svc.typeOf(AccessibilityWindowInfo.TYPE_APPLICATION, true, false))
        assertEquals("app", svc.typeOf(AccessibilityWindowInfo.TYPE_APPLICATION, false, true))
        assertEquals("dialog", svc.typeOf(AccessibilityWindowInfo.TYPE_APPLICATION, false, false))
        assertEquals("ime", svc.typeOf(AccessibilityWindowInfo.TYPE_INPUT_METHOD, true, true))
        assertEquals("split", svc.typeOf(AccessibilityWindowInfo.TYPE_SPLIT_SCREEN_DIVIDER, false, false))
        assertEquals("system", svc.typeOf(AccessibilityWindowInfo.TYPE_SYSTEM, true, true))
    }
}
