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
    // alive (R20).
    @Test fun bothCutButtonsPassTheChannel() {
        val up = Regex("""val up = CutAccess\.channelUp\(app\.bridge\.state\.value\)""")
        val cut = Regex("""CutAccess\.cut\(\s*(context|this)\s*,\s*up\s*\)""")
        val withoutChannel = Regex("""CutAccess\.cut\(\s*(context|this)\s*\)""")
        for (path in listOf("ui/ChatRoot.kt", "bridge/BridgeService.kt")) {
            val src = File("src/main/kotlin/tech/hybriq/adbuddy/$path").readText()
            assertTrue("$path reads the channel", up.containsMatchIn(src))
            assertTrue("$path passes it to cut()", cut.containsMatchIn(src))
            assertFalse("$path cuts without the channel", withoutChannel.containsMatchIn(src))
        }
    }
}
