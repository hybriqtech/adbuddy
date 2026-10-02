package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performScrollTo
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.listener.NotificationAccess.Effective
import tech.hybriq.adbuddy.ui.NotificationAccessContent
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * Validation 27: the notification access screen from Settings in each of its four states (the validation says three:
 * it was written before ruling 15 of M5c added "allowed in the system, not confirmed here") and while it is checking.
 */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class NotificationAccessScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private fun text(id: Int): String = RuntimeEnvironment.getApplication().getString(id)

    private fun show(effective: Effective, confirmed: Boolean, checking: Boolean = false) {
        compose.setContent { AdbuddyTheme(dynamic = false) { NotificationAccessContent(null, checking, confirmed, effective, {}, {}, {}, {}, {}, {}) } }
    }

    /** The state's title and detail, the buttons it shows and the ones it must not; then the texts and the image. */
    private fun check(name: String, title: Int, detail: Int?, shown: List<Int>, hidden: List<Int>) {
        compose.onNodeWithText(text(title)).assertIsDisplayed()
        if (detail != null) compose.onNodeWithText(text(detail)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(text(R.string.access_title), knownUiIssue(R.string.access_title, mode))
        compose.shot(name, mode)
        var scrolled = false
        for (id in shown) {
            val node = compose.onNodeWithText(text(id))
            val before = node.fetchSemanticsNode().positionInRoot.y
            node.performScrollTo().assertIsDisplayed()
            if (node.fetchSemanticsNode().positionInRoot.y != before) scrolled = true
        }
        for (id in hidden) compose.onNodeWithText(text(id)).assertDoesNotExist()
        // What was below the fold: whole, and on record when the screen did scroll (otherwise it is the same image).
        compose.assertTextsFit(scrolled = true)
        if (scrolled) compose.shot("${name}_below", mode)
    }

    @Test fun allowed() {
        show(Effective.ON, confirmed = true)
        check(
            "access_allowed", R.string.access_state_granted, R.string.access_detail_granted,
            shown = listOf(R.string.access_withdraw, R.string.access_open),
            hidden = listOf(R.string.access_confirm, R.string.access_restricted_hint),
        )
    }

    @Test fun allowedInTheSystemNotConfirmedHere() {
        show(Effective.NEEDS_CONFIRMATION, confirmed = false)
        check(
            "access_needs_confirmation", R.string.access_state_confirm, R.string.access_detail_confirm,
            shown = listOf(R.string.access_confirm, R.string.access_open),
            hidden = listOf(R.string.access_withdraw, R.string.access_restricted_hint),
        )
    }

    @Test fun notAllowed() {
        show(Effective.OFF, confirmed = false)
        check(
            "access_not_allowed", R.string.access_state_denied, R.string.access_detail_denied,
            shown = listOf(R.string.access_open, R.string.access_restricted_hint),
            hidden = listOf(R.string.access_confirm, R.string.access_withdraw),
        )
    }

    @Test fun unknown() {
        show(Effective.UNKNOWN, confirmed = false)
        check(
            "access_unknown", R.string.access_state_unknown, R.string.access_detail_unknown,
            shown = listOf(R.string.access_open, R.string.access_restricted_hint),
            hidden = listOf(R.string.access_confirm, R.string.access_withdraw),
        )
    }

    @Test fun unknownWithTheMarkStillOffersToWithdrawIt() {
        // Withdrawing only takes permission away: it shows whenever the mark is there, even with the state unknown.
        show(Effective.UNKNOWN, confirmed = true)
        compose.onNodeWithText(text(R.string.access_state_unknown)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_withdraw)).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_confirm)).assertDoesNotExist()
        compose.assertTextsFit(scrolled = true)
    }

    @Test fun checking() {
        show(Effective.UNKNOWN, confirmed = false, checking = true)
        check(
            "access_checking", R.string.access_state_checking, null,
            shown = listOf(R.string.access_open),
            hidden = listOf(R.string.access_state_unknown, R.string.access_confirm, R.string.access_withdraw, R.string.access_restricted_hint),
        )
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
