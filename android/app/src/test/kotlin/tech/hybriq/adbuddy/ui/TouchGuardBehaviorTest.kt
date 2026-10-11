package tech.hybriq.adbuddy.ui

import android.view.View
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.test.junit4.createComposeRule
import java.time.Instant
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.bridge.PermissionRequest
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * Tapjacking (O1), drawn for real: while a granting surface is composed its window filters touches that another
 * window covers, and when it leaves the original value comes back. The dialogs (own windows) are covered by
 * TouchGuardSourceTest: Robolectric does not lay out a Dialog's window for ui-test to reach its root view.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class TouchGuardBehaviorTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val now = Instant.parse("2026-10-01T10:08:30Z").toEpochMilli()
    private var root: View? = null

    @Composable
    private fun Card() {
        val p = PermissionRequest("p1", "Bash", "Clean", "rm -rf build", "2026-10-01T10:10:00Z", true)
        PermissionCard(p, others = 0, connected = true, hostNow = now, answering = false, guardLeftMs = 0, controllerCanAnswer = true, onAnswer = { _, _ -> })
    }

    @Composable
    private fun Probe() {
        root = LocalView.current.rootView
    }

    private fun filtering(): Boolean = root!!.filterTouchesWhenObscured

    @Test fun consentScreenFiltersWhileShownAndRestoresAfter() {
        var show by mutableStateOf(true)
        compose.setContent { AdbuddyTheme(dynamic = false) { Probe(); if (show) ConsentScreen("my-pc") {} } }
        compose.waitForIdle()
        assertTrue(filtering())
        show = false
        compose.waitForIdle()
        assertFalse(filtering())
    }

    @Test fun permissionCardFiltersWhileShownAndRestoresAfter() {
        var show by mutableStateOf(true)
        compose.setContent { AdbuddyTheme(dynamic = false) { Probe(); if (show) Card() } }
        compose.waitForIdle()
        assertTrue(filtering())
        show = false
        compose.waitForIdle()
        assertFalse(filtering())
    }

    @Test fun twoUsersAtOnceKeepTheFlagUntilTheLastLeaves() {
        var consent by mutableStateOf(true)
        var card by mutableStateOf(true)
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                Probe()
                if (consent) ConsentScreen("my-pc") {}
                if (card) Card()
            }
        }
        compose.waitForIdle()
        assertTrue(filtering())
        consent = false
        compose.waitForIdle()
        assertTrue("the card is still there", filtering())
        card = false
        compose.waitForIdle()
        assertFalse(filtering())
    }
}
