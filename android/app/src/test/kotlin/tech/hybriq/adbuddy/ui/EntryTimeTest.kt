package tech.hybriq.adbuddy.ui

import java.time.Instant
import java.time.ZoneOffset
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
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
        assertTrue("$before should carry the date: $today", before.length > today.length)
        val date = java.time.format.DateTimeFormatter.ofLocalizedDate(java.time.format.FormatStyle.SHORT).withZone(zone)
            .format(Instant.parse("2026-10-01T16:03:11Z"))
        assertTrue("$before should show the date $date", before.contains(date))
    }

    @Test fun theDayBoundaryFollowsTheZone() {
        // The same two instants: the 3rd both in UTC (22:30 and 21:30), but at +02:00 now is the 4th 00:30 and the entry the 3rd 23:30.
        val lateNow = Instant.parse("2026-10-03T22:30:00Z").toEpochMilli()
        val ts = "2026-10-03T21:30:00Z"
        val sameDay = entryTime(ts, lateNow, ZoneOffset.UTC)!!
        val otherDay = entryTime(ts, lateNow, ZoneOffset.ofHours(2))!!
        assertTrue("$otherDay should carry the date: $sameDay", otherDay.length > sameDay.length)
    }

    @Test fun anUnreadableOneIsNull() {
        assertNull(entryTime("nope", now, zone))
        assertNull(entryTime("", now, zone))
    }
}
