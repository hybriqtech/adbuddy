package tech.hybriq.adbuddy.ui

import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.PermissionRequest
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** C6: the agent's question is announced: the summary (what changes with each question) is a polite live region, the title a heading, the countdown silent. */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class PermissionCardSemanticsTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private val now = Instant.parse("2026-10-01T10:08:30Z").toEpochMilli()

    private fun show() {
        val p = PermissionRequest("p1", "Bash", "Clean the build folder", "", "2026-10-01T10:10:00Z", true)
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                PermissionCard(
                    p, others = 0, connected = true, hostNow = now, answering = false, guardLeftMs = 0,
                    controllerCanAnswer = true, onAnswer = { _, _ -> },
                )
            }
        }
        compose.waitForIdle()
    }

    @Test fun theSummaryIsAnnouncedTheTitleIsAHeadingAndTheCountdownIsSilent() {
        show()
        val summary = compose.onNodeWithText(app.getString(R.string.perm_summary, "Clean the build folder")).fetchSemanticsNode()
        assertEquals(LiveRegionMode.Polite, summary.config.getOrNull(SemanticsProperties.LiveRegion))
        val title = compose.onNodeWithText(app.getString(R.string.perm_title)).fetchSemanticsNode()
        assertNotNull("title is a heading", title.config.getOrNull(SemanticsProperties.Heading))
        val left = compose.onNodeWithText(app.getString(R.string.perm_left, 90), substring = true).fetchSemanticsNode()
        assertNull("the countdown must not be a live region", left.config.getOrNull(SemanticsProperties.LiveRegion))
    }
}
