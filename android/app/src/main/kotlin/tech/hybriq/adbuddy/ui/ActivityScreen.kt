package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
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

/** Every adb command of the agent, with the level the host gave it and how to undo it (spec section 6). */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ActivityScreen(state: BridgeState, onBack: () -> Unit) {
    var filter by rememberSaveable { mutableStateOf<String?>(null) }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.home_activity)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.action_back))
                    }
                },
            )
        },
    ) { inner ->
        Column(Modifier.padding(inner).padding(horizontal = 16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(selected = filter == null, onClick = { filter = null }, label = { Text(stringResource(R.string.filter_all)) })
                for (level in listOf("green", "yellow", "red")) {
                    FilterChip(selected = filter == level, onClick = { filter = level }, label = { Text(levelIcon(level)) })
                }
            }
            // distinctBy: LazyColumn keys must be unique, whatever the host sends.
            val entries = state.journal.asReversed().distinctBy { it.id }.filter { filter == null || it.level == filter }
            if (entries.isEmpty()) Text(stringResource(R.string.home_empty), style = MaterialTheme.typography.bodyMedium)
            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(entries, key = { it.id }) { EntryRow(it) }
            }
        }
    }
}

@Composable
private fun EntryRow(e: JournalEntry) {
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
                e.inverse?.takeIf { e.undoneBy == null }?.let {
                    Text(
                        stringResource(R.string.activity_inverse, it.argv.joinToString(" ")),
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
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
