package tech.hybriq.adbuddy.confirm

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import java.time.Instant
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
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.ConfirmDialog
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The fixed lines of the 🔴 dialog (the countdown and the scroll notice) are on screen before any scrolling. */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ConfirmDialogLayoutTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private val t0 get() = app.clock.now()

    private fun request(raw: String = "adb shell reboot") =
        ConfirmRequest(
            id = "j_1", title = "Restart the phone", detail = "", rawCommand = raw,
            expiresAt = Instant.ofEpochMilli(t0).plusSeconds(600).toString(),
        )

    private fun show(r: ConfirmRequest, status: ConfirmUi.Status = ConfirmUi.Status.OPEN, now: Long = t0) {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ConfirmDialog(r, status, app.clock, armed = true, now = now, onClose = {}, onAnswer = {})
            }
        }
        compose.waitForIdle()
    }

    private fun expiresIn(s: String) = app.getString(R.string.confirm_expires_in, s)

    @Test fun theFixedLinesAreVisibleBeforeScrollingALongCommand() {
        val words = (1..1200).joinToString(" ") { "word$it" }
        show(request("adb shell echo $words"))
        compose.onNodeWithText(app.getString(R.string.confirm_scroll)).assertIsDisplayed()
        compose.onNodeWithText(expiresIn("10:00")).assertIsDisplayed()
    }

    @Test fun itSaysHowLongIsLeft() {
        val r = request()
        show(r, now = ConfirmUi.expiry(r)!! - 42_000)
        compose.onNodeWithText(expiresIn("0:42")).assertIsDisplayed()
    }

    @Test fun underTenSecondsItStillCounts() {
        val r = request()
        show(r, now = ConfirmUi.expiry(r)!! - 9_000)
        compose.onNodeWithText(expiresIn("0:09")).assertIsDisplayed()
    }

    @Test fun withoutAnOpenQuestionThereIsNoCountdown() {
        val r = request()
        show(r, status = ConfirmUi.Status.OFFLINE, now = ConfirmUi.expiry(r)!! - 42_000)
        compose.onNodeWithText(expiresIn("0:42")).assertDoesNotExist()
    }
}
