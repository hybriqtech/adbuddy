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
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDirection
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.DialogProperties
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
    ObscuredTouchGuard()
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
 * the ⚠ warning first, the host's rule, then the agent's command, and it
 * scrolls if long. The whole detail and up to ConfirmUi.DIALOG_MAX_COMMAND
 * characters of the command are shown, with how many more are hidden (then it
 * can only be denied, H3), and the Allow buttons wait until the text has been
 * scrolled to its end (A4-2), as in the PermissionCard, and until [armed]
 * (ConfirmHost: ConfirmUi.ARMING_MS since the question appeared or the app came
 * back to the front; meanwhile they keep their text and read as disabled). Deny
 * never waits. It offers its buttons only while
 * the question is open, and says why otherwise. It can always be closed with Back (and Close when it has no
 * buttons), not with a tap outside: a stray tap no longer hides a question; the chat keeps a notice that reopens
 * it. Pause and Cut access are never hidden with no way out, and the notification is still there. Touches
 * are refused while another app's window covers it (ObscuredTouchGuard).
 */
@Composable
fun ConfirmDialog(
    r: ConfirmRequest,
    status: ConfirmUi.Status,
    clock: HostClock,
    armed: Boolean,
    now: Long = clock.now(),
    onClose: () -> Unit,
    onAnswer: (String) -> Unit,
) {
    val context = LocalContext.current
    val decisions = ConfirmUi.decisions(r, status)
    val footer = ConfirmNotifier.footer(context, status, ConfirmUi.expiry(r)?.let { clock.toLocal(it) })
    val shown = ConfirmUi.shown(r)
    // The scroll state starts "can scroll forward" until the text is measured: Allow stays off until it is known there is no more to read.
    val scroll = key(r.id, r.detail, r.rawCommand) { rememberScrollState() }
    val unread = scroll.canScrollForward
    AlertDialog(
        onDismissRequest = onClose,
        // Back closes it; a tap outside does not (the chat's notice reopens it, but a stray tap hides nothing).
        properties = DialogProperties(dismissOnClickOutside = false),
        // The title comes from the host's rules, but it never takes the whole dialog.
        title = {
            Column {
                // Whose question it is: ADBuddy's own (the shim), not the agent's (PermissionCard).
                Text(stringResource(R.string.confirm_label), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.error)
                Text(stringResource(R.string.confirm_title, r.title), maxLines = 3, overflow = TextOverflow.Ellipsis)
            }
        },
        text = {
            ObscuredTouchGuard()
            Column {
                Column(Modifier.weight(1f, fill = false).verticalScroll(scroll), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (shown.warning) {
                        Text("⚠ " + stringResource(R.string.confirm_special), style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Bold)
                    }
                    // The host's detail and the agent's command are code: left to right whatever their first letter, so
                    // what is read is in the order it runs. The labels follow the language.
                    for (line in shown.detail) {
                        Text(line, style = MaterialTheme.typography.bodyMedium.copy(textDirection = TextDirection.Ltr))
                    }
                    Text(shown.command, style = MaterialTheme.typography.bodySmall.copy(textDirection = TextDirection.Ltr))
                    if (shown.hidden > 0) {
                        // H3: what is not shown cannot have been read: only Deny (ConfirmUi.decisions), and it says so.
                        Text(
                            pluralStringResource(R.plurals.confirm_hidden, shown.hidden, shown.hidden) + " " +
                                stringResource(R.string.confirm_too_long),
                            style = MaterialTheme.typography.labelMedium, fontWeight = FontWeight.Bold,
                        )
                    }
                    Text(footer, style = MaterialTheme.typography.bodySmall)
                }
            // Fixed lines, outside the part that scrolls: always in sight. No live region: it would be read out every second.
            val left = if (status == ConfirmUi.Status.OPEN) ConfirmUi.secondsLeft(r, now) else null
            if (left != null) {
                val urgent = left < 10
                Text(
                    stringResource(R.string.confirm_expires_in, Formats.countdown(left)),
                    modifier = Modifier.padding(top = 8.dp),
                    style = MaterialTheme.typography.labelMedium,
                    fontWeight = if (urgent) FontWeight.Bold else null,
                    color = if (urgent) MaterialTheme.colorScheme.error else Color.Unspecified,
                )
            }
            if (unread && Decision.ALLOW in decisions) {
                Text(
                    stringResource(R.string.confirm_scroll), style = MaterialTheme.typography.labelMedium,
                    fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 8.dp),
                )
            }
            }
        },
        confirmButton = {
            Column {
                if (Decision.ALLOW in decisions) {
                    TextButton(onClick = { onAnswer(Decision.ALLOW) }, enabled = !unread && armed) { Text(stringResource(R.string.action_allow)) }
                }
                if (Decision.ALLOW_SESSION in decisions) {
                    TextButton(onClick = { onAnswer(Decision.ALLOW_SESSION) }, enabled = !unread && armed) {
                        Text(stringResource(R.string.action_allow_session))
                    }
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
