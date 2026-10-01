package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.PermissionRequest
import tech.hybriq.adbuddy.chat.PermissionUi
import tech.hybriq.adbuddy.ui.theme.AdbuddyPreviews
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * The agent's own questions (Claude Code's permissions, spec M4 §7.3), above the input: the oldest one, with
 * its full text, and a count of the others. Not an ADBuddy 🔴 question (ConfirmDialog): the label says whose it is.
 * The card is not removed here: it goes when the host's permission.resolved arrives, wherever it was answered.
 */
@Composable
fun PermissionCards(
    permissions: List<PermissionRequest>,
    connected: Boolean,
    hostNow: () -> Long,
    onAnswer: (PermissionRequest, String) -> Unit,
    modifier: Modifier = Modifier,
) {
    if (permissions.isEmpty()) return
    var now by remember { mutableLongStateOf(hostNow()) }
    LaunchedEffect(permissions) {
        while (true) {
            now = hostNow()
            delay(PermissionUi.nextTickMs(permissions, now) ?: break)
        }
    }
    PermissionCard(permissions.first(), permissions.size - 1, connected, now, onAnswer, modifier)
}

@Composable
fun PermissionCard(
    p: PermissionRequest,
    others: Int,
    connected: Boolean,
    hostNow: Long,
    onAnswer: (PermissionRequest, String) -> Unit,
    modifier: Modifier = Modifier,
) {
    Surface(
        color = MaterialTheme.colorScheme.tertiaryContainer,
        contentColor = MaterialTheme.colorScheme.onTertiaryContainer,
        shape = MaterialTheme.shapes.large,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.tertiary),
        modifier = modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp),
    ) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(stringResource(R.string.perm_title), style = MaterialTheme.typography.labelMedium)
            Text(p.summary.ifEmpty { p.tool }, style = MaterialTheme.typography.titleMedium)
            if (p.detail.isNotEmpty() && p.detail != p.summary) {
                Surface(color = MaterialTheme.colorScheme.surface.copy(alpha = 0.5f), shape = MaterialTheme.shapes.small) {
                    Text(
                        p.detail,
                        Modifier.heightIn(max = 120.dp).verticalScroll(rememberScrollState()).horizontalScroll(rememberScrollState()).padding(8.dp),
                        style = MaterialTheme.typography.bodySmall,
                        fontFamily = FontFamily.Monospace,
                        softWrap = false,
                    )
                }
            }
            val left = PermissionUi.secondsLeft(p, hostNow)
            val status = when {
                !connected -> stringResource(R.string.perm_offline)
                left == 0L -> stringResource(R.string.perm_expired)
                left != null -> stringResource(R.string.perm_left, left)
                else -> null
            }
            if (status != null) Text(status, style = MaterialTheme.typography.labelSmall)
            if (others > 0) Text(pluralStringResource(R.plurals.perm_more, others, others), style = MaterialTheme.typography.labelSmall)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                val can = { d: String -> PermissionUi.canAnswer(p, d, connected, hostNow) }
                Button(onClick = { onAnswer(p, PermissionUi.ALLOW) }, enabled = can(PermissionUi.ALLOW)) { Text(stringResource(R.string.perm_allow)) }
                if (p.allowAlways) {
                    OutlinedButton(onClick = { onAnswer(p, PermissionUi.ALLOW_ALWAYS) }, enabled = can(PermissionUi.ALLOW_ALWAYS)) {
                        Text(stringResource(R.string.perm_always))
                    }
                }
                TextButton(onClick = { onAnswer(p, PermissionUi.DENY) }, enabled = can(PermissionUi.DENY)) { Text(stringResource(R.string.perm_deny)) }
            }
        }
    }
}

@AdbuddyPreviews
@Composable
private fun PermissionCardPreview() {
    AdbuddyTheme(dynamic = false) {
        PermissionCard(
            PermissionRequest("p1", "Bash", "Create the notes file", "touch notes.txt", "2026-10-01T10:10:00Z", true),
            others = 1, connected = true, hostNow = java.time.Instant.parse("2026-10-01T10:08:30Z").toEpochMilli(), onAnswer = { _, _ -> },
        )
    }
}
