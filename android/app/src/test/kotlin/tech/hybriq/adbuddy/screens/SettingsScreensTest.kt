package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.isToggleable
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
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
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.RemoteState
import tech.hybriq.adbuddy.listener.NotificationAccess.Effective
import tech.hybriq.adbuddy.ui.RemoteCard
import tech.hybriq.adbuddy.ui.SettingsContent
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** Validation 27: Settings (its rows, the "proposed" warning and the remote-mode card), in every ScreenMode. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class SettingsScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    private fun show(
        count: Int, confirmed: Boolean, access: Effective, state: BridgeState,
        languageAvailable: Boolean = false, onLanguage: () -> Unit = {}, dynamicColor: Boolean = false, onDynamicColor: (Boolean) -> Unit = {},
    ) {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                SettingsContent(
                    count, confirmed, access, onOpen = {}, onBack = {}, remoteCard = { RemoteCard(state, app.bridge, app.clock, onMessage = {}) },
                    languageAvailable = languageAvailable, onLanguage = onLanguage, dynamicColor = dynamicColor, onDynamicColor = onDynamicColor,
                )
            }
        }
    }

    @Test fun aProposedListAndAnAccessToConfirmAreFlagged() {
        show(12, false, Effective.NEEDS_CONFIRMATION, BridgeState(connection = BridgeState.Connection.CONNECTED, remote = RemoteState(enabled = false)))
        compose.onNodeWithText(text(R.string.settings_protected_proposed, 12)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_state_confirm)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.remote_title)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.remote_off)).assertIsDisplayed()
        // The remote-mode switch and the wallpaper-colors switch of the App section.
        compose.onAllNodes(isToggleable()).assertCountEquals(2)
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(text(R.string.settings_title), knownUiIssue(R.string.settings_title, mode))
        compose.shot("settings_proposed", mode)
    }

    @Test fun aConfirmedListAndTheAccessOnWithThePcAway() {
        show(3, true, Effective.ON, BridgeState())
        compose.onNodeWithText(text(R.string.settings_protected_confirmed, 3)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_state_granted)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.remote_disconnected)).assertIsDisplayed()
        // Not connected: no remote switch, which would read as "off"; only the wallpaper-colors one remains.
        compose.onAllNodes(isToggleable()).assertCountEquals(1)
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(text(R.string.settings_title), knownUiIssue(R.string.settings_title, mode))
        compose.shot("settings_confirmed", mode)
    }

    @Test fun theSectionsAreNamedAndTheLanguageRowAndTheColorsSwitchWork() {
        var language = 0
        val changes = mutableListOf<Boolean>()
        show(
            3, true, Effective.ON, BridgeState(connection = BridgeState.Connection.CONNECTED, remote = RemoteState(enabled = false)),
            languageAvailable = true, onLanguage = { language++ }, dynamicColor = false, onDynamicColor = { changes += it },
        )
        compose.onNodeWithText(text(R.string.settings_section_security)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.settings_section_app)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.settings_language)).assertIsDisplayed().performClick()
        assertEquals(1, language)
        compose.onNodeWithText(text(R.string.settings_dynamic_color)).assertIsDisplayed().performClick()
        assertEquals(listOf(true), changes)
        compose.assertTextsFit()
        compose.shot("settings_app_section", mode)
    }

    @Test fun withoutTheAppsOwnLanguageThereIsNoLanguageRow() {
        show(3, true, Effective.ON, BridgeState(), languageAvailable = false)
        compose.onNodeWithText(text(R.string.settings_language)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.settings_dynamic_color)).assertIsDisplayed()
    }

    @Test fun theColorsSwitchShowsItsState() {
        show(3, true, Effective.ON, BridgeState(), dynamicColor = true)
        compose.onNodeWithText(text(R.string.settings_dynamic_color)).assertIsDisplayed()
        compose.shot("settings_dynamic_on", mode)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
