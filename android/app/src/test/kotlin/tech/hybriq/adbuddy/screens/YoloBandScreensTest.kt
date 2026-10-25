package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.JournalEntry
import tech.hybriq.adbuddy.bridge.PermissionResolved
import tech.hybriq.adbuddy.ui.ActivityScreen
import tech.hybriq.adbuddy.ui.YOLO_APPROVALS_TAG
import tech.hybriq.adbuddy.ui.YOLO_BAND_TAG
import tech.hybriq.adbuddy.ui.YoloApprovals
import tech.hybriq.adbuddy.ui.YoloBand
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme
import tech.hybriq.adbuddy.yolo.YoloUi
import tech.hybriq.adbuddy.yolo.YoloUi.Kind

/** The YOLO band, the approvals line of the chat and the mark in the activity, in every ScreenMode. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class YoloBandScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)
    private var turnedOff = 0

    private fun band(view: YoloUi.View) {
        compose.setContent { AdbuddyTheme(dynamic = false) { YoloBand(view, onOff = { turnedOff++ }) } }
    }

    // Mutation: draw the band whatever the view says.
    @Test fun nothingIsDrawnWhenTheModeIsOff() {
        band(YoloUi.View(Kind.OFF))
        compose.onNodeWithTag(YOLO_BAND_TAG).assertDoesNotExist()
    }

    @Test fun nothingIsDrawnWhenTheModeIsUnknown() {
        band(YoloUi.View(Kind.UNKNOWN))
        compose.onNodeWithTag(YOLO_BAND_TAG).assertDoesNotExist()
    }

    @Test fun theBandCountsDownAndTurnsOffWithOneTap() {
        band(YoloUi.View(Kind.TIMED, 240_000))
        compose.onNodeWithTag(YOLO_BAND_TAG).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.yolo_band_timed, "4:00")).assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("yolo_band_timed", mode)
        compose.onNodeWithText(text(R.string.yolo_band_off)).performClick()
        assertEquals("no question: one tap", 1, turnedOff)
    }

    @Test fun theSessionBand() {
        band(YoloUi.View(Kind.SESSION))
        compose.onNodeWithText(text(R.string.yolo_band_session)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("yolo_band_session", mode)
    }

    @Test fun theApprovalsLineNamesWhatYoloApproved() {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                YoloApprovals(listOf(PermissionResolved("p_1", "allow", "yolo", "Run npm test"), PermissionResolved("p_2", "allow", "yolo", "")))
            }
        }
        compose.onNodeWithTag(YOLO_APPROVALS_TAG).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.yolo_approved, "Run npm test")).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.yolo_approved_unnamed)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("yolo_approvals", mode)
    }

    // Mutation: draw the mark whatever the entry says.
    @Test fun anEntryThatRanUnderYoloIsMarked() {
        val state = BridgeState(
            journal = listOf(
                JournalEntry(id = "j_1", level = "red", description = "Restart the phone", status = "ok", argv = listOf("reboot"), yolo = true),
                JournalEntry(id = "j_2", level = "green", description = "Read the battery level", status = "ok", argv = listOf("shell", "dumpsys", "battery")),
            ),
        )
        compose.setContent { AdbuddyTheme(dynamic = false) { ActivityScreen(state, onBack = {}) } }
        compose.onAllNodesWithText(text(R.string.yolo_entry_mark)).assertCountEquals(1)
        compose.shot("yolo_activity_mark", mode)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
