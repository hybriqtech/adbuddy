package tech.hybriq.adbuddy.ui

import androidx.compose.material3.SnackbarDuration
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
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

@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class SnackbarDurationTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    @Test fun durations() {
        assertEquals(SnackbarDuration.Short, SnackbarPolicy.duration("Done", false))
        assertEquals(SnackbarDuration.Long, SnackbarPolicy.duration("x".repeat(61), false))
        assertEquals(SnackbarDuration.Short, SnackbarPolicy.duration("x".repeat(60), false))
        assertEquals(SnackbarDuration.Indefinite, SnackbarPolicy.duration("x", true))
        assertEquals(SnackbarDuration.Indefinite, SnackbarPolicy.duration("x".repeat(200), true))
    }

    @Test fun noticeWithActionHasActionAndDismiss() {
        val host = SnackbarHostState()
        compose.setContent { SnackbarHost(host) }
        CoroutineScope(Dispatchers.Main).launch { host.notify("x", actionLabel = "Open") }
        compose.waitForIdle()
        compose.onNodeWithText("Open").assertExists()
        compose.onNodeWithContentDescription("Dismiss").assertExists()
    }

    @Test fun shortNoticeGoesAway() {
        val host = SnackbarHostState()
        compose.setContent { SnackbarHost(host) }
        CoroutineScope(Dispatchers.Main).launch { host.notify("Done") }
        compose.waitForIdle()
        compose.onNodeWithText("Done").assertExists()
        compose.mainClock.advanceTimeBy(5_000)
        compose.waitForIdle()
        compose.onNodeWithText("Done").assertDoesNotExist()
    }
}
