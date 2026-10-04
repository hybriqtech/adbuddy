package tech.hybriq.adbuddy.ui

import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isToggleable
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertEquals
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
import tech.hybriq.adbuddy.bridge.RemoteState
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** O6/C14: the switch has a name for TalkBack and the whole row toggles it, still through the confirmation dialog. */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class RemoteCardSemanticsTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private fun text(id: Int): String = app.getString(id)

    private val connectedOff = BridgeState(connection = BridgeState.Connection.CONNECTED, remote = RemoteState(enabled = false))

    /** What the card reported through onMessage: with the bridge not connected, send() can only report that it failed. */
    private val messages = mutableListOf<String>()

    private fun show(state: BridgeState) {
        compose.setContent { AdbuddyTheme(dynamic = false) { RemoteCard(state, app.bridge, app.clock, onMessage = { messages += it }) } }
    }

    @Test fun theSwitchIsOneNodeNamedAfterTheCardAndIsASwitch() {
        show(connectedOff)
        val node = compose.onNode(isToggleable()).fetchSemanticsNode()
        assertEquals(Role.Switch, node.config.getOrNull(SemanticsProperties.Role))
        compose.onNode(isToggleable() and hasText(text(R.string.remote_title))).assertIsDisplayed()
    }

    @Test fun tappingTheTitleOnlyAsksAndSendingNeedsTheConfirmButton() {
        show(connectedOff)
        compose.onNodeWithText(text(R.string.remote_title)).performClick()
        compose.onNodeWithText(text(R.string.remote_enable_title)).assertIsDisplayed()
        assertTrue("asking sends nothing", messages.isEmpty())
        compose.onNodeWithText(text(R.string.remote_enable_confirm)).performClick()
        // The bridge of the test app is not connected: the attempt shows up as the failure notice, and only after the confirm.
        assertEquals(listOf(text(R.string.remote_failed)), messages)
    }

    @Test fun tappingTheTitleWhileOnAsksBeforeTurningOffAndSendsNothingUntilConfirmed() {
        show(BridgeState(connection = BridgeState.Connection.CONNECTED, remote = RemoteState(enabled = true, address = "192.0.2.1:5555")))
        compose.onNodeWithText(text(R.string.remote_title)).performClick()
        compose.onNodeWithText(text(R.string.remote_disable_title)).assertIsDisplayed()
        assertTrue("asking sends nothing", messages.isEmpty())
        compose.onNodeWithText(text(R.string.remote_disable_confirm)).performClick()
        assertEquals(listOf(text(R.string.remote_failed)), messages)
    }

    @Test fun cancellingTheDialogSendsNothing() {
        show(connectedOff)
        compose.onNodeWithText(text(R.string.remote_title)).performClick()
        compose.onNodeWithText(text(R.string.action_cancel)).performClick()
        compose.onNodeWithText(text(R.string.remote_enable_title)).assertDoesNotExist()
        assertTrue("cancelling sends nothing", messages.isEmpty())
    }
}
