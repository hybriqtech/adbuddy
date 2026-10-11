package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
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
import tech.hybriq.adbuddy.ui.PendingQuestionsBanner
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The notice for closed red questions in every ScreenMode: whole text and an actionable "Show". */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class PendingQuestionsBannerScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    @Test fun theNoticeFitsAndOffersShow() {
        compose.setContent { AdbuddyTheme(dynamic = false) { PendingQuestionsBanner(count = 2, onShow = {}) } }
        compose.onNodeWithText(app.getString(R.string.confirm_waiting_show)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("pending_questions_banner", mode)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
