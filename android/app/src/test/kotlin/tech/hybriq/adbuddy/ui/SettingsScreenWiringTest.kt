package tech.hybriq.adbuddy.ui

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
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
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme
import tech.hybriq.adbuddy.ui.theme.ThemePrefs

/** SettingsScreen wires the "wallpaper colors" switch to the app's flow and to the stored choice. */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class SettingsScreenWiringTest {
    private val compose = createAndroidComposeRule<ComponentActivity>()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    @Test fun theSwitchChangesTheAppsFlowAndTheStoredChoice() {
        assertFalse(app.dynamicColor.value)
        assertFalse(ThemePrefs(app).dynamic)
        compose.setContent {
            AdbuddyTheme(dynamic = false) { SettingsScreen(app, BridgeState(), onOpen = {}, onBack = {}, onMessage = {}) }
        }
        compose.onNodeWithText(app.getString(R.string.settings_dynamic_color)).performClick()
        compose.waitForIdle()
        assertTrue(app.dynamicColor.value)
        assertTrue(ThemePrefs(app).dynamic)
        compose.onNodeWithText(app.getString(R.string.settings_dynamic_color)).performClick()
        compose.waitForIdle()
        assertFalse(app.dynamicColor.value)
        assertFalse(ThemePrefs(app).dynamic)
    }
}
