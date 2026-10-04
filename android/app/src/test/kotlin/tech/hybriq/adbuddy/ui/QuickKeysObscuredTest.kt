package tech.hybriq.adbuddy.ui

import android.os.SystemClock
import android.view.InputDevice
import android.view.MotionEvent
import android.view.MotionEvent.FLAG_WINDOW_IS_OBSCURED
import android.view.MotionEvent.FLAG_WINDOW_IS_PARTIALLY_OBSCURED
import android.view.View
import androidx.compose.runtime.Composable
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performScrollTo
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.term.QuickKey
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * Tapjacking of the terminal's quick keys: a tap that reaches "1" while another app's window covers it would answer
 * "1. Yes" to the agent's permission prompt on the PC. A window flag cannot be used (it would block the whole chat
 * under any harmless overlay), so each key refuses the gestures whose MotionEvents carry FLAG_WINDOW_IS_OBSCURED or
 * FLAG_WINDOW_IS_PARTIALLY_OBSCURED, and the rule is the safest one: one covered event anywhere in the gesture
 * refuses the whole gesture.
 *
 * ui-test's performTouchInput builds its MotionEvents itself and has no way to set their flags, so the events are
 * built here with the public MotionEvent.obtain overload that takes them and sent to the Compose view with
 * dispatchTouchEvent, the way ui-test's own input dispatcher sends them (screen coordinates, moved to the view's).
 * Every refusal is checked next to a plain tap on the same key that does go through, so the test cannot pass by
 * missing the key.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class QuickKeysObscuredTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val pressed = ArrayList<QuickKey>()
    private var view: View? = null

    @Composable
    private fun Probe() {
        view = LocalView.current
    }

    private fun show() {
        compose.setContent { AdbuddyTheme(dynamic = false) { Probe(); QuickKeysRow { pressed += it } } }
        compose.waitForIdle()
    }

    /** The row scrolls sideways and 1, 2, 3 start off screen: the key is scrolled into view first, as a finger would. */
    private fun centerOf(label: String): Offset {
        compose.onNodeWithText(label).performScrollTo().assertIsDisplayed()
        return compose.onNodeWithText(label).fetchSemanticsNode().boundsInRoot.center
    }

    private fun event(downTime: Long, action: Int, at: Offset, flags: Int): MotionEvent {
        val v = view!!
        val loc = IntArray(2).also { v.getLocationOnScreen(it) }
        val props = MotionEvent.PointerProperties().apply { id = 0; toolType = MotionEvent.TOOL_TYPE_FINGER }
        val coords = MotionEvent.PointerCoords().apply { x = at.x + loc[0]; y = at.y + loc[1]; pressure = 1f; size = 1f }
        val e = MotionEvent.obtain(
            downTime, SystemClock.uptimeMillis(), action, 1, arrayOf(props), arrayOf(coords),
            0, 0, 1f, 1f, 0, 0, InputDevice.SOURCE_TOUCHSCREEN, flags,
        )
        e.offsetLocation(-loc[0].toFloat(), -loc[1].toFloat())
        return e
    }

    /** A tap on [label]: the down with [downFlags], the up with [upFlags]. */
    private fun tap(label: String, downFlags: Int = 0, upFlags: Int = 0, moveFlags: Int? = null) {
        val at = centerOf(label)
        val downTime = SystemClock.uptimeMillis()
        compose.runOnUiThread {
            val v = view!!
            val events = buildList {
                add(event(downTime, MotionEvent.ACTION_DOWN, at, downFlags))
                // Moved a little (well under the touch slop): Compose does not deliver a move that changes nothing.
                if (moveFlags != null) add(event(downTime, MotionEvent.ACTION_MOVE, at + Offset(2f, 0f), moveFlags))
                add(event(downTime, MotionEvent.ACTION_UP, at, upFlags))
            }
            for (e in events) {
                v.dispatchTouchEvent(e)
                e.recycle()
            }
        }
        compose.waitForIdle()
    }

    @Test fun aPlainTapSendsTheKey() {
        show()
        tap("1")
        tap("Enter")
        assertEquals(listOf(QuickKey.ONE, QuickKey.ENTER), pressed)
    }

    @Test fun aWhollyCoveredTapIsRefused() {
        show()
        tap("1", downFlags = FLAG_WINDOW_IS_OBSCURED, upFlags = FLAG_WINDOW_IS_OBSCURED)
        assertEquals(emptyList<QuickKey>(), pressed)
        tap("1")
        assertEquals("the same key still answers a plain tap", listOf(QuickKey.ONE), pressed)
    }

    @Test fun aPartlyCoveredTapIsRefused() {
        show()
        tap("1", downFlags = FLAG_WINDOW_IS_PARTIALLY_OBSCURED, upFlags = FLAG_WINDOW_IS_PARTIALLY_OBSCURED)
        assertEquals(emptyList<QuickKey>(), pressed)
        tap("1")
        assertEquals(listOf(QuickKey.ONE), pressed)
    }

    @Test fun aGestureThatStartsCoveredIsRefusedEvenIfItEndsUncovered() {
        show()
        tap("1", downFlags = FLAG_WINDOW_IS_OBSCURED)
        tap("2", downFlags = FLAG_WINDOW_IS_PARTIALLY_OBSCURED)
        assertEquals(emptyList<QuickKey>(), pressed)
        tap("2")
        assertEquals(listOf(QuickKey.TWO), pressed)
    }

    @Test fun aGestureThatEndsCoveredIsRefusedEvenIfItStartedUncovered() {
        show()
        tap("1", upFlags = FLAG_WINDOW_IS_OBSCURED)
        tap("3", upFlags = FLAG_WINDOW_IS_PARTIALLY_OBSCURED)
        assertEquals(emptyList<QuickKey>(), pressed)
        tap("3")
        assertEquals(listOf(QuickKey.THREE), pressed)
    }

    @Test fun oneCoveredEventInTheMiddleRefusesTheGesture() {
        show()
        tap("1", moveFlags = FLAG_WINDOW_IS_OBSCURED)
        assertEquals(emptyList<QuickKey>(), pressed)
        tap("1", moveFlags = 0)   // the same gesture without the covered move does go through
        assertEquals(listOf(QuickKey.ONE), pressed)
    }
}
