package tech.hybriq.adbuddy.ui

import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.util.Locale

/** Times as the language in force writes them: "14:05:09" in Spanish, "2:05:09 PM" in English. */
object Formats {
    /** A number of seconds as "m:ss": 42 is "0:42", 125 is "2:05", 3600 is "60:00". */
    fun countdown(seconds: Long): String = "%d:%02d".format(Locale.ROOT, seconds / 60, seconds % 60)

    fun time(epochMs: Long, style: FormatStyle, zone: ZoneId = ZoneId.systemDefault(), locale: Locale = Locale.getDefault()): String =
        DateTimeFormatter.ofLocalizedTime(style).withLocale(locale).withZone(zone).format(Instant.ofEpochMilli(epochMs))
}
