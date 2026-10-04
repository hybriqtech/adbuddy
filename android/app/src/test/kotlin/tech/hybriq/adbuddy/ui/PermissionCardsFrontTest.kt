package tech.hybriq.adbuddy.ui

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.lifecycle.Lifecycle
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.bridge.FakeChannel
import tech.hybriq.adbuddy.bridge.PermissionRequest
import tech.hybriq.adbuddy.chat.ChatController
import tech.hybriq.adbuddy.chat.PermissionUi
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * The agent's question card with the same criterion as the 🔴 dialog: when the app comes back to the front
 * (ON_START/ON_RESUME) PermissionCards rearms the promote guard (PermissionUi.PROMOTE_GUARD_MS), so a tap as the app
 * comes up answers nothing. Showing the card does not rearm it by itself.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class PermissionCardsFrontTest {
    private val compose = createAndroidComposeRule<ComponentActivity>()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private var now = 1_000_000L
    private val chat = ChatController(FakeChannel(), clock = { now }) { "c" }
    private val hostNow = Instant.parse("2026-10-01T10:08:30Z").toEpochMilli()
    private val p = PermissionRequest("p1", "Bash", "Clean", "rm -rf build", "2026-10-01T10:10:00Z", true)

    @Test fun backToTheFrontTheCardIsGuardedAgain() {
        chat.onPermissions(listOf(p))
        now += PermissionUi.PROMOTE_GUARD_MS
        compose.setContent { AdbuddyTheme(dynamic = false) { PermissionCards(listOf(p), connected = true, hostNow = { hostNow }, chat = chat) } }
        compose.waitForIdle()
        assertTrue("showing the card does not guard it again", chat.canAnswer(p.id))

        compose.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
        compose.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
        assertFalse("a tap as the app comes back answers nothing", chat.canAnswer(p.id))
        assertEquals(PermissionUi.PROMOTE_GUARD_MS, chat.guardLeftMs(p.id))
        assertFalse(chat.answerPermission(p.id, PermissionUi.ALLOW))
        now += PermissionUi.PROMOTE_GUARD_MS
        assertTrue(chat.answerPermission(p.id, PermissionUi.ALLOW))
    }
}
