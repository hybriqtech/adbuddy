package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.hasScrollToKeyAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
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
import tech.hybriq.adbuddy.ui.ProtectedAppsContent
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** Validation 27: the list of protected apps from Settings (with a change to save, and a search with no match). */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ProtectedAppsScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private fun text(id: Int, vararg args: Any): String = RuntimeEnvironment.getApplication().getString(id, *args)

    @Test fun fromSettingsWithAChangeToSave() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) { ProtectedAppsContent(null, false, false, {}, SAMPLE_ROWS, 2, false, "", {}, { _, _ -> }, true, {}, {}, {}) }
        }
        compose.onNodeWithContentDescription(text(R.string.action_back)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.step_progress, 1, 2)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.onboarding_skip)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.protected_proposal)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.protected_save)).assertIsDisplayed().assertIsEnabled()
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(text(R.string.protected_title), knownUiIssue(R.string.protected_title, mode))
        compose.shot("protected_apps_settings", mode)
        // A lazy list only composes the rows in view: scroll by key, then look.
        compose.onNode(hasScrollToKeyAction()).performScrollToKey("com.google.android.gm")
        compose.onNodeWithText("Gmail").assertIsDisplayed().assertIsOn()
        compose.onNodeWithText(text(R.string.protected_reason_codes)).assertIsDisplayed()
        compose.assertTextsFit(scrolled = true)
        compose.onNode(hasScrollToKeyAction()).performScrollToKey("com.whatsapp")
        compose.onNodeWithText("WhatsApp").assertIsDisplayed().assertIsOff()
        compose.assertTextsFit(scrolled = true)
        if (mode.fontScale > 1f) compose.shot("protected_apps_settings_rows", mode)
    }

    @Test fun aSearchWithNoMatchSaysSo() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) { ProtectedAppsContent(null, false, false, {}, emptyList(), 2, false, "zzz", {}, { _, _ -> }, false, {}, {}, {}) }
        }
        compose.onNodeWithText("zzz").assertIsDisplayed()
        compose.onNodeWithText(text(R.string.protected_empty)).performScrollTo().assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("protected_apps_no_match", mode)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
