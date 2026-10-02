package tech.hybriq.adbuddy.ui

import android.app.KeyguardManager
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.provider.Settings
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.Icon
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.ui.Alignment
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import tech.hybriq.adbuddy.bridge.Inverse
import tech.hybriq.adbuddy.ui.theme.AdbuddyPreviews
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme
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
import androidx.compose.ui.text.style.TextDirection
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

/** Consent for the PC to finish the setup (spec §3.1 step 6.1). Header, content, actions at the foot. */
@Composable
fun ConsentScreen(host: String, onAnswer: (Boolean) -> Unit) {
    Scaffold(
        bottomBar = {
            Surface(tonalElevation = 3.dp) {
                Row(
                    Modifier.fillMaxWidth().navigationBarsPadding().padding(horizontal = 24.dp, vertical = 12.dp),
                    horizontalArrangement = Arrangement.spacedBy(12.dp, Alignment.End),
                ) {
                    OutlinedButton(
                        onClick = { onAnswer(false) },
                        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outline),
                    ) { Text(stringResource(R.string.consent_decline), color = MaterialTheme.colorScheme.onSurface) }
                    Button(onClick = { onAnswer(true) }) { Text(stringResource(R.string.consent_accept)) }
                }
            }
        },
    ) { inner ->
        Column(
            Modifier.fillMaxSize().padding(inner).verticalScroll(rememberScrollState()).padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Box(
                Modifier.size(56.dp).clip(CircleShape).background(MaterialTheme.colorScheme.primaryContainer),
                contentAlignment = Alignment.Center,
            ) {
                Icon(Icons.Default.Lock, contentDescription = null, tint = MaterialTheme.colorScheme.onPrimaryContainer)
            }
            Text(stringResource(R.string.consent_title, host), style = MaterialTheme.typography.headlineSmall)
            Text(stringResource(R.string.consent_intro, host), style = MaterialTheme.typography.bodyLarge)
            for ((icon, item) in listOf(
                Icons.Default.Lock to R.string.consent_grant,
                Icons.Default.Notifications to R.string.consent_battery,
                Icons.Default.Settings to R.string.consent_a11y,
            )) {
                ConsentRow(icon, stringResource(item))
            }
            Surface(color = MaterialTheme.colorScheme.surfaceVariant, shape = MaterialTheme.shapes.medium) {
                Text(
                    stringResource(R.string.consent_note),
                    Modifier.padding(16.dp),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun ConsentRow(icon: ImageVector, text: String) {
    Row(horizontalArrangement = Arrangement.spacedBy(16.dp), verticalAlignment = Alignment.Top) {
        Icon(icon, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
        Text(text, style = MaterialTheme.typography.bodyMedium)
    }
}

@AdbuddyPreviews
@Composable
private fun ConsentScreenPreview() {
    AdbuddyTheme(dynamic = false) { ConsentScreen(host = "my-pc", onAnswer = {}) }
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
        title = {
            Column {
                // Whose question it is: ADBuddy's own (the shim), not the agent's (PermissionCard).
                Text(stringResource(R.string.confirm_label), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.error)
                Text(stringResource(R.string.confirm_title, r.title), maxLines = 3, overflow = TextOverflow.Ellipsis)
            }
        },
        text = {
            val view = LocalView.current
            DisposableEffect(view) {
                view.rootView.filterTouchesWhenObscured = true
                onDispose {}
            }
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                for ((i, line) in lines.withIndex()) {
                    val small = i >= lines.size - 2 // the command and the footer
                    // The agent's command (second to last) and the host's detail (first, when there is one) are code: left to right
                    // whatever their first letter, so what is read is in the order it runs. The labels follow the language.
                    val code = i == lines.size - 2 || (i == 0 && r.detail.isNotEmpty())
                    val base = if (small) MaterialTheme.typography.bodySmall else MaterialTheme.typography.bodyMedium
                    Text(line, style = if (code) base.copy(textDirection = TextDirection.Ltr) else base)
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
