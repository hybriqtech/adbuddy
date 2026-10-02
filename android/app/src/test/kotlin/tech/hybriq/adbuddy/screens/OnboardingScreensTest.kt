package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.hasScrollToKeyAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToKey
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.listener.NotificationAccess.Effective
import tech.hybriq.adbuddy.ui.NotificationAccessContent
import tech.hybriq.adbuddy.ui.OnboardingStep
import tech.hybriq.adbuddy.ui.ProtectedAppsContent
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** Validation 27: the two first-run steps, in every ScreenMode (golden images and the texts on screen). */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class OnboardingScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private fun text(id: Int, vararg args: Any): String = RuntimeEnvironment.getApplication().getString(id, *args)

    @Test fun stepOneProtectedApps() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ProtectedAppsContent(OnboardingStep.PROTECTED_APPS, false, false, {}, SAMPLE_ROWS, 2, true, "", {}, { _, _ -> }, true, {}, {}, {})
            }
        }
        compose.onNodeWithText(text(R.string.step_progress, 1, 2)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.protected_proposal)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.protected_count, 2)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_skip)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.protected_confirm)).assertIsDisplayed().assertIsEnabled()
        compose.assertTextsFit()
        compose.shot("onboarding_1_protected_apps", mode)
        // A lazy list only composes the rows in view: scroll by key, then look, and check the rows' texts too.
        compose.onNode(hasScrollToKeyAction()).performScrollToKey("com.google.android.gm")
        compose.onNodeWithText("Gmail").assertIsDisplayed()
        compose.assertTextsFit(scrolled = true)
        compose.shot("onboarding_1_protected_apps_rows", mode)
        compose.onNode(hasScrollToKeyAction()).performScrollToKey("com.example.mybank")
        compose.onNodeWithText("Mi Banco").assertIsDisplayed()
        compose.assertTextsFit(scrolled = true)
    }

    @Test fun stepTwoNotificationAccess() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                NotificationAccessContent(OnboardingStep.NOTIFICATION_ACCESS, false, false, Effective.OFF, {}, {}, {}, {}, {}, {})
            }
        }
        compose.onNodeWithText(text(R.string.step_progress, 2, 2)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_skip)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_done)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("onboarding_2_notification_access", mode)
        compose.onNodeWithText(text(R.string.access_open)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_codes_hint)).performScrollTo().assertIsDisplayed()
    }

    @Test fun stepTwoWithTheAccessOnHasNoNotNow() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                NotificationAccessContent(OnboardingStep.NOTIFICATION_ACCESS, false, true, Effective.ON, {}, {}, {}, {}, {}, {})
            }
        }
        compose.onNodeWithText(text(R.string.access_state_granted)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_done)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_skip)).assertDoesNotExist()
        compose.assertTextsFit()
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
