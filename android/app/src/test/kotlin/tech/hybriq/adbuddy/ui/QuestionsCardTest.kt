package tech.hybriq.adbuddy.ui

import androidx.activity.ComponentActivity
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertEquals
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
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The card for closed red questions (ActionCard QUESTIONS): says how many, "Show" reopens, and nothing in it answers a question. */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class QuestionsCardTest {
    private val compose = createAndroidComposeRule<ComponentActivity>()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private fun show(count: Int, onShow: () -> Unit = {}) =
        compose.setContent { AdbuddyTheme(dynamic = false) { ActionCard(HomeNotice.QUESTIONS, count, onShow) } }

    @Test fun twoQuestionsSayTheirCountAndOfferShow() {
        show(2)
        compose.onNodeWithText("2 delicate actions are waiting for your answer").assertIsDisplayed()
        compose.onNodeWithText(app.getString(R.string.confirm_waiting_show)).assertIsDisplayed()
    }

    @Test fun oneQuestionUsesTheSingular() {
        show(1)
        compose.onNodeWithText("1 delicate action is waiting for your answer").assertIsDisplayed()
    }

    @Test fun showCallsOnShowOnce() {
        var shown = 0
        show(1) { shown++ }
        compose.onNodeWithText(app.getString(R.string.confirm_waiting_show)).performClick()
        assertEquals(1, shown)
    }

    @Test fun theTextIsAnnouncedPolitely() {
        show(1)
        compose.onNodeWithText("1 delicate action is waiting for your answer")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Polite))
    }

    @Test fun nothingInItAnswersTheQuestion() {
        show(2)
        for (id in listOf(R.string.action_allow, R.string.action_allow_session, R.string.action_deny)) {
            compose.onNodeWithText(app.getString(id)).assertDoesNotExist()
        }
    }
}
