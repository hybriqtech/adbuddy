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
}
