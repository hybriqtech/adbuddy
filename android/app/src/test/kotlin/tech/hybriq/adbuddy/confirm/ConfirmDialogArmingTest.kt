package tech.hybriq.adbuddy.confirm

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
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
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Decision
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.ConfirmDialog
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * The 🔴 dialog's arming drawn for real (Compose ui-test on Robolectric): until ConfirmHost says it is armed, Allow
 * and Allow during the session are off (with their text, so TalkBack reads them as disabled) and Deny is not; armed,
 * they turn on, unless the text has not been read to its end (ConfirmDialogScrollTest).
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ConfirmDialogArmingTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private val answers = mutableListOf<String>()

    private fun request(detail: String = "") = ConfirmRequest(
        id = "j_1", title = "Restart the phone", detail = detail, rawCommand = "adb shell reboot",
        expiresAt = Instant.ofEpochMilli(app.clock.now()).plusSeconds(600).toString(), allowSession = true,
    )

    private var armed by mutableStateOf(false)

    private fun show(r: ConfirmRequest) {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ConfirmDialog(r, ConfirmUi.Status.OPEN, app.clock, armed = armed, onClose = {}, onAnswer = { answers += it })
            }
        }
        compose.waitForIdle()
    }

    private fun button(id: Int): SemanticsNodeInteraction = compose.onNodeWithText(app.getString(id))

    @Test fun beforeTheArmingOnlyDenyIsOn() {
        show(request())
        button(R.string.action_allow).assertIsNotEnabled()
        button(R.string.action_allow_session).assertIsNotEnabled()
        button(R.string.action_allow).performClick()
        button(R.string.action_allow_session).performClick()
        assertEquals("a tap on Allow before the arming answers nothing", emptyList<String>(), answers)
        button(R.string.action_deny).assertIsEnabled().performClick()
        assertEquals(listOf(Decision.DENY), answers)
    }

    @Test fun armedTheAllowButtonsTurnOn() {
        show(request())
        button(R.string.action_allow).assertIsNotEnabled()
        armed = true
        compose.waitForIdle()
        button(R.string.action_allow).assertIsEnabled()
        button(R.string.action_allow_session).assertIsEnabled().performClick()
        assertEquals(listOf(Decision.ALLOW_SESSION), answers)
    }

    @Test fun armedButUnreadTheyStayOff() {
        armed = true
        show(request(detail = (1..60).joinToString("\n") { "Line $it of what the rule says about this command" }))
        button(R.string.action_allow).assertIsNotEnabled()
        button(R.string.action_allow_session).assertIsNotEnabled()
        button(R.string.action_deny).assertIsEnabled()
    }
}
