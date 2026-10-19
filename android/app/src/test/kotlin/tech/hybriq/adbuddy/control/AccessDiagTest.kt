package tech.hybriq.adbuddy.control

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

class AccessDiagTest {
    @Test fun theLineSaysWhatWasReadAndHow() {
        val line = AccessDiag.line(
            "observe", 37,
            listOf(AccessDiag.Reading("adb_enabled", "0", null), AccessDiag.Reading("adb_wifi_enabled", null, "SecurityException")),
            mapOf("adb_enabled" to null, "adb_wifi_enabled" to 1),
        )
        assertEquals(
            "observe sdk=37 adb_enabled=raw:0 adb_wifi_enabled=error:SecurityException saved.adb_enabled=- saved.adb_wifi_enabled=1",
            line,
        )
    }

    @Test fun aNeverWrittenSettingIsNull_notZero() {
        val line = AccessDiag.line("cut", 30, listOf(AccessDiag.Reading("adb_wifi_enabled", null, null)), emptyMap())
        assertEquals("cut sdk=30 adb_wifi_enabled=null", line)
    }

    @Test fun theLineCarriesNoTextFromOutside() {
        // A raw value is cut to 16 chars and stripped of line breaks: the log is not a place for anything else.
        val line = AccessDiag.line("x", 37, listOf(AccessDiag.Reading("adb_enabled", "1\n" + "9".repeat(40), null)), emptyMap())
        assertFalse(line.contains("\n"))
        assertEquals("x sdk=37 adb_enabled=raw:1 99999999999999", line)
    }

    // Addendum 2026-10-19 R21: the line says when the adb_enabled read is a 0 the rest contradicts.
    private fun r(adb: String?, wifi: String?) = listOf(AccessDiag.Reading("adb_enabled", adb, null), AccessDiag.Reading("adb_wifi_enabled", wifi, null))

    @Test fun theLineSaysWhenWirelessContradictsAZero() {
        // What the user's Pixel (Android 17) logged on 2026-10-19 while `adb shell` said 1 and 1.
        assertEquals(
            "observe sdk=37 adb_enabled=raw:0 adb_wifi_enabled=raw:1 saved.adb_enabled=- saved.adb_wifi_enabled=- doubtful=adb_enabled(wifi)",
            AccessDiag.line("observe", 37, r("0", "1"), mapOf("adb_enabled" to null, "adb_wifi_enabled" to null)),
        )
    }

    @Test fun theLineSaysTheChannelWhenItIsKnown() {
        assertEquals(
            "cut.before sdk=37 adb_enabled=raw:0 adb_wifi_enabled=raw:0 channel=up doubtful=adb_enabled(channel)",
            AccessDiag.line("cut.before", 37, r("0", "0"), emptyMap(), channelUp = true),
        )
        assertEquals(
            "cut.before sdk=37 adb_enabled=raw:0 adb_wifi_enabled=raw:1 channel=up doubtful=adb_enabled(wifi+channel)",
            AccessDiag.line("cut.before", 37, r("0", "1"), emptyMap(), channelUp = true),
        )
    }

    @Test fun nothingContradictedNothingSaid() {
        assertEquals("cut.before sdk=37 adb_enabled=raw:0 adb_wifi_enabled=raw:0 channel=down", AccessDiag.line("cut.before", 37, r("0", "0"), emptyMap(), channelUp = false))
        assertEquals("observe sdk=37 adb_enabled=raw:1 adb_wifi_enabled=raw:1", AccessDiag.line("observe", 37, r("1", "1"), emptyMap()))
        assertEquals("observe sdk=37 adb_enabled=null adb_wifi_enabled=raw:1", AccessDiag.line("observe", 37, r(null, "1"), emptyMap()))
    }
}
