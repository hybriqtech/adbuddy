package tech.hybriq.adbuddy.ui

import java.time.Instant
import java.time.ZoneOffset
import java.time.format.FormatStyle
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Test

class FormatsTest {
    private val at = Instant.parse("2026-10-02T14:05:09Z").toEpochMilli()

    /** Newer JDKs put a narrow no-break space before PM: the same text for the person. */
    private fun plain(s: String) = s.replace(' ', ' ').replace(' ', ' ')

    @Test fun spanishWritesTheHourOf24() {
        assertEquals("14:05:09", plain(Formats.time(at, FormatStyle.MEDIUM, ZoneOffset.UTC, Locale.forLanguageTag("es-ES"))))
        assertEquals("14:05", plain(Formats.time(at, FormatStyle.SHORT, ZoneOffset.UTC, Locale.forLanguageTag("es"))))
    }

    @Test fun englishWritesItsOwnWay() {
        assertEquals("2:05:09 PM", plain(Formats.time(at, FormatStyle.MEDIUM, ZoneOffset.UTC, Locale.US)))
        assertEquals("2:05 PM", plain(Formats.time(at, FormatStyle.SHORT, ZoneOffset.UTC, Locale.US)))
    }

    @Test fun theZoneIsTheOneAsked() {
        assertEquals("16:05:09", plain(Formats.time(at, FormatStyle.MEDIUM, ZoneOffset.ofHours(2), Locale.forLanguageTag("es-ES"))))
    }
}
