package tech.hybriq.adbuddy.ui

import android.app.KeyguardManager
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.provider.Settings
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.delay
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.Bridge
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Decision
import tech.hybriq.adbuddy.bridge.JournalEntry
import tech.hybriq.adbuddy.confirm.ConfirmNotifier
import tech.hybriq.adbuddy.confirm.ConfirmUi
import tech.hybriq.adbuddy.confirm.HostClock
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
fun HomeScreen(
    state: BridgeState,
    bridge: Bridge,
    clock: HostClock,
    focusConfirm: String?,
    closed: Set<String>,
    onClose: (String) -> Unit,
) {
    val context = LocalContext.current
    var message by remember { mutableStateOf<String?>(null) }
    var failed by remember { mutableStateOf(false) }
    var filter by remember { mutableStateOf<String?>(null) }
    // Follows both settings, whoever changes them (this screen, the Settings app, adb).
    val cut by remember(context) { CutAccess.observe(context) }
        .collectAsStateWithLifecycle(initialValue = CutAccess.isCut(context.contentResolver))
    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(stringResource(R.string.home_title), style = MaterialTheme.typography.headlineSmall)
        val status = LinkStatus.of(state, cut)
        Text(if (status == LinkStatus.CONNECTED) stringResource(status.text, state.hostName) else stringResource(status.text))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            // What the host says (session.state), not whether the request was queued.
            PauseButton.of(state)?.let { button ->
                Button(onClick = { bridge.pause(button == PauseButton.PAUSE) }) { Text(stringResource(button.text)) }
            }
            val cutFailed = stringResource(R.string.cut_failed)
            val restoreFailed = stringResource(R.string.restore_failed)
            val cutRestored = stringResource(R.string.cut_restored)
            val cutDone = stringResource(R.string.cut_done)
            OutlinedButton(onClick = {
                val ok = if (cut) CutAccess.restore(context) else CutAccess.cut(context)
                failed = !ok
                message = when {
                    !ok && cut -> restoreFailed
                    !ok -> cutFailed
                    cut -> cutRestored
                    else -> cutDone
                }
            }) { Text(stringResource(if (cut) R.string.action_restore else R.string.action_cut)) }
        }
        message?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
        // Without the permission (or if the change did not stick), the person does it by hand.
        if (failed) {
            TextButton(onClick = { openDeveloperOptions(context) }) { Text(stringResource(R.string.action_developer_options)) }
        }

        Text(stringResource(R.string.home_activity), style = MaterialTheme.typography.titleMedium)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            FilterChip(selected = filter == null, onClick = { filter = null }, label = { Text(stringResource(R.string.filter_all)) })
            for (level in listOf("green", "yellow", "red")) {
                FilterChip(selected = filter == level, onClick = { filter = level }, label = { Text(levelIcon(level)) })
            }
        }
        // distinctBy: LazyColumn keys must be unique, whatever the host sends.
        val entries = state.journal.asReversed().distinctBy { it.id }.filter { filter == null || it.level == filter }
        if (entries.isEmpty()) Text(stringResource(R.string.home_empty))
        LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items(entries, key = { it.id }) { EntryRow(it) }
        }
    }
    // The clock the questions are judged by (the host's): it moves on at each
    // expiry, so a question left open on screen loses its buttons when it expires.
    var now by remember { mutableLongStateOf(clock.now()) }
    LaunchedEffect(state.pending) {
        while (true) {
            now = clock.now()
            delay(ConfirmUi.nextExpiry(state.pending, now) ?: break)
        }
    }
    val question = ConfirmUi.question(state, focusConfirm, now, closed)
    if (question != null) {
        ConfirmDialog(question, ConfirmUi.status(question, state, now), clock, onClose = { onClose(question.id) }) { decision ->
            // The same rule as the notification buttons (ConfirmReceiver), with the
            // current state and time, not those of this composition: a second quick
            // tap sees the first answer (answering), and the delay stops while the
            // phone sleeps. Allowing needs the phone unlocked here too.
            val t = clock.now()
            val locked = context.getSystemService(KeyguardManager::class.java).isDeviceLocked
            if (ConfirmUi.canSend(bridge.state.value, question.id, decision, t, locked)) {
                bridge.reply(question.id, decision)
            } else {
                now = t
            }
        }
    }
}

private fun openDeveloperOptions(context: Context) {
    val intent = Intent(Settings.ACTION_APPLICATION_DEVELOPMENT_SETTINGS).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    try {
        context.startActivity(intent)
    } catch (e: ActivityNotFoundException) {
        context.startActivity(Intent(Settings.ACTION_SETTINGS).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    }
}

/**
 * The same question as the notification, when the app is open (spec §4.5):
 * the host's rule first, then the agent's command, and it scrolls if long.
 * It offers its buttons only while the question is open, and says why
 * otherwise. It can always be closed (back or a tap outside; Close when it
 * has no buttons): Pause and Cut access are never hidden with no way out,
 * and the notification is still there. Touches are refused while another
 * app's window covers it (filterTouchesWhenObscured).
 */
@Composable
fun ConfirmDialog(r: ConfirmRequest, status: ConfirmUi.Status, clock: HostClock, onClose: () -> Unit, onAnswer: (String) -> Unit) {
    val context = LocalContext.current
    val decisions = ConfirmUi.decisions(r, status)
    val footer = ConfirmNotifier.footer(context, status, ConfirmUi.expiry(r)?.let { clock.toLocal(it) })
    val lines = ConfirmUi.lines(r, stringResource(R.string.confirm_special), footer, maxCommand = DIALOG_MAX_COMMAND)
    AlertDialog(
        onDismissRequest = onClose,
        // The title comes from the host's rules, but it never takes the whole dialog.
        title = { Text(stringResource(R.string.confirm_title, r.title), maxLines = 3, overflow = TextOverflow.Ellipsis) },
        text = {
            val view = LocalView.current
            DisposableEffect(view) {
                view.rootView.filterTouchesWhenObscured = true
                onDispose {}
            }
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                for ((i, line) in lines.withIndex()) {
                    val small = i >= lines.size - 2 // the command and the footer
                    Text(line, style = if (small) MaterialTheme.typography.bodySmall else MaterialTheme.typography.bodyMedium)
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
                if (decisions.isEmpty()) {
                    TextButton(onClick = onClose) { Text(stringResource(R.string.action_close)) }
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

/** The dialog scrolls, so it shows much more of the command than the notification. */
private const val DIALOG_MAX_COMMAND = 4_000

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
