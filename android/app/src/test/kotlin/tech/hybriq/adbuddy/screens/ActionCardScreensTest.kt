package tech.hybriq.adbuddy.screens

import androidx.compose.foundation.layout.Column
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasClickAction
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
import tech.hybriq.adbuddy.ui.ActionCard
import tech.hybriq.adbuddy.ui.HomeNotice
import tech.hybriq.adbuddy.ui.LinkStatus
import tech.hybriq.adbuddy.ui.StatusPill
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** Every card of HomeNotice in every ScreenMode: whole text, only the cards with an action have a button, captures. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ActionCardScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private fun title(n: HomeNotice): String = when (n) {
        HomeNotice.QUESTIONS -> app.resources.getQuantityString(R.plurals.confirm_waiting, 2, 2)
        HomeNotice.CUT -> app.getString(R.string.card_cut_title)
        HomeNotice.UPDATE_APP -> app.getString(R.string.card_update_app_title)
        HomeNotice.UPDATE_HOST -> app.getString(R.string.card_update_host_title)
        HomeNotice.NOT_SET_UP -> app.getString(R.string.card_not_set_up_title)
        HomeNotice.NOTIFICATIONS -> app.getString(R.string.card_notifications_title)
        HomeNotice.PAUSED -> app.getString(R.string.card_paused_title)
        HomeNotice.WAITING -> app.getString(R.string.card_waiting_title)
    }

    private fun check(n: HomeNotice) {
        // The pill says CONNECTED on purpose: its text matches no card title.
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                Column {
                    StatusPill(LinkStatus.CONNECTED, "my-pc", remote = false)
                    ActionCard(n, 2, {})
                }
            }
        }
        compose.onNodeWithText(title(n)).assertIsDisplayed()
        compose.onAllNodes(hasClickAction()).assertCountEquals(if (n.hasAction) 1 else 0)
        compose.assertTextsFit()
        compose.shot("action_card_${n.name.lowercase()}", mode)
    }

    @Test fun questionsCard() = check(HomeNotice.QUESTIONS)
    @Test fun cutCard() = check(HomeNotice.CUT)
    @Test fun updateAppCard() = check(HomeNotice.UPDATE_APP)
    @Test fun updateHostCard() = check(HomeNotice.UPDATE_HOST)
    @Test fun notSetUpCard() = check(HomeNotice.NOT_SET_UP)
    @Test fun notificationsCard() = check(HomeNotice.NOTIFICATIONS)
    @Test fun pausedCard() = check(HomeNotice.PAUSED)
    @Test fun waitingCard() = check(HomeNotice.WAITING)

    @Test fun thePillSaysRemoteWhenRemoteIsOn() {
        compose.setContent { AdbuddyTheme(dynamic = false) { StatusPill(LinkStatus.CONNECTED, "my-pc", remote = true) } }
        val expected = app.getString(R.string.status_pill_remote, app.getString(R.string.status_connected, "my-pc"))
        compose.onNodeWithText(expected).assertIsDisplayed()
        compose.assertTextsFit()
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
