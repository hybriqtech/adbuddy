package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollToNode
import java.time.Instant
import java.time.ZoneOffset
import java.util.TimeZone
import org.junit.After
import org.junit.Before
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
import tech.hybriq.adbuddy.bridge.Inverse
import tech.hybriq.adbuddy.bridge.JournalEntry
import tech.hybriq.adbuddy.ui.ActivityScreen
import tech.hybriq.adbuddy.ui.entryTime
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The journal (C26) in every ScreenMode: level in words, the hour of each entry, the two kinds of empty, how to undo. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ActivityScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private val zone = ZoneOffset.UTC
    private val now = Instant.parse("2026-10-03T20:00:00Z").toEpochMilli()
    private var savedZone: TimeZone? = null

    // The screen reads the system zone: fixed, so the images do not depend on the PC.
    @Before fun fixZone() {
        savedZone = TimeZone.getDefault()
        TimeZone.setDefault(TimeZone.getTimeZone("UTC"))
    }

    @After fun restoreZone() {
        savedZone?.let { TimeZone.setDefault(it) }
    }

    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    private fun show(state: BridgeState) {
        compose.setContent { AdbuddyTheme(dynamic = false) { ActivityScreen(state, onBack = {}, now = { now }) } }
    }

    private val green = JournalEntry(id = "j_1", ts = "2026-10-03T08:15:00Z", level = "green", description = "Read the battery level", status = "ok", argv = listOf("shell", "dumpsys", "battery"))
    private val yellow = JournalEntry(
        id = "j_2", ts = "2026-10-03T12:30:00+00:00", level = "yellow", description = "Change the screen timeout", status = "ok",
        argv = listOf("shell", "settings", "put", "system", "screen_off_timeout", "60000"),
        inverse = Inverse(listOf("shell", "settings", "put", "system", "screen_off_timeout", "30000")),
    )
    private val red = JournalEntry(id = "j_3", ts = "2026-10-01T09:45:00Z", level = "red", description = "Delete a file", status = "denied", argv = listOf("shell", "rm", "/sdcard/x"))

    @Test fun entriesShowLevelInWordsTheirHourAndHowToUndo() {
        show(BridgeState(journal = listOf(green, yellow, red)))
        for (id in listOf(R.string.level_green, R.string.level_yellow, R.string.level_red)) {
            compose.onNodeWithText(text(id), substring = true, ignoreCase = false).assertIsDisplayed()
        }
        compose.assertTextsFit()
        compose.shot("activity_entries", mode)
        // At 200 % the list does not fit the screen: each line is scrolled to before it is checked.
        for (e in listOf(green, yellow, red)) showsInList(entryTime(e.ts, now, zone)!!)
        showsInList(text(R.string.activity_inverse, yellow.inverse!!.argv.joinToString(" ")))
        showsInList(text(R.string.activity_undo_hint, "j_2"))
    }

    private fun showsInList(text: String) {
        compose.onNode(hasScrollAction()).performScrollToNode(hasText(text))
        compose.onNodeWithText(text).assertIsDisplayed()
    }

    // Bug 16: the level is no longer a lone emoji; the bar carries its name, merged into the entry's one node for TalkBack.
    @Test fun eachEntryCarriesItsLevelNameForTalkBack() {
        show(BridgeState(journal = listOf(green, yellow, red)))
        for (id in listOf(R.string.level_green, R.string.level_yellow, R.string.level_red)) {
            showsDescribed(text(id))
        }
    }

    private fun showsDescribed(description: String) {
        compose.onNode(hasScrollAction()).performScrollToNode(hasContentDescription(description))
        compose.onNodeWithContentDescription(description).assertExists()
    }

    @Test fun noEntriesAtAll() {
        show(BridgeState())
        compose.onNodeWithText(text(R.string.home_empty)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("activity_empty", mode)
    }

    @Test fun aFilterThatHidesEverythingSaysSo() {
        show(BridgeState(journal = listOf(green)))
        compose.onNodeWithText(text(R.string.level_red), substring = true).performClick()
        compose.onNodeWithText(text(R.string.activity_filter_empty)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.home_empty)).assertDoesNotExist()
    }

    @Test fun anUnreadableTimeShowsNoHourAndAnOddIdNoUndoHint() {
        val odd = yellow.copy(id = "x; rm", ts = "nope")
        show(BridgeState(journal = listOf(odd)))
        compose.onNodeWithText(odd.description).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.activity_undo_hint, odd.id)).assertDoesNotExist()
    }

    private fun hostileId(id: String) {
        show(BridgeState(journal = listOf(yellow.copy(id = id))))
        compose.onNodeWithText("Change the screen timeout").assertIsDisplayed()
        compose.onAllNodes(hasText("adbuddy undo", substring = true)).assertCountEquals(0)
    }

    @Test fun anIdWithAValidPrefixAndATrailingCommandShowsNoUndoHint() = hostileId("j_1; rm -rf")

    @Test fun anIdWithAValidPrefixAndANewlineShowsNoUndoHint() = hostileId("j_1\nrm")

    @Test fun aDeniedEntryWithAnInverseShowsNoUndoHint() {
        show(BridgeState(journal = listOf(yellow.copy(status = "denied"))))
        compose.onNodeWithText("Change the screen timeout").assertIsDisplayed()
        compose.onAllNodes(hasText("adbuddy undo", substring = true)).assertCountEquals(0)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
