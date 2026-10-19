package tech.hybriq.adbuddy.control

import android.content.Context
import android.provider.Settings
import java.io.File
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.SessionState
import tech.hybriq.adbuddy.listener.CodeScan

@RunWith(RobolectricTestRunner::class)
class CutAccessTest {
    private val app get() = RuntimeEnvironment.getApplication()

    @Test fun onlyWirelessOnAndNoRecordIsNotCut_untilACutOfOurs() {
        Settings.Global.putInt(app.contentResolver, AccessSwitch.ADB_ENABLED, 0)
        Settings.Global.putInt(app.contentResolver, AccessSwitch.ADB_WIFI_ENABLED, 1)
        assertFalse(CutAccess.isCut(app))
        assertFalse("the first emission too", runBlocking { CutAccess.observe(app).first() })
        assertTrue(CutAccess.cut(app))
        assertTrue(CutAccess.isCut(app))
        assertTrue(runBlocking { CutAccess.observe(app).first() })
    }

    // Addendum 2026-10-19 R20: only a host that welcomed the app (paused included) is a live channel.
    @Test fun theChannelIsUpOnlyWhenTheHostIsConnected() {
        assertTrue(CutAccess.channelUp(BridgeState(connection = BridgeState.Connection.CONNECTED)))
        assertTrue(
            "paused is still a live channel",
            CutAccess.channelUp(BridgeState(connection = BridgeState.Connection.CONNECTED, sessionState = SessionState(paused = true))),
        )
        for (c in listOf(BridgeState.Connection.WAITING, BridgeState.Connection.CONNECTING, BridgeState.Connection.INCOMPATIBLE)) {
            assertFalse(c.name, CutAccess.channelUp(BridgeState(connection = c)))
        }
    }

    @Test fun aLiveChannelSavesUsbAsOn() {
        val prefs = app.getSharedPreferences(CutAccess.PREFS, Context.MODE_PRIVATE)
        prefs.edit().clear().commit()
        Settings.Global.putInt(app.contentResolver, AccessSwitch.ADB_ENABLED, 0)
        Settings.Global.putInt(app.contentResolver, AccessSwitch.ADB_WIFI_ENABLED, 0)
        assertTrue(CutAccess.cut(app, channelUp = true))
        assertEquals(1, prefs.getInt(AccessSwitch.ADB_ENABLED, -1))
        assertEquals(0, prefs.getInt(AccessSwitch.ADB_WIFI_ENABLED, -1))
    }

    // The shield of the screen and Cut in the notification (spec 2026-10-18 R5, §8.7) tell cut() whether the channel is
    // alive (R20) and check the cut afterwards (R22). Read as code (no comments, no strings), exactly one call each.
    @Test fun bothCutButtonsPassTheChannelAndCheckTheCut() {
        val cutCall = Regex("""CutAccess\.cut\(""")
        val confirmCall = Regex("""CutAccess\.confirm\(""")
        // The channel is read at the tap, right before the only cut(), in the same block.
        val upThenCut = Regex("""val up = CutAccess\.channelUp\(app\.bridge\.state\.value\)\s+if \(!CutAccess\.cut\(\s*(context|this)\s*,\s*up\s*\)\)""")
        val confirm = Regex("""CutAccess\.confirm\(\s*(context|this@BridgeService)\s*,\s*up\s*\)\s*\{\s*CutAccess\.channelUp\(app\.bridge\.state\.value\)\s*\}""")
        for (path in listOf("ui/ChatRoot.kt", "bridge/BridgeService.kt")) {
            val src = CodeScan.code(File("src/main/kotlin/tech/hybriq/adbuddy/$path").readText())
            assertEquals("$path has exactly one cut() call", 1, cutCall.findAll(src).count())
            assertTrue("$path reads the channel at the tap and passes it to that cut()", upThenCut.containsMatchIn(src))
            assertEquals("$path has exactly one confirm() call", 1, confirmCall.findAll(src).count())
            assertTrue("$path checks the cut with the channel now", confirm.containsMatchIn(src))
            assertTrue("$path says when the PC is still connected", src.contains("CutCheck.STILL_CONNECTED"))
        }
    }
}
