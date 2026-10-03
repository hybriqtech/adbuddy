package tech.hybriq.adbuddy.ui

import android.content.res.Configuration
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeUp
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.ExternalResource
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.PermissionRequest
import tech.hybriq.adbuddy.chat.PermissionUi
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * Validation M4-c: the agent's question with a long detail. Yes and "Yes, always" stay off until the detail has been
 * scrolled to its end (a swipe on the detail, the same ScrollState the gate reads), and No is always there; drawn for
 * real (Compose ui-test on Robolectric) in portrait and landscape, with text at 100 % and 200 %. The keyboard is not
 * covered: Robolectric has no input method to open.
 */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class PermissionCardScrollTest(private val qualifiers: String, private val fontScale: Float) {
    private val compose = createComposeRule()

    private val config = object : ExternalResource() {
        override fun before() {
            RuntimeEnvironment.setQualifiers(qualifiers)
            RuntimeEnvironment.setFontScale(fontScale)
        }
    }

    @get:Rule val rules: RuleChain = RuleChain.outerRule(config).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private fun text(id: Int): String = app.getString(id)

    private val now = Instant.parse("2026-10-01T10:08:30Z").toEpochMilli()

    private val answers = mutableListOf<String>()

    private fun show(detail: String) {
        val conf = app.resources.configuration
        val land = qualifiers == "+land"
        assertEquals("orientation", if (land) Configuration.ORIENTATION_LANDSCAPE else Configuration.ORIENTATION_PORTRAIT, conf.orientation)
        assertTrue("landscape is wider than tall", (conf.screenWidthDp > conf.screenHeightDp) == land)
        assertEquals(fontScale, conf.fontScale)
        val p = PermissionRequest("p1", "Bash", "Clean the build folder", detail, "2026-10-01T10:10:00Z", true)
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                PermissionCard(
                    p, others = 0, connected = true, hostNow = now, answering = false, guardLeftMs = 0,
                    controllerCanAnswer = true, onAnswer = { _, d -> answers += d },
                )
            }
        }
        compose.waitForIdle()
    }

    private fun button(id: Int): SemanticsNodeInteraction = compose.onNodeWithText(text(id))

    private fun scrollable(): SemanticsNodeInteraction = compose.onNode(hasScrollAction())

    private fun atTheEnd(): Boolean {
        val range = scrollable().fetchSemanticsNode().config.getOrNull(SemanticsProperties.VerticalScrollAxisRange)!!
        return range.value() >= range.maxValue()
    }

    @Test fun yesWaitsForTheEndOfALongDetailAndNoNever() {
        show((1..40).joinToString("\n") { "rm -rf build/output/part-$it" })
        assertTrue("the detail must be taller than its box for this test", !atTheEnd())
        button(R.string.perm_allow).assertIsNotEnabled()
        button(R.string.perm_always).assertIsNotEnabled()
        button(R.string.perm_deny).assertIsEnabled()
        compose.onNodeWithText(text(R.string.perm_scroll)).assertIsDisplayed()
        button(R.string.perm_allow).performClick()
        button(R.string.perm_always).performClick()
        assertEquals("a tap on a button that is off answers nothing", emptyList<String>(), answers)

        repeat(200) {
            if (atTheEnd()) return@repeat
            scrollable().performTouchInput { swipeUp() }
            compose.waitForIdle()
        }
        assertTrue("the detail never reached its end", atTheEnd())
        button(R.string.perm_allow).assertIsEnabled()
        button(R.string.perm_always).assertIsEnabled()
        button(R.string.perm_deny).assertIsEnabled()
        compose.onNodeWithText(text(R.string.perm_scroll)).assertDoesNotExist()
        button(R.string.perm_always).performClick()
        assertEquals(listOf(PermissionUi.ALLOW_ALWAYS), answers)
    }

    @Test fun noCanBeTappedBeforeReading() {
        show((1..40).joinToString("\n") { "rm -rf build/output/part-$it" })
        button(R.string.perm_deny).performClick()
        assertEquals(listOf(PermissionUi.DENY), answers)
    }

    @Test fun aShortDetailCanBeAnsweredAtOnce() {
        show("rm -rf build")
        button(R.string.perm_allow).assertIsEnabled()
        button(R.string.perm_always).assertIsEnabled()
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0} x{1}")
        fun configs() = listOf(
            arrayOf<Any>("+port", 1f),
            arrayOf<Any>("+port", 2f),
            arrayOf<Any>("+land", 1f),
            arrayOf<Any>("+land", 2f),
        )
    }
}
