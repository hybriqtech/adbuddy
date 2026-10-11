package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.isDialog
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.hasAnyAncestor
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
import tech.hybriq.adbuddy.ui.NotificationsRationaleContent
import tech.hybriq.adbuddy.ui.NotificationsRationaleDialog
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The reason for the notification permission in every ScreenMode (the content; a dialog lives in its own window). */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class NotificationsRationaleScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private fun text(id: Int): String = app.getString(id)

    @Test fun theContentSaysWhy() {
        compose.setContent { AdbuddyTheme(dynamic = false) { NotificationsRationaleContent() } }
        compose.onNodeWithText(text(R.string.notifications_rationale_title)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.notifications_rationale_body)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("notifications_rationale", mode)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}

@RunWith(org.robolectric.RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class NotificationsRationaleDialogTest {
    private val compose = createAndroidComposeRule<androidx.activity.ComponentActivity>()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication()

    private fun text(id: Int): String = app.getString(id)

    private fun inDialog(id: Int) = compose.onNode(hasText(text(id)) and hasAnyAncestor(isDialog()))

    @Test fun bothButtonsAnswerAndBackIsNotNow() {
        var go = 0
        var skip = 0
        compose.setContent { AdbuddyTheme(dynamic = false) { NotificationsRationaleDialog(onContinue = { go++ }, onSkip = { skip++ }) } }
        compose.onNode(isDialog()).assertIsDisplayed()
        inDialog(R.string.notifications_rationale_continue).assertIsDisplayed()
        inDialog(R.string.onboarding_skip).assertIsDisplayed()
        inDialog(R.string.notifications_rationale_continue).performClick()
        assertEquals(1, go)
        assertEquals(0, skip)
        inDialog(R.string.onboarding_skip).performClick()
        assertEquals(1, skip)
        // Back goes to the dialog's own window, not to the activity's dispatcher.
        compose.runOnUiThread { (org.robolectric.shadows.ShadowDialog.getLatestDialog() as androidx.activity.ComponentDialog).onBackPressedDispatcher.onBackPressed() }
        compose.waitForIdle()
        assertEquals(2, skip)
        assertEquals(1, go)
    }
}
