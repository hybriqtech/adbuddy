package tech.hybriq.adbuddy.screens

import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertEquals
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
import tech.hybriq.adbuddy.ui.NotificationsBanner
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The "notifications are blocked" banner in every ScreenMode: whole text, an actionable button, announced politely. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class NotificationsBannerScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private fun text(id: Int): String = app.getString(id)

    @Test fun theBannerSaysItAndOpensTheSettings() {
        var opened = 0
        compose.setContent { AdbuddyTheme(dynamic = false) { NotificationsBanner(onOpenSettings = { opened++ }) } }
        compose.onNodeWithText(text(R.string.notifications_blocked)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.notifications_open_settings)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.onNodeWithText(text(R.string.notifications_blocked)).assert(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Polite))
        compose.shot("notifications_banner", mode)
        compose.onNodeWithText(text(R.string.notifications_open_settings)).performClick()
        assertEquals(1, opened)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
