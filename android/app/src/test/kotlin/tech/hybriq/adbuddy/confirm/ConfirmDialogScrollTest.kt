package tech.hybriq.adbuddy.confirm

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeUp
import java.time.Instant
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
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Decision
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.ConfirmDialog
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * The 🔴 dialog drawn for real (Compose ui-test on Robolectric), in place of the source check it had (B3 of the review
 * of tern): Allow and Allow during the session stay off until the text has been scrolled to its end, and the gate comes
 * from the same ScrollState the text scrolls with (a swipe on the text turns them on); Deny never waits; a command longer
 * than the dialog shows (H3) offers no Allow at all and says how many characters it hides. Validation A4-1 (the dialog's
 * part); the notification's part is ConfirmNotificationTest.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ConfirmDialogScrollTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private fun text(id: Int): String = app.getString(id)

    private val answers = mutableListOf<String>()

    private fun request(detail: String = "", raw: String = "adb shell reboot", special: Boolean = false, allowSession: Boolean = false) =
        ConfirmRequest(
            id = "j_1", title = "Restart the phone", detail = detail, rawCommand = raw,
            expiresAt = Instant.ofEpochMilli(app.clock.now()).plusSeconds(600).toString(), special = special, allowSession = allowSession,
        )

    private fun show(r: ConfirmRequest) {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ConfirmDialog(r, ConfirmUi.Status.OPEN, app.clock, onClose = {}, onAnswer = { answers += it })
            }
        }
        compose.waitForIdle()
    }

    /** A rule of 60 lines: far taller than the dialog. */
    private val longDetail = (1..60).joinToString("\n") { "Line $it of what the rule says about this command" }

    private fun button(id: Int): SemanticsNodeInteraction = compose.onNodeWithText(text(id))

    private fun scrollable(): SemanticsNodeInteraction = compose.onNode(hasScrollAction())

    private fun atTheEnd(): Boolean {
        val range = scrollable().fetchSemanticsNode().config.getOrNull(SemanticsProperties.VerticalScrollAxisRange)!!
        return range.value() >= range.maxValue()
    }

    /** Swipes the text up, as a finger would, until it can scroll no further. */
    private fun scrollToTheEnd() {
        repeat(200) {
            if (atTheEnd()) return
            scrollable().performTouchInput { swipeUp() }
            compose.waitForIdle()
        }
        assertTrue("the text never reached its end", atTheEnd())
    }

    @Test fun theAllowButtonsWaitForTheEndOfTheText() {
        show(request(detail = longDetail, allowSession = true))
        assertTrue("the text must be taller than the dialog for this test", !atTheEnd())
        button(R.string.action_allow).assertIsNotEnabled()
        button(R.string.action_allow_session).assertIsNotEnabled()
        button(R.string.action_deny).assertIsEnabled()
        compose.onNodeWithText(text(R.string.confirm_scroll)).assertIsDisplayed()
        button(R.string.action_allow).performClick()
        button(R.string.action_allow_session).performClick()
        assertEquals("a tap on a button that is off answers nothing", emptyList<String>(), answers)

        // Half way down it still waits.
        scrollable().performTouchInput { swipeUp() }
        compose.waitForIdle()
        if (!atTheEnd()) button(R.string.action_allow).assertIsNotEnabled()

        scrollToTheEnd()
        button(R.string.action_allow).assertIsEnabled()
        button(R.string.action_allow_session).assertIsEnabled()
        compose.onNodeWithText(text(R.string.confirm_scroll)).assertDoesNotExist()
        button(R.string.action_allow).performClick()
        assertEquals(listOf(Decision.ALLOW), answers)
    }

    @Test fun denyNeverWaits() {
        show(request(detail = longDetail))
        button(R.string.action_deny).assertIsEnabled().performClick()
        assertEquals(listOf(Decision.DENY), answers)
    }

    @Test fun aShortQuestionCanBeAllowedAtOnce() {
        show(request())
        assertTrue("a short question fits", atTheEndOrNotScrollable())
        button(R.string.action_allow).assertIsEnabled()
        compose.onNodeWithText(text(R.string.confirm_scroll)).assertDoesNotExist()
    }

    private fun atTheEndOrNotScrollable(): Boolean =
        compose.onAllNodes(hasScrollAction()).fetchSemanticsNodes().isEmpty() || atTheEnd()

    // A command the notification cuts (> 300) is allowed only here, and only once read to its end.
    @Test fun aCutCommandWaitsForItsEndToo() {
        show(request(raw = "adb shell echo " + "a".repeat(6_000)))
        button(R.string.action_allow).assertIsNotEnabled()
        scrollToTheEnd()
        button(R.string.action_allow).assertIsEnabled()
    }

    // A special question: its ⚠ comes first, above the rule; it is allowed here once read.
    @Test fun aSpecialQuestionShowsItsWarningFirst() {
        show(request(detail = longDetail, special = true))
        val warning = compose.onNodeWithText("⚠ " + text(R.string.confirm_special)).fetchSemanticsNode()
        val firstLine = compose.onNodeWithText("Line 1 of what the rule says about this command").fetchSemanticsNode()
        assertTrue("the ⚠ is above the rule", warning.positionInRoot.y < firstLine.positionInRoot.y)
        button(R.string.action_allow).assertIsNotEnabled()
        scrollToTheEnd()
        button(R.string.action_allow).assertIsEnabled()
    }

    // H3: more than the dialog shows: no Allow at all, even read to the end, and it says how many characters it hides.
    @Test fun aHiddenCommandCanOnlyBeDenied() {
        val r = request(raw = "adb shell echo " + "b".repeat(ConfirmUi.DIALOG_MAX_COMMAND), allowSession = true)
        val hidden = ConfirmUi.shown(r).hidden
        assertTrue(hidden > 0)
        show(r)
        compose.onNodeWithText(app.resources.getQuantityString(R.plurals.confirm_hidden, hidden, hidden), substring = true).assertExists()
        compose.onNodeWithText(text(R.string.confirm_too_long), substring = true).assertExists()
        button(R.string.action_allow).assertDoesNotExist()
        button(R.string.action_allow_session).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.confirm_scroll)).assertDoesNotExist()
        scrollToTheEnd()
        button(R.string.action_allow).assertDoesNotExist()
        button(R.string.action_deny).assertIsEnabled().performClick()
        assertEquals(listOf(Decision.DENY), answers)
    }
}
