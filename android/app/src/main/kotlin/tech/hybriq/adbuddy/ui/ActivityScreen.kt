package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.Inverse
import tech.hybriq.adbuddy.bridge.JournalEntry
import tech.hybriq.adbuddy.ui.theme.AdbuddyPreviews
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme
import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/** Every adb command of the agent, with the level the host gave it and how to undo it (spec section 6). */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun ActivityScreen(state: BridgeState, onBack: () -> Unit, now: () -> Long = { System.currentTimeMillis() }) {
    var filter by rememberSaveable { mutableStateOf<String?>(null) }
    Scaffold(
        topBar = {
            AdbuddyTopBar(stringResource(R.string.home_activity), onBack)
        },
    ) { inner ->
        Column(Modifier.padding(inner).padding(horizontal = 16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(selected = filter == null, onClick = { filter = null }, label = { Text(stringResource(R.string.filter_all)) })
                for (level in listOf("green", "yellow", "red")) {
                    FilterChip(selected = filter == level, onClick = { filter = level }, label = { Text(stringResource(R.string.level_chip, levelIcon(level), levelName(level))) })
                }
            }
            // distinctBy: LazyColumn keys must be unique, whatever the host sends.
            val entries = state.journal.asReversed().distinctBy { it.id }.filter { filter == null || it.level == filter }
            if (entries.isEmpty()) {
                val text = if (state.journal.isEmpty()) R.string.home_empty else R.string.activity_filter_empty
                Text(stringResource(text), style = MaterialTheme.typography.bodyMedium)
            }
            val nowMs = now()
            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(entries, key = { it.id }) { EntryRow(it, nowMs) }
            }
        }
    }
}

@Composable
private fun EntryRow(e: JournalEntry, now: Long) {
    ElevatedCard(Modifier.fillMaxWidth()) {
        Row(Modifier.padding(12.dp), horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.Top) {
            Text(levelIcon(e.level), style = MaterialTheme.typography.titleMedium)
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(e.description, style = MaterialTheme.typography.bodyLarge)
                Text(
                    stringResource(R.string.activity_entry, statusText(e.status), e.argv.joinToString(" ")),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                entryTime(e.ts, now)?.let {
                    Text(it, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                e.inverse?.takeIf { e.undoneBy == null }?.let {
                    Text(
                        stringResource(R.string.activity_inverse, it.argv.joinToString(" ")),
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    // Without the id, `adbuddy undo` undoes the LAST entry, not this one. The id may carry text the agent
                    // influences: only the shape the host gives is drawn.
                    if (e.status == "ok" && JOURNAL_ID.matches(e.id)) {
                        Text(
                            stringResource(R.string.activity_undo_hint, e.id),
                            style = MaterialTheme.typography.labelMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        }
    }
}

/** The journal's status word (running, ok, error, denied, blocked, timeout) in the language in force; one this app does not know shows as it came. */
@Composable
private fun statusText(status: String): String = when (status) {
    "running" -> stringResource(R.string.activity_status_running)
    "ok" -> stringResource(R.string.activity_status_ok)
    "error" -> stringResource(R.string.activity_status_error)
    "denied" -> stringResource(R.string.activity_status_denied)
    "blocked" -> stringResource(R.string.activity_status_blocked)
    "timeout" -> stringResource(R.string.activity_status_timeout)
    else -> status
}

private val JOURNAL_ID = Regex("^j_[0-9A-Za-z]+$")

/** The entry's time in local time: only the hour if it is from today (by [now] and [zone]), date and hour if not; null if [ts] cannot be read. */
internal fun entryTime(ts: String, now: Long = System.currentTimeMillis(), zone: ZoneId = ZoneId.systemDefault()): String? =
    runCatching { OffsetDateTime.parse(ts).toInstant().toEpochMilli() }.getOrNull()?.let { ms ->
        val today = Instant.ofEpochMilli(now).atZone(zone).toLocalDate() == Instant.ofEpochMilli(ms).atZone(zone).toLocalDate()
        if (today) Formats.time(ms, FormatStyle.SHORT, zone)
        else DateTimeFormatter.ofLocalizedDateTime(FormatStyle.SHORT).withZone(zone).format(Instant.ofEpochMilli(ms))
    }

@Composable
private fun levelName(level: String): String = when (level) {
    "green" -> stringResource(R.string.level_green)
    "yellow" -> stringResource(R.string.level_yellow)
    "red" -> stringResource(R.string.level_red)
    else -> level
}

private fun levelIcon(level: String) = when (level) {
    "green" -> "🟢"
    "yellow" -> "🟡"
    "red" -> "🔴"
    else -> "·"
}

@AdbuddyPreviews
@Composable
private fun ActivityScreenPreview() {
    AdbuddyTheme(dynamic = false) {
        ActivityScreen(
            BridgeState(
                journal = listOf(
                    JournalEntry(id = "1", level = "green", description = "Read the battery level", status = "done", argv = listOf("shell", "dumpsys", "battery")),
                    JournalEntry(
                        id = "2", level = "yellow", description = "Change the screen timeout", status = "done",
                        argv = listOf("shell", "settings", "put", "system", "screen_off_timeout", "60000"),
                        inverse = Inverse(listOf("shell", "settings", "put", "system", "screen_off_timeout", "30000")),
                    ),
                ),
            ),
            onBack = {},
        )
    }
}
