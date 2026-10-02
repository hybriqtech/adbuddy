package tech.hybriq.adbuddy.ui

import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.util.Locale

/** Times as the language in force writes them: "14:05:09" in Spanish, "2:05:09 PM" in English. */
object Formats {
    fun time(epochMs: Long, style: FormatStyle, zone: ZoneId = ZoneId.systemDefault(), locale: Locale = Locale.getDefault()): String =
        DateTimeFormatter.ofLocalizedTime(style).withLocale(locale).withZone(zone).format(Instant.ofEpochMilli(epochMs))
}
