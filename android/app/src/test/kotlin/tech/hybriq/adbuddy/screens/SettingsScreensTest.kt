package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.isToggleable
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

    private fun show(count: Int, confirmed: Boolean, access: Effective, state: BridgeState) {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                SettingsContent(count, confirmed, access, onOpen = {}, onBack = {}, remoteCard = { RemoteCard(state, app.bridge, app.clock, onMessage = {}) })
            }
        }
    }

    @Test fun aProposedListAndAnAccessToConfirmAreFlagged() {
        show(12, false, Effective.NEEDS_CONFIRMATION, BridgeState(connection = BridgeState.Connection.CONNECTED, remote = RemoteState(enabled = false)))
        compose.onNodeWithText(text(R.string.settings_protected_proposed, 12)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_state_confirm)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.remote_title)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.remote_off)).assertIsDisplayed()
        compose.onNode(isToggleable()).assertIsDisplayed()
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(text(R.string.settings_title), knownUiIssue(R.string.settings_title, mode))
        compose.shot("settings_proposed", mode)
    }

    @Test fun aConfirmedListAndTheAccessOnWithThePcAway() {
        show(3, true, Effective.ON, BridgeState())
        compose.onNodeWithText(text(R.string.settings_protected_confirmed, 3)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_state_granted)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.remote_disconnected)).assertIsDisplayed()
        // Not connected: no switch, which would read as "off".
        compose.onNode(isToggleable()).assertDoesNotExist()
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(text(R.string.settings_title), knownUiIssue(R.string.settings_title, mode))
        compose.shot("settings_confirmed", mode)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
