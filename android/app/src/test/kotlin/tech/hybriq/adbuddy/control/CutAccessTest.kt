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
    // The screen hands the check to the app (it must outlive a rotation); the service runs its own.
    @Test fun bothCutButtonsPassTheChannelAndCheckTheCut() {
        fun code(path: String) = CodeScan.code(File("src/main/kotlin/tech/hybriq/adbuddy/$path").readText())
        val cutCall = Regex("""CutAccess\.cut\(""")
        val confirmCall = Regex("""CutAccess\.confirm\(""")
        // The channel is read at the tap, right before the only cut(), in the same block.
        fun upThenCut(who: String) = Regex("""val up = CutAccess\.channelUp\(app\.bridge\.state\.value\)\s+if \(!CutAccess\.cut\(\s*$who\s*,\s*up\s*\)\)""")

        val screen = code("ui/ChatRoot.kt")
        assertEquals("ChatRoot has exactly one cut() call", 1, cutCall.findAll(screen).count())
        assertTrue(
            "ChatRoot reads the channel at the tap, passes it to that cut() and hands the check to the app in its else branch",
            Regex(upThenCut("context").pattern + """\s*\{[^{}]*\}\s*else\s*\{\s*app\.confirmCut\(up\)\s*\}""").containsMatchIn(screen),
        )
        assertEquals("ChatRoot hands over exactly one check", 1, Regex("""app\.confirmCut\(""").findAll(screen).count())
        assertEquals("the check does not run in the screen", 0, confirmCall.findAll(screen).count())
        assertTrue("ChatRoot says when the PC is still connected", screen.contains("CutCheck.STILL_CONNECTED"))

        val app = code("AdbuddyApp.kt")
        assertEquals("the app runs exactly one check", 1, confirmCall.findAll(app).count())
        assertTrue(
            "the app checks with the channel now",
            Regex("""CutAccess\.confirm\(\s*this@AdbuddyApp\s*,\s*wasUp\s*\)\s*\{\s*CutAccess\.channelUp\(bridge\.state\.value\)\s*\}""").containsMatchIn(app),
        )

        val service = code("bridge/BridgeService.kt")
        assertEquals("BridgeService has exactly one cut() call", 1, cutCall.findAll(service).count())
        assertTrue("BridgeService reads the channel at the tap and passes it to that cut()", upThenCut("this").containsMatchIn(service))
        assertEquals("BridgeService has exactly one confirm() call", 1, confirmCall.findAll(service).count())
        assertTrue(
            "BridgeService checks the cut with the channel now",
            Regex("""CutAccess\.confirm\(\s*this@BridgeService\s*,\s*up\s*\)\s*\{\s*CutAccess\.channelUp\(app\.bridge\.state\.value\)\s*\}""").containsMatchIn(service),
        )
        assertTrue("BridgeService says when the PC is still connected", service.contains("CutCheck.STILL_CONNECTED"))
    }
}
