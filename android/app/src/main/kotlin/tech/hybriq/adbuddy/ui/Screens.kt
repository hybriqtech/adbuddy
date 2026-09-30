package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.Bridge
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Decision
import tech.hybriq.adbuddy.bridge.JournalEntry
import tech.hybriq.adbuddy.confirm.ConfirmUi
import tech.hybriq.adbuddy.control.CutAccess

/** Consent for the PC to finish the setup (spec §3.1 step 6.1). */
@Composable
fun ConsentScreen(host: String, onAnswer: (Boolean) -> Unit) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(stringResource(R.string.consent_title, host), style = MaterialTheme.typography.headlineSmall)
        Text(stringResource(R.string.consent_intro, host))
        for (item in listOf(R.string.consent_grant, R.string.consent_battery, R.string.consent_a11y)) {
            Text("• " + stringResource(item))
        }
        Text(stringResource(R.string.consent_note), style = MaterialTheme.typography.bodySmall)
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            OutlinedButton(onClick = { onAnswer(false) }) { Text(stringResource(R.string.consent_decline)) }
            Button(onClick = { onAnswer(true) }) { Text(stringResource(R.string.consent_accept)) }
        }
    }
}

/** Connection, Pause, Cut access and the activity panel (spec §6). */
@Composable
fun HomeScreen(state: BridgeState, bridge: Bridge, focusConfirm: String?) {
    val context = LocalContext.current
    var message by remember { mutableStateOf<String?>(null) }
    var filter by remember { mutableStateOf<String?>(null) }
    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(stringResource(R.string.home_title), style = MaterialTheme.typography.headlineSmall)
        Text(
            when {
                state.connection == BridgeState.Connection.INCOMPATIBLE && state.outdated == BridgeState.Outdated.APP -> stringResource(R.string.status_update_app)
                state.connection == BridgeState.Connection.INCOMPATIBLE -> stringResource(R.string.status_update_host)
                state.sessionState.paused -> stringResource(R.string.status_paused)
                state.connection == BridgeState.Connection.CONNECTED -> stringResource(R.string.status_connected, state.hostName)
                state.connection == BridgeState.Connection.CONNECTING -> stringResource(R.string.status_connecting)
                else -> stringResource(R.string.status_waiting)
            },
        )
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            if (state.connection == BridgeState.Connection.CONNECTED) {
                // What the host says (session.state), not whether the request was queued.
                val paused = state.sessionState.paused
                Button(onClick = { bridge.pause(!paused) }) {
                    Text(stringResource(if (paused) R.string.action_resume else R.string.action_pause))
                }
            }
            val cut = CutAccess.isCut(context.contentResolver)
            val cutFailed = stringResource(R.string.cut_failed)
            val cutRestored = stringResource(R.string.cut_restored)
            val cutDone = stringResource(R.string.cut_done)
            OutlinedButton(onClick = {
                val ok = if (cut) CutAccess.restore(context.contentResolver) else CutAccess.cut(context.contentResolver)
                message = when {
                    !ok -> cutFailed
                    cut -> cutRestored
                    else -> cutDone
                }
            }) { Text(stringResource(if (cut) R.string.action_restore else R.string.action_cut)) }
        }
        message?.let { Text(it, style = MaterialTheme.typography.bodySmall) }

        Text(stringResource(R.string.home_activity), style = MaterialTheme.typography.titleMedium)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            FilterChip(selected = filter == null, onClick = { filter = null }, label = { Text(stringResource(R.string.filter_all)) })
            for (level in listOf("green", "yellow", "red")) {
                FilterChip(selected = filter == level, onClick = { filter = level }, label = { Text(levelIcon(level)) })
            }
        }
        val entries = state.journal.asReversed().filter { filter == null || it.level == filter }
        if (entries.isEmpty()) Text(stringResource(R.string.home_empty))
        LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items(entries, key = { it.id }) { EntryRow(it) }
        }
    }
    // The clock the questions are judged by: it moves on at each expiry, so a
    // question left open on screen loses its buttons when it expires.
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(state.pending) {
        while (true) {
            now = System.currentTimeMillis()
            delay(ConfirmUi.nextExpiry(state.pending, now) ?: break)
        }
    }
    // Expired questions the person closed: their final entry may be slow to come.
    var closed by remember { mutableStateOf(emptySet<String>()) }
    val open = state.copy(pending = state.pending.filter { it.id !in closed })
    val question = ConfirmUi.question(open, focusConfirm, now)
    if (question != null) {
        val status = ConfirmUi.status(question, question.id in state.answering, now)
        ConfirmDialog(
            question, status,
            onClose = { closed = closed + question.id },
        ) { decision ->
            // The clock may be late (the delay stops while the phone sleeps): check again.
            val t = System.currentTimeMillis()
            if (ConfirmUi.status(question, question.id in state.answering, t) == ConfirmUi.Status.OPEN) {
                bridge.reply(question.id, decision)
            } else {
                now = t
            }
        }
    }
}

/**
 * The same question as the notification, when the app is open (spec §4.5).
 * While its answer is being sent it shows "Sending…", and once expired
 * "Expired", both without buttons; an expired one can be closed (back or a
 * tap outside), so it never hides the screen while its final entry is late.
 */
@Composable
fun ConfirmDialog(r: ConfirmRequest, status: ConfirmUi.Status, onClose: () -> Unit, onAnswer: (String) -> Unit) {
    val decisions = ConfirmUi.decisions(r, status)
    AlertDialog(
        onDismissRequest = { if (status == ConfirmUi.Status.EXPIRED) onClose() },
        title = { Text(stringResource(R.string.confirm_title, r.title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(r.rawCommand, style = MaterialTheme.typography.bodySmall)
                if (r.detail.isNotEmpty()) Text(r.detail)
                if (r.special) Text("⚠ " + stringResource(R.string.confirm_special))
                when (status) {
                    ConfirmUi.Status.SENDING -> Text(stringResource(R.string.confirm_sending), style = MaterialTheme.typography.bodySmall)
                    ConfirmUi.Status.EXPIRED -> Text(stringResource(R.string.confirm_expired), style = MaterialTheme.typography.bodySmall)
                    ConfirmUi.Status.OPEN -> Unit
                }
            }
        },
        confirmButton = {
            Column {
                if (Decision.ALLOW in decisions) {
                    TextButton(onClick = { onAnswer(Decision.ALLOW) }) { Text(stringResource(R.string.action_allow)) }
                }
                if (Decision.ALLOW_SESSION in decisions) {
                    TextButton(onClick = { onAnswer(Decision.ALLOW_SESSION) }) { Text(stringResource(R.string.action_allow_session)) }
                }
            }
        },
        dismissButton = {
            if (Decision.DENY in decisions) {
                TextButton(onClick = { onAnswer(Decision.DENY) }) { Text(stringResource(R.string.action_deny)) }
            }
        },
    )
}

@Composable
private fun EntryRow(e: JournalEntry) {
    Column(Modifier.fillMaxWidth()) {
        Text("${levelIcon(e.level)} ${e.description}")
        Text("${e.status} · adb ${e.argv.joinToString(" ")}", style = MaterialTheme.typography.bodySmall)
        e.inverse?.takeIf { e.undoneBy == null }?.let {
            Text("↩ adb ${it.argv.joinToString(" ")}", style = MaterialTheme.typography.bodySmall)
        }
    }
}

private fun levelIcon(level: String) = when (level) {
    "green" -> "🟢"
    "yellow" -> "🟡"
    "red" -> "🔴"
    else -> "·"
}
