package tech.hybriq.adbuddy.control

import android.provider.Settings
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment

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
}
