package tech.hybriq.adbuddy.ui

import androidx.compose.material3.Button
import androidx.compose.material3.Text
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.screens.ScreenMode
import tech.hybriq.adbuddy.screens.ScreenModeRule
import tech.hybriq.adbuddy.screens.assertTextsFit
import tech.hybriq.adbuddy.screens.screenModes
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** O5: the footer's actions sit side by side with normal text and stack at full width with large text, never squeezed. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ActionFooterTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val main = if (mode.fontScale > STACK_FROM_FONT_SCALE) "Confirmar la lista de apps protegidas" else "Confirmar la lista"
    private val other = "Ahora no"

    @Test fun sideBySideWithNormalTextAndStackedWithLargeText() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ActionFooter(
                    main = { m -> Button(onClick = {}, modifier = m) { Text(main) } },
                    other = { m -> SecondaryActionButton(other, onClick = {}, modifier = m) },
                )
            }
        }
        val a = compose.onNodeWithText(main).fetchSemanticsNode().boundsInRoot
        val b = compose.onNodeWithText(other).fetchSemanticsNode().boundsInRoot
        if (mode.fontScale > STACK_FROM_FONT_SCALE) {
            // Stacked: the main action above, both as wide as the footer's content, neither squeezed.
            assertTrue("main above other", a.bottom <= b.top + 0.5f)
            assertEquals("same left", a.left, b.left, 0.5f)
            assertEquals("same width", a.width, b.width, 0.5f)
        } else {
            // Side by side: the same row, the secondary action at the left of the main one.
            assertEquals("same row", a.center.y, b.center.y, 2f)
            assertTrue("other left of main", b.right <= a.left + 0.5f)
        }
        compose.assertTextsFit()
    }

    @Test fun withoutASecondaryActionTheMainOneIsAlone() {
        compose.setContent { AdbuddyTheme(dynamic = false) { ActionFooter(main = { m -> Button(onClick = {}, modifier = m) { Text(main) } }) } }
        compose.onNodeWithText(main).assertIsDisplayed()
        compose.assertTextsFit()
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
