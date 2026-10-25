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
        var readsZero = emptySet<String>() // reads 0 whatever the value, as Android 17 does for adb_enabled (addendum 2026-10-19)
        var refuses = emptySet<String>() // put() says false and nothing changes (Settings.Global.putInt failing)
        override fun get(name: String): Int? {
            if (name in unreadable) throw SecurityException("cannot read $name")
            if (name in readsZero) return 0
            return values[name]
        }
        override fun put(name: String, value: Int): Boolean {
            if (!allowed) throw SecurityException("no WRITE_SECURE_SETTINGS")
            if (name in refuses) return false
            if (name !in ignoreWrites) values[name] = value
            return true
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

    // Addendum 2026-10-19 R16: a 0 for adb_enabled with wireless debugging at 1 is not believed. Before, this test saved
    // adb_enabled=0 and restored it as 0; on Android 17 that 0 is false and restoring it would turn USB debugging off.
    @Test fun cuttingWithOnlyWirelessReadSavesUsbAsOn_andRestoresBoth() {
        val s = FakeSettings(adb to 0, wifi to 1)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        assertEquals("a 0 that wireless debugging contradicts is saved as 1", 1, saved.values[adb])
        assertEquals(1, saved.values[wifi])
        assertTrue(sw.isCut())
        assertTrue(sw.restore())
        assertEquals(1, s.values[adb])
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

    @Test fun aStaleRecordIsForgottenBeforeTheNextCut() {
        val s = FakeSettings(adb to 1, wifi to 1)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        s.values[adb] = 1 // the person turns only USB debugging on by hand; forgetIfRestored() has not run
        assertEquals("record still there", 1, saved.values[wifi])
        assertTrue(sw.cut())
        assertTrue(sw.restore())
        assertEquals(1, s.values[adb])
        assertEquals("wireless was off just before the second cut", 0, s.values[wifi])
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

    // --- Addendum 2026-10-19: Android 17 reads adb_enabled=0 for apps; adb_wifi_enabled reads right. ---

    @Test fun android17_cuttingSavesUsbAsOnAndCuts() {
        val s = FakeSettings(adb to 1, wifi to 1).apply { readsZero = setOf(adb) }
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        assertEquals("the false 0 is not saved", 1, saved.values[adb])
        assertEquals(1, saved.values[wifi])
        assertEquals(0, s.values[adb])
        assertEquals(0, s.values[wifi])
        assertTrue(sw.isCut())
    }

    @Test fun android17_restoringBringsBothBackAndIsDone() {
        val s = FakeSettings(adb to 1, wifi to 1).apply { readsZero = setOf(adb) }
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        assertTrue("adb_enabled reads 0 after writing 1, but wireless reads 1: the 0 is the doubtful one", sw.restore())
        assertEquals(1, s.values[adb])
        assertEquals(1, s.values[wifi])
        assertTrue("forgotten once restored", saved.values.isEmpty())
        assertFalse(sw.isCut())
    }

    @Test fun aWirelessThatDoesNotComeBackFailsTheRestore() {
        for (android17 in listOf(false, true)) {
            val s = FakeSettings(adb to 1, wifi to 1).apply { if (android17) readsZero = setOf(adb) }
            val saved = FakeSaved()
            val sw = AccessSwitch(s, saved)
            assertTrue(sw.cut())
            s.ignoreWrites = setOf(wifi)
            assertFalse("android17=$android17", sw.restore())
            assertEquals("the record stays", 1, saved.values[adb])
            assertEquals("the record stays", 1, saved.values[wifi])
            assertEquals("still cut only while nothing reads 1 (USB came back where it reads right)", android17, sw.isCut())
        }
    }

    @Test fun aZeroWithNothingAgainstItIsBelieved() {
        val s = FakeSettings(adb to 0, wifi to 0)
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        assertEquals(0, saved.values[adb])
        assertEquals(0, saved.values[wifi])
        assertTrue("R4: a record of both off is not guessed", sw.needsManualRestore())
    }

    @Test fun withoutAContradictionAZeroAfterRestoringIsBelieved() {
        // Known cost (addendum R19): USB only on Android 17. Writing 1 and reading 0 with wireless at 0 is not done.
        val s = FakeSettings(adb to 0, wifi to 0).apply { readsZero = setOf(adb) }
        val saved = FakeSaved().apply { values[adb] = 1; values[wifi] = 0 }
        val sw = AccessSwitch(s, saved)
        assertFalse(sw.restore())
        assertEquals("the record stays", 1, saved.values[adb])
    }

    @Test fun onlyAContradictedZeroIsDoubted() {
        assertTrue(AccessSwitch.adbZeroIsDoubtful(0, 1))
        assertTrue(AccessSwitch.adbZeroIsDoubtful(0, 0, channelUp = true))
        assertFalse(AccessSwitch.adbZeroIsDoubtful(0, 0))
        assertFalse(AccessSwitch.adbZeroIsDoubtful(0, null))
        assertFalse(AccessSwitch.adbZeroIsDoubtful(1, 1, channelUp = true))
        assertFalse(AccessSwitch.adbZeroIsDoubtful(null, 1, channelUp = true))
    }

    // --- Addendum 2026-10-19 R18: put() saying false is a failed write, even when the reading cannot see it. ---

    @Test fun android17_aWriteAndroidRefusesIsNotDone() {
        val s = FakeSettings(adb to 1, wifi to 1).apply { readsZero = setOf(adb); refuses = setOf(adb) }
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertFalse("adb_enabled reads 0 anyway: only put() can tell", sw.cut())
        assertTrue(saved.values.isEmpty())
        assertFalse(sw.isCut())
    }

    @Test fun android17_aRefusedRestoreKeepsTheRecord() {
        val s = FakeSettings(adb to 1, wifi to 1).apply { readsZero = setOf(adb) }
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertTrue(sw.cut())
        s.refuses = setOf(adb)
        assertFalse(sw.restore())
        assertEquals(1, saved.values[adb])
    }

    // --- Addendum 2026-10-19 R23: a record of only wireless debugging comes from the build before the addendum. ---

    @Test fun anOldRecordOfOnlyWirelessRestoresUsbToo() {
        for (android17 in listOf(false, true)) {
            val s = FakeSettings(adb to 0, wifi to 0).apply { if (android17) readsZero = setOf(adb) }
            val saved = FakeSaved().apply { values[adb] = 0; values[wifi] = 1 }
            assertTrue("android17=$android17", AccessSwitch(s, saved).restore())
            assertEquals("android17=$android17", 1, s.values[adb])
            assertEquals(1, s.values[wifi])
            assertTrue(saved.values.isEmpty())
        }
    }

    // Round 1 review: a put() that says false must not stop the other writes: cutting still tries to turn USB off.
    @Test fun aRefusedWirelessWriteStillTriesToCutUsb() {
        val s = FakeSettings(adb to 1, wifi to 1).apply { refuses = setOf(wifi) }
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved)
        assertFalse(sw.cut())
        assertEquals("USB was still tried", 0, s.values[adb])
        assertTrue("a failed cut leaves no record", saved.values.isEmpty())
    }

    // --- Addendum 2026-10-19 R16/R20: a live channel with the PC goes through adbd. ---

    @Test fun aLiveChannelSavesUsbAsOnEvenIfItReadsOff() {
        val s = FakeSettings(adb to 1, wifi to 0).apply { readsZero = setOf(adb) } // USB only, Android 17, channel alive
        val saved = FakeSaved()
        assertTrue(AccessSwitch(s, saved, channelUp = true).cut())
        assertEquals(1, saved.values[adb])
        assertEquals("wireless debugging is saved as read", 0, saved.values[wifi])
    }

    @Test fun aDoubtRecordedAtTheCutLetsTheRestorePass() {
        // R19: restoring is only possible once cut (the channel is gone by then), so the doubt travels in the record.
        val s = FakeSettings(adb to 1, wifi to 0).apply { readsZero = setOf(adb) } // USB only, Android 17
        val saved = FakeSaved()
        assertTrue(AccessSwitch(s, saved, channelUp = true).cut())
        assertEquals(1, saved.values[AccessSwitch.ADB_DOUBTED])
        assertTrue(AccessSwitch(s, saved).restore())
        assertEquals(1, s.values[adb])
        assertEquals(0, s.values[wifi])
        assertTrue("the doubt goes with the record", saved.values.isEmpty())
    }

    @Test fun aZeroNobodyDoubtedLeavesNoMark() {
        val s = FakeSettings(adb to 1, wifi to 1)
        val saved = FakeSaved()
        assertTrue(AccessSwitch(s, saved, channelUp = true).cut())
        assertEquals("a 1 read is not a doubt", null, saved.values[AccessSwitch.ADB_DOUBTED])
        s.ignoreWrites = setOf(adb) // and a USB write that does not stick is still seen when the reading is right
        assertFalse(AccessSwitch(s, saved).restore())
    }

    @Test fun aLiveChannelNeverUndoesACut() {
        // In the second the tunnel takes to fall after a cut the channel still says CONNECTED: the record stays.
        val s = FakeSettings(adb to 1, wifi to 1).apply { readsZero = setOf(adb) }
        val saved = FakeSaved()
        val sw = AccessSwitch(s, saved, channelUp = true)
        assertTrue(sw.cut())
        sw.forgetIfRestored()
        assertTrue(sw.isCut())
        assertEquals(1, saved.values[wifi])
    }
}
