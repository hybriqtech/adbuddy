package tech.hybriq.adbuddy.control

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AccessSwitchTest {
    /** Settings.Global as seen by an app: null for a setting never written. */
    private class FakeSettings(vararg values: Pair<String, Int>) : AccessSwitch.Settings {
        val values = mutableMapOf(*values)
        var allowed = true
        var ignoreWrites = emptySet<String>() // written without error, but the value does not change
        var unreadable = emptySet<String>() // reading throws, as Android 12+ may do for a hidden setting
        override fun get(name: String): Int? {
            if (name in unreadable) throw SecurityException("cannot read $name")
            return values[name]
        }
        override fun put(name: String, value: Int) {
            if (!allowed) throw SecurityException("no WRITE_SECURE_SETTINGS")
            if (name !in ignoreWrites) values[name] = value
        }
    }

    private class FakeSaved : AccessSwitch.Saved {
        val values = mutableMapOf<String, Int>()
        override fun get(name: String) = values[name]
        override fun put(name: String, value: Int?) {
            if (value == null) values.remove(name) else values[name] = value
        }
    }

    private val adb = AccessSwitch.ADB_ENABLED
    private val wifi = AccessSwitch.ADB_WIFI_ENABLED

    @Test fun cuttingTurnsBothOff() {
        val s = FakeSettings(adb to 1, wifi to 1)
        val sw = AccessSwitch(s, FakeSaved())
        assertTrue(sw.cut())
        assertEquals(0, s.values[adb])
        assertEquals(0, s.values[wifi])
        assertTrue(sw.isCut())
    }

    @Test fun withoutThePermissionNothingIsCut() {
        val s = FakeSettings(adb to 1, wifi to 1).apply { allowed = false }
        val sw = AccessSwitch(s, FakeSaved())
        assertFalse(sw.cut())
        assertFalse(sw.isCut())
    }

    @Test fun aWriteThatDidNotStickIsNotDone() {
        val s = FakeSettings(adb to 1, wifi to 1).apply { ignoreWrites = setOf(adb) }
        assertFalse("adb_enabled is still 1", AccessSwitch(s, FakeSaved()).cut())
    }

    @Test fun restoringPutsBackWhatWasThere() {
        val s = FakeSettings(adb to 1, wifi to 0)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        assertTrue(sw.restore())
        assertEquals(1, s.values[adb])
        assertEquals("wireless debugging was off: it stays off", 0, s.values[wifi])
        assertTrue("forgotten once restored", saved.values.isEmpty())
    }

    @Test fun restoringWithWirelessDebuggingOnTurnsItBackOn() {
        val s = FakeSettings(adb to 1, wifi to 1)
        val sw = AccessSwitch(s, FakeSaved())
        sw.cut()
        assertTrue(sw.restore())
        assertEquals(1, s.values[wifi])
    }

    @Test fun restoringWithNothingSavedTurnsOnlyAdbOn() {
        val s = FakeSettings(adb to 0, wifi to 0) // cut outside ADBuddy
        assertTrue(AccessSwitch(s, FakeSaved()).restore())
        assertEquals(1, s.values[adb])
        assertEquals("never turned on blindly", 0, s.values[wifi])
    }

    @Test fun aSettingNeverWrittenCountsAsOff() {
        val s = FakeSettings(adb to 1) // adb_wifi_enabled missing
        val sw = AccessSwitch(s, FakeSaved())
        assertTrue(sw.cut())
        assertTrue(sw.restore())
        assertEquals(0, s.values[wifi])
    }

    @Test fun cuttingTwiceKeepsTheFirstValues() {
        val s = FakeSettings(adb to 1, wifi to 1)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        sw.cut()
        sw.cut()
        assertEquals(1, saved.values[wifi])
    }

    @Test fun reactivatingByHandForgetsTheSavedValues() {
        val s = FakeSettings(adb to 1, wifi to 1)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        sw.cut()
        sw.forgetIfRestored()
        assertEquals("still cut: kept", 1, saved.values[wifi])
        s.values[adb] = 1 // the person turns USB debugging on in Settings
        sw.forgetIfRestored()
        assertTrue(saved.values.isEmpty())
        s.values[adb] = 0 // and cuts again later, by hand: nothing old comes back
        assertTrue(sw.restore())
        assertEquals(0, s.values[wifi])
    }

    @Test fun aFailedRestoreKeepsTheSavedValues() {
        val s = FakeSettings(adb to 1, wifi to 1)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        sw.cut()
        s.allowed = false
        assertFalse(sw.restore())
        assertEquals(1, saved.values[wifi])
    }

    @Test fun onlyWirelessDebuggingOnIsNotCut() {
        val sw = AccessSwitch(FakeSettings(adb to 0, wifi to 1), FakeSaved())
        assertFalse(sw.isCut())
    }

    @Test fun withoutACutOfOursNothingIsCut_whateverIsRead() {
        // The Pixel case (Android 17): the app may read 0 for both while adb works. Without our own record it is not cut.
        assertFalse(AccessSwitch(FakeSettings(adb to 0, wifi to 0), FakeSaved()).isCut())
        assertFalse(AccessSwitch(FakeSettings(), FakeSaved()).isCut())
    }

    @Test fun unreadableWithoutARecordIsNotCut() {
        val s = FakeSettings(adb to 0, wifi to 0).apply { unreadable = setOf(wifi) }
        assertFalse(AccessSwitch(s, FakeSaved()).isCut())
    }

    @Test fun cuttingWithOnlyWirelessSavesBothAndRestoresThemAsTheyWere() {
        val s = FakeSettings(adb to 0, wifi to 1)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        assertEquals(0, saved.values[adb])
        assertEquals(1, saved.values[wifi])
        assertTrue(sw.isCut())
        assertTrue(sw.restore())
        assertEquals("USB debugging was off: it stays off", 0, s.values[adb])
        assertEquals("wireless debugging comes back", 1, s.values[wifi])
        assertFalse(sw.isCut())
    }

    @Test fun aFailedCutLeavesNoRecord() {
        val s = FakeSettings(adb to 1, wifi to 1).apply { ignoreWrites = setOf(wifi) }
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertFalse(sw.cut())
        assertTrue("a cut that did not stick must not look like one", saved.values.isEmpty())
        assertFalse(sw.isCut())
    }

    @Test fun aReadBackThatCannotBeReadIsNotDone() {
        val s = FakeSettings(adb to 1, wifi to 1)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        s.unreadable = setOf(wifi)
        assertFalse(sw.cut())
        assertTrue(saved.values.isEmpty())
    }

    @Test fun unreadableAfterOurCutStaysCut_andKeepsTheRecord() {
        val s = FakeSettings(adb to 1, wifi to 1)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        s.unreadable = setOf(wifi)
        sw.forgetIfRestored()
        assertTrue(sw.isCut())
        assertEquals(1, saved.values[wifi])
    }

    @Test fun aRecordOfBothOffIsNeverGuessed() {
        val s = FakeSettings(adb to 0, wifi to 0)
        val saved = FakeSaved().apply { values[adb] = 0; values[wifi] = 0 }
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.needsManualRestore())
        assertFalse(sw.restore())
        assertEquals("nothing written", 0, s.values[adb])
        assertEquals("nothing written", 0, s.values[wifi])
        assertEquals("the record stays", 0, saved.values[wifi])
    }

    @Test fun restoringTwiceWithOnlyWirelessNeverTurnsItOff() {
        val s = FakeSettings(adb to 0, wifi to 1)
        val sw = AccessSwitch(s, FakeSaved())
        assertTrue(sw.restore())
        assertTrue(sw.restore())
        assertEquals("wireless debugging stays on", 1, s.values[wifi])
        assertEquals("nothing turned on that was off", 0, s.values[adb])
    }

    @Test fun turningOnlyWirelessBackOnByHandForgetsTheRecord() {
        val s = FakeSettings(adb to 1, wifi to 1)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        s.values[wifi] = 1 // the person turns wireless debugging on in Settings (USB stays off)
        sw.forgetIfRestored()
        assertTrue(saved.values.isEmpty())
        assertFalse(sw.isCut())
    }
}
