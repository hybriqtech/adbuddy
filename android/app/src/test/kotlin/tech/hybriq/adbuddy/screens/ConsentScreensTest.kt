package tech.hybriq.adbuddy.screens

import androidx.compose.material3.MaterialTheme
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.text.TextStyle
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.ui.ConsentScreen
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The consent screen (what the accessibility service grants, its note and its two buttons), in every ScreenMode. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ConsentScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private val answers = mutableListOf<Boolean>()
    private var bodyMedium: TextStyle? = null

    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    private fun show() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                bodyMedium = MaterialTheme.typography.bodyMedium
                ConsentScreen(host = "my-pc", onAnswer = { answers += it })
            }
        }
    }

    private fun button(id: Int) = compose.onNode(hasText(text(id)) and hasClickAction())

    @Test fun theScreenSaysWhatItGrantsAndItsNoteIsReadable() {
        show()
        val title = text(R.string.consent_title, "my-pc")
        compose.onNodeWithText(title).assertIsDisplayed()
        compose.onNodeWithText(title).assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
        compose.onNodeWithText(text(R.string.consent_intro, "my-pc")).assertIsDisplayed()
        for (id in listOf(R.string.consent_grant, R.string.consent_battery, R.string.consent_a11y, R.string.consent_note)) {
            compose.onNodeWithText(text(id)).performScrollTo().assertIsDisplayed()
        }
        // The accessibility row says what it really lets the agent do.
        val a11y = text(R.string.consent_a11y)
        val wanted = if (mode == ScreenMode.FONT_200_ES) listOf("leer la pantalla", "escribir", "Pausa", "apps protegidas")
        else listOf("read the screen", "type", "Pause", "protected apps")
        for (w in wanted) assertTrue("consent_a11y lacks \"$w\": $a11y", a11y.contains(w))
        // The note is bodyMedium, not the smallest style.
        val results = mutableListOf<androidx.compose.ui.text.TextLayoutResult>()
        compose.onNodeWithText(text(R.string.consent_note)).fetchSemanticsNode().config[SemanticsActions.GetTextLayoutResult].action!!(results)
        assertEquals(bodyMedium!!.fontSize, results.single().layoutInput.style.fontSize)
        compose.assertTextsFit(scrolled = true)
        compose.shot("consent", mode)
    }

    @Test fun theAnswersAreAcceptTrueAndDeclineFalse() {
        show()
        button(R.string.consent_accept).assertIsDisplayed().performClick()
        assertEquals(listOf(true), answers)
        button(R.string.consent_decline).assertIsDisplayed().performClick()
        assertEquals(listOf(true, false), answers)
    }

    @Test fun theButtonsStackWithLargeTextAndSitInARowOtherwise() {
        show()
        val accept = button(R.string.consent_accept).fetchSemanticsNode()
        val decline = button(R.string.consent_decline).fetchSemanticsNode()
        if (mode.fontScale > 1.3f) {
            assertEquals(accept.positionInRoot.x, decline.positionInRoot.x, 0.5f)
            assertEquals(accept.size.width, decline.size.width)
            assertTrue("accept on top", accept.positionInRoot.y < decline.positionInRoot.y)
        } else {
            assertEquals(accept.positionInRoot.y, decline.positionInRoot.y, 0.5f)
            assertTrue("decline to the left", decline.positionInRoot.x < accept.positionInRoot.x)
        }
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
