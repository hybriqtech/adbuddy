package tech.hybriq.adbuddy.ui

import android.content.Context
import android.provider.Settings
import androidx.activity.ComponentActivity
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
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
import tech.hybriq.adbuddy.control.AccessSwitch
import tech.hybriq.adbuddy.control.CutAccess
import tech.hybriq.adbuddy.control.CutCheck
import tech.hybriq.adbuddy.control.CutConfirm
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * ChatRoot wires the shield to the real CutAccess: the shield that says "Cut" cuts (both settings to 0, "cut_done"), and giving
 * access back asks first and then restores what was saved, or says to do it by hand (restore_manual) without writing.
 * Swapping cut and restore in ChatRoot's ChatActions turns these red.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ChatRootCutWiringTest {
    private val compose = createAndroidComposeRule<ComponentActivity>()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    // The PC running the tests may have an emulator on 127.0.0.1:5555: the check after a cut (R22) must not see it.
    @Before fun noAdbdHere() {
        CutAccess.probe = { false }
    }

    @After fun realProbe() {
        CutAccess.probe = CutConfirm::adbdListening
        CutAccess.waitMs = CutConfirm.WAIT_MS
    }

    private fun waitForText(id: Int) = compose.waitUntil(5_000) { compose.onAllNodesWithText(text(id)).fetchSemanticsNodes().isNotEmpty() }

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private fun text(id: Int) = app.getString(id)
    private fun setting(name: String): Int? = Settings.Global.getString(app.contentResolver, name)?.toIntOrNull()
    private fun put(adb: Int, wifi: Int) {
        Settings.Global.putInt(app.contentResolver, AccessSwitch.ADB_ENABLED, adb)
        Settings.Global.putInt(app.contentResolver, AccessSwitch.ADB_WIFI_ENABLED, wifi)
    }
    private fun record(adb: Int, wifi: Int) {
        app.getSharedPreferences(CutAccess.PREFS, Context.MODE_PRIVATE).edit()
            .putInt(AccessSwitch.ADB_ENABLED, adb).putInt(AccessSwitch.ADB_WIFI_ENABLED, wifi).commit()
    }
    private fun savedRecord(): Pair<Int?, Int?> = app.getSharedPreferences(CutAccess.PREFS, Context.MODE_PRIVATE).let { p ->
        fun get(n: String) = if (p.contains(n)) p.getInt(n, 0) else null
        get(AccessSwitch.ADB_ENABLED) to get(AccessSwitch.ADB_WIFI_ENABLED)
    }

    private fun mount() = compose.setContent {
        AdbuddyTheme(dynamic = false) { ChatRoot(app, focusConfirm = null, closed = emptySet(), firstRun = false, askForNotifications = false, onClose = {}) }
    }

    // R15: the full-screen chat starts with the terminal folded too (the default of ChatRoot).
    @Test fun theTerminalStartsFolded() {
        mount()
        compose.onNodeWithContentDescription(text(R.string.chat_terminal_show)).assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.chat_terminal_hide)).assertDoesNotExist()
    }

    @Test fun theShieldCutsBothSettingsAndSaysSo() {
        app.getSharedPreferences(CutAccess.PREFS, Context.MODE_PRIVATE).edit().clear().commit()
        put(1, 1)
        mount()
        compose.onNodeWithContentDescription(text(R.string.action_cut)).performClick()
        compose.waitForIdle()
        assertEquals(0, setting(AccessSwitch.ADB_ENABLED))
        assertEquals(0, setting(AccessSwitch.ADB_WIFI_ENABLED))
        waitForText(R.string.cut_done) // after the check of R22, off the main thread
        compose.onNodeWithText(text(R.string.cut_done)).assertIsDisplayed()
    }

    @Test fun aCutThatLeavesAdbAnsweringSaysSoWithTheWayToDeveloperOptions() {
        app.getSharedPreferences(CutAccess.PREFS, Context.MODE_PRIVATE).edit().clear().commit()
        put(1, 1)
        CutAccess.probe = { true } // adbd still answers on 5555: USB debugging did not go off
        CutAccess.waitMs = 50
        mount()
        compose.onNodeWithContentDescription(text(R.string.action_cut)).performClick()
        waitForText(R.string.cut_still_connected)
        compose.onNodeWithText(text(R.string.cut_still_connected)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.action_developer_options)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.cut_done)).assertDoesNotExist()
    }

    // R22: the check outlives the screen. Leaving the composition (turning the phone, going back) within the wait must not
    // lose the notice: it shows in the next composition.
    @Test fun theNoticeSurvivesTheScreenBeingRecreatedDuringTheCheck() {
        app.getSharedPreferences(CutAccess.PREFS, Context.MODE_PRIVATE).edit().clear().commit()
        put(1, 1)
        CutAccess.probe = { true }
        CutAccess.waitMs = 1_500
        var show by mutableStateOf(true)
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                if (show) ChatRoot(app, focusConfirm = null, closed = emptySet(), firstRun = false, askForNotifications = false, onClose = {})
            }
        }
        compose.onNodeWithContentDescription(text(R.string.action_cut)).performClick()
        compose.waitForIdle()
        show = false // the screen goes away while the check still waits
        compose.waitForIdle()
        show = true
        waitForText(R.string.cut_still_connected)
        compose.onNodeWithText(text(R.string.cut_still_connected)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.cut_done)).assertDoesNotExist()
    }

    // Giving access back while the check still waits cancels it: no late "done" or "still connected" appears.
    @Test fun givingAccessBackDuringTheCheckLeavesNoResult() {
        app.getSharedPreferences(CutAccess.PREFS, Context.MODE_PRIVATE).edit().clear().commit()
        put(1, 1)
        CutAccess.probe = { true }
        CutAccess.waitMs = 1_000
        mount()
        compose.onNodeWithContentDescription(text(R.string.action_cut)).performClick()
        compose.waitForIdle()
        compose.onNodeWithContentDescription(text(R.string.action_restore)).performClick()
        compose.waitForIdle()
        compose.onNodeWithText(text(R.string.restore_confirm_confirm)).performClick()
        compose.waitForIdle()
        Thread.sleep(2_500) // longer than the wait of the check
        compose.waitForIdle()
        assertEquals("the cancelled check published nothing", null, app.cutCheck.value)
        compose.onNodeWithText(text(R.string.cut_still_connected)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.cut_done)).assertDoesNotExist()
    }

    // R22: the notice is consumed once shown. One still up when the screen is recreated comes back in the new one.
    @Test fun aStillConnectedNoticeStillUpComesBackWhenTheScreenIsRecreated() {
        app.getSharedPreferences(CutAccess.PREFS, Context.MODE_PRIVATE).edit().clear().commit()
        put(1, 1)
        CutAccess.probe = { true }
        CutAccess.waitMs = 300
        var show by mutableStateOf(true)
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                if (show) ChatRoot(app, focusConfirm = null, closed = emptySet(), firstRun = false, askForNotifications = false, onClose = {})
            }
        }
        compose.onNodeWithContentDescription(text(R.string.action_cut)).performClick()
        waitForText(R.string.cut_still_connected) // up, not closed
        show = false
        compose.waitForIdle()
        compose.onNodeWithText(text(R.string.cut_still_connected)).assertDoesNotExist()
        assertEquals("not consumed: nobody closed it", CutCheck.STILL_CONNECTED, app.cutCheck.value)
        show = true
        waitForText(R.string.cut_still_connected)
        compose.onNodeWithText(text(R.string.cut_still_connected)).assertIsDisplayed()
    }

    @Test fun aBothOffRecordAsksFirstThenSaysToDoItByHandAndWritesNothing() {
        put(0, 0)
        record(0, 0)
        mount()
        compose.onNodeWithContentDescription(text(R.string.action_restore)).performClick()
        compose.waitForIdle()
        assertEquals("asking writes nothing", 0, setting(AccessSwitch.ADB_ENABLED))
        compose.onNodeWithText(text(R.string.restore_confirm_confirm)).performClick()
        compose.waitForIdle()
        compose.onNodeWithText(text(R.string.restore_manual)).assertIsDisplayed()
        assertEquals(0, setting(AccessSwitch.ADB_ENABLED))
        assertEquals(0, setting(AccessSwitch.ADB_WIFI_ENABLED))
        assertEquals("the record stays", 0 to 0, savedRecord())
    }

    @Test fun anOwnRecordIsRestoredAsItWasAfterConfirming() {
        put(0, 0)
        record(1, 0)
        mount()
        compose.onNodeWithContentDescription(text(R.string.action_restore)).performClick()
        compose.waitForIdle()
        assertEquals("asking writes nothing", 0, setting(AccessSwitch.ADB_ENABLED))
        compose.onNodeWithText(text(R.string.restore_confirm_confirm)).performClick()
        compose.waitForIdle()
        assertEquals(1, setting(AccessSwitch.ADB_ENABLED))
        assertEquals("the wireless one stays as it was", 0, setting(AccessSwitch.ADB_WIFI_ENABLED))
        compose.onNodeWithText(text(R.string.cut_restored)).assertIsDisplayed()
    }
}
