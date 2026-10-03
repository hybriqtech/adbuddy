package tech.hybriq.adbuddy.ui

import java.time.Instant
import java.time.ZoneOffset
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Test

class EntryTimeTest {
    private val zone = ZoneOffset.UTC
    private val now = Instant.parse("2026-10-03T20:00:00Z").toEpochMilli()

    @Test fun theOffsetOfTheTimestampDoesNotMatter() {
        assertEquals(entryTime("2026-10-03T18:03:11+02:00", now, zone), entryTime("2026-10-03T16:03:11Z", now, zone))
    }

    @Test fun todayIsOnlyTheHour() {
        val t = entryTime("2026-10-03T16:03:11Z", now, zone)!!
        assertEquals(Formats.time(Instant.parse("2026-10-03T16:03:11Z").toEpochMilli(), java.time.format.FormatStyle.SHORT, zone), t)
    }

    @Test fun anotherDayIsDateAndHour() {
        val today = entryTime("2026-10-03T16:03:11Z", now, zone)!!
        val before = entryTime("2026-10-01T16:03:11Z", now, zone)!!
        assertNotEquals(today, before)
        assert(before.length > today.length) { "$before should carry the date: $today" }
    }

    @Test fun anUnreadableOneIsNull() {
        assertNull(entryTime("nope", now, zone))
        assertNull(entryTime("", now, zone))
    }
}
