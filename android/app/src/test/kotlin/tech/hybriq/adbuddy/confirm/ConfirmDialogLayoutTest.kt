package tech.hybriq.adbuddy.confirm

import androidx.compose.material3.MaterialTheme
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.layout.boundsInRoot
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.performClick
import androidx.compose.ui.unit.dp
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
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
import tech.hybriq.adbuddy.screens.ScreenMode
import tech.hybriq.adbuddy.screens.ScreenModeRule
import tech.hybriq.adbuddy.screens.screenModes
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

    private var errorColor = Color.Unspecified

    private fun show(r: ConfirmRequest, status: ConfirmUi.Status = ConfirmUi.Status.OPEN, now: Long = t0) {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                errorColor = MaterialTheme.colorScheme.error
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

    private fun countdownNode(s: String) = compose.onNodeWithText(expiresIn(s)).fetchSemanticsNode()

    private fun countdownLayout(s: String): TextLayoutResult {
        val results = mutableListOf<TextLayoutResult>()
        countdownNode(s).config[SemanticsActions.GetTextLayoutResult].action!!(results)
        return results.first()
    }

    @Test fun underTenSecondsItIsBoldInTheErrorColor() {
        val r = request()
        show(r, now = ConfirmUi.expiry(r)!! - 9_000)
        val style = countdownLayout("0:09").layoutInput.style
        assertEquals(errorColor, style.color)
        assertEquals(FontWeight.Bold, style.fontWeight)
    }

    @Test fun atTenSecondsItIsNotYetInTheErrorColor() {
        val r = request()
        show(r, now = ConfirmUi.expiry(r)!! - 10_000)
        val style = countdownLayout("0:10").layoutInput.style
        assertNotEquals(errorColor, style.color)
        assertNotEquals(FontWeight.Bold, style.fontWeight)
    }

    @Test fun theCountdownIsNotALiveRegion() {
        val r = request()
        show(r, now = ConfirmUi.expiry(r)!! - 9_000)
        assertNull(countdownNode("0:09").config.getOrNull(SemanticsProperties.LiveRegion))
    }
}

/**
 * The three buttons of the red dialog in every ScreenMode (light, dark, 200 % text in both languages): Deny, the filled
 * one, is on at once, first in the reading order and at least 48 dp tall; Allow during the session is below Allow with
 * room between them; no two touch areas overlap (a tap meant for Deny cannot reach an Allow); and the scroll notice is
 * in the body, not in a button. The dialog is its own window: no image, the numbers are the check.
 */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ConfirmDialogButtonsTest(private val mode: ScreenMode) {
    companion object {
        @JvmStatic @ParameterizedRobolectricTestRunner.Parameters(name = "{0}") fun modes() = screenModes()
    }

    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private val answers = mutableListOf<String>()
    private var armed by mutableStateOf(false)

    private fun request(raw: String = "adb shell reboot") = ConfirmRequest(
        id = "j_1", title = "Restart the phone", detail = "", rawCommand = raw, allowSession = true,
        expiresAt = Instant.ofEpochMilli(app.clock.now()).plusSeconds(600).toString(),
    )

    private fun show(r: ConfirmRequest = request()) {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ConfirmDialog(r, ConfirmUi.Status.OPEN, app.clock, armed = armed, onClose = {}, onAnswer = { answers += it })
            }
        }
        compose.waitForIdle()
    }

    private fun button(id: Int): SemanticsNodeInteraction = compose.onNodeWithText(app.getString(id))
    private fun bounds(id: Int): Rect = button(id).fetchSemanticsNode().boundsInRoot

    @Test fun denyIsOnFromTheFirstFrameAndLeadsTheReadingOrder() {
        show()
        button(R.string.action_deny).assertIsEnabled().assertHeightIsAtLeast(48.dp)
        button(R.string.action_allow).assertIsNotEnabled()
        button(R.string.action_allow_session).assertIsNotEnabled()
        val deny = bounds(R.string.action_deny)
        val allow = bounds(R.string.action_allow)
        assertTrue("Deny is first: left of Allow, or above it", deny.right <= allow.left || deny.bottom <= allow.top)
        assertTrue("Deny is at least 48 dp tall", deny.height >= 48f * app.resources.displayMetrics.density - 1f)
    }

    /** The touch area of a button: its bounds grown to the 48 dp minimum Material gives every button. */
    private fun touchArea(id: Int): Rect {
        val r = bounds(id)
        val min = 48f * app.resources.displayMetrics.density
        val grow = ((min - r.height) / 2f).coerceAtLeast(0f)
        return Rect(r.left, r.top - grow, r.right, r.bottom + grow)
    }

    private val eightDp get() = 8f * app.resources.displayMetrics.density

    @Test fun eachButtonIsBelowTheOtherWithEightDpBetweenTheirTouchAreas() {
        show()
        val deny = touchArea(R.string.action_deny)
        val allow = touchArea(R.string.action_allow)
        val session = touchArea(R.string.action_allow_session)
        assertTrue("Allow below Deny: $deny $allow", allow.top >= deny.bottom)
        assertTrue("8 dp between Deny and Allow: ${allow.top - deny.bottom}", allow.top - deny.bottom >= eightDp - 1f)
        assertTrue("session below Allow: $allow $session", session.top >= allow.bottom)
        assertTrue("8 dp between Allow and the session one: ${session.top - allow.bottom}", session.top - allow.bottom >= eightDp - 1f)
    }

    @Test fun noTouchAreaOverlapsAnother() {
        show()
        val all = listOf(R.string.action_deny, R.string.action_allow, R.string.action_allow_session).map { touchArea(it) }
        for (i in all.indices) for (j in i + 1 until all.size) {
            assertFalse("touch areas ${all[i]} and ${all[j]} overlap", all[i].overlaps(all[j]))
        }
    }

    @Test fun aTapOnAnAllowAnswersNothingWhileDisarmedAndDenyAnswersDeny() {
        show()
        button(R.string.action_allow).performClick()
        button(R.string.action_allow_session).performClick()
        assertEquals(emptyList<String>(), answers)
        button(R.string.action_deny).performClick()
        assertEquals(listOf(Decision.DENY), answers)
    }

    @Test fun theScrollNoticeIsInTheBodyWhileThereIsUnreadText() {
        val words = (1..1200).joinToString(" ") { "word$it" }
        show(request("adb shell echo $words"))
        val notice = compose.onNodeWithText(app.getString(R.string.confirm_scroll))
        notice.assertIsDisplayed()
        var p = notice.fetchSemanticsNode().parent
        while (p != null) {
            assertFalse("the notice is inside a button", p.config.contains(SemanticsActions.OnClick))
            p = p.parent
        }
        assertTrue(notice.fetchSemanticsNode().boundsInRoot.bottom <= bounds(R.string.action_allow).top + 1f)
    }

    @Test fun withNothingLeftToReadThereIsNoScrollNotice() {
        show()
        compose.onNodeWithText(app.getString(R.string.confirm_scroll)).assertDoesNotExist()
    }

    @Test fun theLabelSaysItsScope() {
        val expected = if (mode == ScreenMode.FONT_200_ES) "Permitir este mismo comando durante la sesión"
        else "Allow this same command during the session"
        assertEquals(expected, app.getString(R.string.action_allow_session))
    }
}
