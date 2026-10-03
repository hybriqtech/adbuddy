package tech.hybriq.adbuddy.ui

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import org.junit.Assert.assertNotNull
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
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.screens.ScreenMode
import tech.hybriq.adbuddy.screens.ScreenModeRule
import tech.hybriq.adbuddy.screens.assertTextsFit
import tech.hybriq.adbuddy.screens.assertTitleFitsTheTopBar
import tech.hybriq.adbuddy.screens.screenModes
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** O3 and O13: the title of the top bar is a heading and fits the bar it is drawn in, in every mode (the longest real title: access_title). */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class AdbuddyTopBarTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val title get() = (RuntimeEnvironment.getApplication() as AdbuddyApp).getString(R.string.access_title)

    @Test fun theTitleIsAHeadingAndFitsTheBar() {
        compose.setContent { AdbuddyTheme(dynamic = false) { AdbuddyTopBar(title = title, onBack = {}) } }
        val node = compose.onNodeWithText(title).fetchSemanticsNode()
        assertNotNull("the title is a heading", node.config.getOrNull(SemanticsProperties.Heading))
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(title)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
