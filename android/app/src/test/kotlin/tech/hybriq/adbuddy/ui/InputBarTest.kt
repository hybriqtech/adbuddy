package tech.hybriq.adbuddy.ui

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshots.Snapshot
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
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
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.files.Draft
import tech.hybriq.adbuddy.files.DraftStatus
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class InputBarTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private fun s(id: Int): String = RuntimeEnvironment.getApplication().getString(id)

    private var sent = 0
    private var stopped = 0

    private fun show(text: String, working: Boolean, drafts: List<Draft> = emptyList()) {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                InputBar(text, {}, drafts, {}, {}, {}, onSend = { sent++ }, working = working, onStop = { stopped++ })
            }
        }
    }

    private fun draft(status: DraftStatus) = Draft(
        "k", "a.png", "image/png", 10, 10, status,
        attachment = if (status == DraftStatus.DONE) Attachment("0".repeat(32), "a.png", "image/png", 10, "x") else null,
    )

    @Test fun notWorkingShowsSend() {
        show("hi", working = false)
        compose.onNodeWithContentDescription(s(R.string.chat_stop)).assertDoesNotExist()
        compose.onNodeWithContentDescription(s(R.string.chat_send)).performClick()
        assertEquals(1, sent)
        assertEquals(0, stopped)
    }

    @Test fun workingAndEmptyShowsAnEnabledStop() {
        show("", working = true)
        compose.onNodeWithContentDescription(s(R.string.chat_send)).assertDoesNotExist()
        compose.onNodeWithContentDescription(s(R.string.chat_stop)).assertIsEnabled().performClick()
        assertEquals(1, stopped)
        assertEquals(0, sent)
    }

    @Test fun workingWithTextKeepsSend() {
        show("hi", working = true)
        compose.onNodeWithContentDescription(s(R.string.chat_send)).assertIsEnabled()
        compose.onNodeWithContentDescription(s(R.string.chat_stop)).assertDoesNotExist()
    }

    @Test fun workingWithADoneDraftKeepsSend() {
        show("", working = true, drafts = listOf(draft(DraftStatus.DONE)))
        compose.onNodeWithContentDescription(s(R.string.chat_send)).assertIsEnabled()
        compose.onNodeWithContentDescription(s(R.string.chat_stop)).assertDoesNotExist()
    }

    @Test fun anUploadingDraftKeepsADisabledSendAndNeverStop() {
        show("hi", working = true, drafts = listOf(draft(DraftStatus.UPLOADING)))
        compose.onNodeWithContentDescription(s(R.string.chat_send)).assertIsNotEnabled()
        compose.onNodeWithContentDescription(s(R.string.chat_stop)).assertDoesNotExist()
    }

    @Test fun stopRestsAfterATapSoASecondTapSendsNoSecondEsc() {
        compose.mainClock.autoAdvance = false
        show("", working = true)
        val stop = compose.onNodeWithContentDescription(s(R.string.chat_stop))
        stop.performClick()
        compose.mainClock.advanceTimeBy(100)
        Snapshot.sendApplyNotifications()
        compose.mainClock.advanceTimeByFrame()
        assertEquals(1, stopped)
        stop.assertIsNotEnabled()
        stop.performClick()
        assertEquals("a second tap adds nothing", 1, stopped)
        compose.mainClock.advanceTimeBy(2_000)
        stop.assertIsNotEnabled()
        compose.mainClock.advanceTimeBy(1_500)
        stop.assertIsEnabled()
    }

    @Test fun stoppingIsResetWhenWorkingChanges() {
        compose.mainClock.autoAdvance = false
        var working by mutableStateOf(true)
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                InputBar("", {}, emptyList(), {}, {}, {}, onSend = { sent++ }, working = working, onStop = { stopped++ })
            }
        }
        compose.onNodeWithContentDescription(s(R.string.chat_stop)).performClick()
        compose.mainClock.advanceTimeBy(100)
        Snapshot.sendApplyNotifications()
        compose.mainClock.advanceTimeByFrame()
        assertEquals(1, stopped)
        compose.onNodeWithContentDescription(s(R.string.chat_stop)).assertIsNotEnabled()
        // The agent stopped and started again: the new run can be stopped without waiting for the 3 s.
        working = false
        Snapshot.sendApplyNotifications()
        compose.mainClock.advanceTimeByFrame()
        working = true
        Snapshot.sendApplyNotifications()
        compose.mainClock.advanceTimeByFrame()
        compose.onNodeWithContentDescription(s(R.string.chat_stop)).assertIsEnabled()
    }

    @Test fun aFailedDraftWithAnEmptyBarWhileWorkingStillShowsSend() {
        show("", working = true, drafts = listOf(draft(DraftStatus.FAILED)))
        compose.onNodeWithContentDescription(s(R.string.chat_send)).assertExists()
        compose.onNodeWithContentDescription(s(R.string.chat_stop)).assertDoesNotExist()
    }
}

