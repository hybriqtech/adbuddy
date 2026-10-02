package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.BorderStroke
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
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDirection
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.PermissionRequest
import tech.hybriq.adbuddy.chat.ChatController
import tech.hybriq.adbuddy.chat.PermissionUi
import tech.hybriq.adbuddy.ui.theme.AdbuddyPreviews
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * The agent's own questions (Claude Code's permissions, spec M4 §7.3), above the input: the oldest one, with
 * its full text, and a count of the others. Not an ADBuddy 🔴 question (ConfirmDialog): the label says whose it is.
 * The card is not removed here: it goes when the host's permission.resolved arrives, wherever it was answered.
 * Whether it may be answered is [ChatController]'s (canAnswer, guardLeftMs): this only re-reads it when something changes
 * (the list, the connection, an answer on its way) and while a second or the promote guard runs out.
 */
@Composable
fun PermissionCards(
    permissions: List<PermissionRequest>,
    connected: Boolean,
    hostNow: () -> Long,
    chat: ChatController,
    modifier: Modifier = Modifier,
) {
    if (permissions.isEmpty()) return
    val answeringIds by chat.answering.collectAsStateWithLifecycle()
    val first = permissions.first()
    // The controller says when the guard restarts (a new first card, a reconnection), after it has done it: no race to wait out.
    val epoch by chat.guardEpoch.collectAsStateWithLifecycle()
    var now by remember { mutableLongStateOf(hostNow()) }
    var guardLeft by remember(first.id, epoch) { mutableLongStateOf(chat.guardLeftMs(first.id)) }
    var canAnswer by remember(first.id, epoch) { mutableStateOf(chat.canAnswer(first.id)) }
    LaunchedEffect(permissions, connected, answeringIds, epoch) {
        while (true) {
            now = hostNow()
            guardLeft = chat.guardLeftMs(first.id)
            canAnswer = chat.canAnswer(first.id)
            delay(PermissionUi.nextTickMs(permissions, now, guardLeft) ?: break)
        }
    }
    PermissionCard(first, permissions.size - 1, connected, now, first.id in answeringIds, guardLeft, canAnswer, modifier) { p, d -> chat.answerPermission(p.id, d) }
}

/** The detail may be this tall on screen; taller, and the person must scroll to its end before any button works. */
private val DETAIL_MAX_HEIGHT = 120.dp

@Composable
fun PermissionCard(
    p: PermissionRequest,
    others: Int,
    connected: Boolean,
    hostNow: Long,
    answering: Boolean,
    guardLeftMs: Long,
    /** ChatController.canAnswer(p.id): the first card, not answered, past its guard. The card does not work this out. */
    controllerCanAnswer: Boolean,
    modifier: Modifier = Modifier,
    onAnswer: (PermissionRequest, String) -> Unit,
) {
    Surface(
        color = MaterialTheme.colorScheme.tertiaryContainer,
        contentColor = MaterialTheme.colorScheme.onTertiaryContainer,
        shape = MaterialTheme.shapes.large,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.tertiary),
        modifier = modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp),
    ) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            // Whose question it is outweighs what it says: the label is bold and larger, the agent's own words are marked as such.
            Text(stringResource(R.string.perm_title), style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.tertiary)
            Text(stringResource(R.string.perm_summary, p.summary.ifEmpty { p.tool }), style = MaterialTheme.typography.bodyMedium.copy(textDirection = TextDirection.Ltr))
            // The scroll state starts "can scroll forward" until the text is measured: the buttons stay off until it is known there is no more to read.
            val scroll = key(p.id, p.detail) { rememberScrollState() }
            val hasDetail = p.detail.isNotEmpty() && p.detail != p.summary
            val unread = hasDetail && scroll.canScrollForward
            if (hasDetail) {
                Surface(color = MaterialTheme.colorScheme.surface.copy(alpha = 0.5f), shape = MaterialTheme.shapes.small) {
                    // Wrapped (never sideways: no text hides to the right) and never taller than the cap without a way to read the rest.
                    Text(
                        p.detail,
                        Modifier.heightIn(max = DETAIL_MAX_HEIGHT).verticalScroll(scroll).padding(8.dp),
                        style = MaterialTheme.typography.bodySmall.copy(textDirection = TextDirection.Ltr),
                        fontFamily = FontFamily.Monospace,
                        softWrap = true,
                    )
                }
                if (unread) Text(stringResource(R.string.perm_scroll), style = MaterialTheme.typography.labelMedium, fontWeight = FontWeight.Bold)
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
                // Only the grants wait for the whole text; No is always the safe answer, and keeps the guard and the controller's say.
                val can = { d: String ->
                    controllerCanAnswer && (d == PermissionUi.DENY || !unread) && PermissionUi.canAnswer(p, d, connected, hostNow, answering, guardLeftMs)
                }
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
            others = 1, connected = true, hostNow = java.time.Instant.parse("2026-10-01T10:08:30Z").toEpochMilli(),
            answering = false, guardLeftMs = 0, controllerCanAnswer = true, onAnswer = { _, _ -> },
        )
    }
}
