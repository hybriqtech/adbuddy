package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
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
import tech.hybriq.adbuddy.confirm.ConfirmUi
import tech.hybriq.adbuddy.ui.YoloNoLockDialog
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The warning of a phone with no screen lock (R20): what it says, and that its button is armed before it sends anything. */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class YoloNoLockDialogTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private fun text(id: Int): String = app.getString(id)

    private var elapsed = 10_000L
    private var confirmed = 0
    private var dismissed = 0

    private fun show() {
        compose.mainClock.autoAdvance = false // the arming wait ends when the test says so
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                YoloNoLockDialog(onConfirm = { confirmed++ }, onDismiss = { dismissed++ }, elapsed = { elapsed })
            }
        }
        compose.mainClock.advanceTimeBy(50)
    }

    private fun arm() {
        repeat(2) { // twice: the dialog arms again when the activity resumes
            elapsed += ConfirmUi.ARMING_MS
            compose.mainClock.advanceTimeBy(ConfirmUi.ARMING_MS + 100, ignoreFrameDuration = true)
            compose.mainClock.advanceTimeBy(50)
        }
    }

    @Test fun itSaysWhatTheRiskIs() {
        show()
        compose.onNodeWithText(text(R.string.yolo_nolock_title)).assertExists()
        compose.onNodeWithText(text(R.string.yolo_nolock_body)).assertExists()
    }

    // Mutation: `enabled = true` on the button, or drop the check in its onClick.
    @Test fun nothingIsConfirmedUntilTheButtonIsArmed() {
        show()
        compose.onNodeWithText(text(R.string.yolo_nolock_confirm)).assertIsNotEnabled().performClick()
        assertEquals(0, confirmed)
        arm()
        compose.onNodeWithText(text(R.string.yolo_nolock_confirm)).assertIsEnabled().performClick()
        assertEquals(1, confirmed)
        assertEquals(0, dismissed)
    }

    @Test fun cancelDismissesAndConfirmsNothing() {
        show()
        arm()
        compose.onNodeWithText(text(R.string.action_cancel)).performClick()
        assertEquals(1, dismissed)
        assertEquals(0, confirmed)
    }
}
