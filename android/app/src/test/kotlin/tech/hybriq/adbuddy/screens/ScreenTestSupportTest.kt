package tech.hybriq.adbuddy.screens

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.wrapContentHeight
import androidx.compose.material3.Text
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.ui.TOP_BAR_TAG

/**
 * assertTextsFit catches what it says it catches (and passes a plain text): the screen tests lean on it. It needs
 * the native graphics: in Robolectric's legacy mode text is not really measured and nothing is ever cut short.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ScreenTestSupportTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private fun failure(): String = assertThrows(AssertionError::class.java) { compose.assertTextsFit() }.message.orEmpty()

    @Test fun aTextWithRoomFits() {
        compose.setContent { Text("Protected apps") }
        compose.assertTextsFit()
    }

    @Test fun anEllipsisIsCutShort() {
        compose.setContent { Box(Modifier.width(60.dp)) { Text("A text far too long for its box", maxLines = 1, overflow = TextOverflow.Ellipsis) } }
        assertTrue(failure().startsWith("cut short"))
    }

    @Test fun linesThatDoNotFitAreCutShort() {
        compose.setContent { Box(Modifier.width(60.dp)) { Text("A text far too long for its box", maxLines = 1) } }
        assertTrue(failure().startsWith("cut short"))
    }

    @Test fun aTextOffTheScreenSidewaysIsCaught() {
        compose.setContent { Text("Gone", Modifier.offset(x = 500.dp)) }
        assertTrue(failure().startsWith("off screen sideways"))
    }

    @Test fun aTextAboveTheScreenIsCaughtUnlessTheListWasScrolled() {
        compose.setContent { Text("Gone", Modifier.offset(y = (-30).dp)) }
        assertTrue(failure().startsWith("off screen at the top"))
        compose.assertTextsFit(scrolled = true)
    }

    /** Eight lines of text: taller than the 64 dp bar. */
    private val TALL = listOf("One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight").joinToString(separator = "\n")

    @Test fun aTitleTallerThanTheTopBarIsCaught() {
        // The text may be taller than the bar's 64 dp (unbounded), as a title that the bar does not clip.
        compose.setContent { Box(Modifier.height(64.dp).testTag(TOP_BAR_TAG)) { Box(Modifier.wrapContentHeight(Alignment.Top, unbounded = true)) { Text(TALL) } } }
        val error = assertThrows(AssertionError::class.java) { compose.assertTitleFitsTheTopBar(TALL) }
        assertTrue(error.message.orEmpty().startsWith("title sticks out"))
        // As a known issue it passes while it still sticks out.
        compose.assertTitleFitsTheTopBar(TALL, known = true)
    }

    @Test fun aKnownIssueThatIsFixedFails() {
        compose.setContent { Box(Modifier.height(64.dp).testTag(TOP_BAR_TAG)) { Text("Notification access") } }
        compose.assertTitleFitsTheTopBar("Notification access")
        val error = assertThrows(AssertionError::class.java) { compose.assertTitleFitsTheTopBar("Notification access", known = true) }
        assertTrue(error.message.orEmpty().startsWith("known issue gone"))
    }

    @Test fun twoTextsDrawnOverEachOtherAreCaught() {
        compose.setContent {
            Box {
                Text("Allowed")
                Text("Unknown")
            }
        }
        assertTrue(failure().startsWith("overlap"))
    }
}
