package tech.hybriq.adbuddy.confirm

import androidx.activity.ComponentActivity
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performSemanticsAction
import androidx.lifecycle.Lifecycle
import java.time.Instant
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
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Decision
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.ConfirmHost
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * The 🔴 dialog's arming as ConfirmHost runs it, with Compose's clock in the test's hands: a tap on Allow in the first
 * ConfirmUi.ARMING_MS sends nothing and one after it does; Deny answers at once; another question that takes the place
 * waits again; and so does the dialog when the app comes back to the front, even for a tap that lands before the
 * screen is drawn again (the answer checks the arming at the tap, not as last drawn).
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ConfirmHostArmingTest {
    private val compose = createAndroidComposeRule<ComponentActivity>()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    /** The bridge's state; a reply marks the question as answering, as Bridge.reply does. */
    private var bridge by mutableStateOf(BridgeState())
    private val replies = mutableListOf<Pair<String, String>>()

    private fun request(id: String) = ConfirmRequest(
        id = id, title = "Restart the phone", rawCommand = "adb shell reboot",
        expiresAt = Instant.ofEpochMilli(app.clock.now()).plusSeconds(600).toString(), allowSession = true,
    )

    private fun pending(vararg r: ConfirmRequest) = BridgeState(connection = BridgeState.Connection.CONNECTED, pending = r.toList())

    private fun show(vararg r: ConfirmRequest) {
        bridge = pending(*r)
        compose.mainClock.autoAdvance = false
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ConfirmHost(
                    bridge, { bridge }, { id, decision -> replies += id to decision; bridge = bridge.answered(id) },
                    app.clock, focusConfirm = null, closed = emptySet(), onClose = {},
                )
            }
        }
        frame()
    }

    /** One frame: what changed is drawn (the state written from the test reaches the composition first). */
    private fun frame() {
        compose.waitForIdle()
        compose.mainClock.advanceTimeByFrame()
    }

    private fun advance(ms: Long) = compose.mainClock.advanceTimeBy(ms)

    private fun button(id: Int): SemanticsNodeInteraction = compose.onNodeWithText(app.getString(id))

    @Test fun allowWaitsSixHundredMilliseconds() {
        show(request("j_1"))
        button(R.string.action_allow).assertIsNotEnabled().performClick()
        button(R.string.action_allow_session).assertIsNotEnabled().performClick()
        advance(ConfirmUi.ARMING_MS - 50)
        button(R.string.action_allow).assertIsNotEnabled().performClick()
        assertEquals("no answer before the arming", emptyList<Pair<String, String>>(), replies)
        advance(200)
        button(R.string.action_allow).assertIsEnabled().performClick()
        assertEquals(listOf("j_1" to Decision.ALLOW), replies)
    }

    @Test fun denyAnswersAtOnce() {
        show(request("j_1"))
        button(R.string.action_deny).assertIsEnabled().performClick()
        assertEquals(listOf("j_1" to Decision.DENY), replies)
    }

    @Test fun anotherQuestionInItsPlaceWaitsAgain() {
        show(request("j_1"))
        advance(ConfirmUi.ARMING_MS + 100)
        button(R.string.action_allow).assertIsEnabled()
        bridge = pending(request("j_2"))
        frame()
        button(R.string.action_allow).assertIsNotEnabled().performClick()
        advance(ConfirmUi.ARMING_MS - 50)
        button(R.string.action_allow).performClick()
        assertEquals("the new question waits its own arming", emptyList<Pair<String, String>>(), replies)
        advance(200)
        button(R.string.action_allow).assertIsEnabled().performClick()
        assertEquals(listOf("j_2" to Decision.ALLOW), replies)
    }

    // The app back in front (the screen woke, the person came back from another app): a tap there is not an answer
    // to a question the person has not looked at again, even before the dialog is drawn again.
    @Test fun backToTheFrontItWaitsAgainEvenBeforeItIsDrawn() {
        show(request("j_1"))
        advance(ConfirmUi.ARMING_MS + 100)
        button(R.string.action_allow).assertIsEnabled()
        compose.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
        compose.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
        // No frame yet: the button is still drawn as on. Its click (as TalkBack sends it: a touch would let a frame
        // through between down and up) reaches the answer, which checks the arming itself.
        button(R.string.action_allow).assertIsEnabled().performSemanticsAction(SemanticsActions.OnClick)
        assertEquals("a tap as the app comes back answers nothing", emptyList<Pair<String, String>>(), replies)
        frame()
        button(R.string.action_allow).assertIsNotEnabled()
        advance(ConfirmUi.ARMING_MS + 100)
        button(R.string.action_allow).assertIsEnabled().performClick()
        assertEquals(listOf("j_1" to Decision.ALLOW), replies)
    }
}
