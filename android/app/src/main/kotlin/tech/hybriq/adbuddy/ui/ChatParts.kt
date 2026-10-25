package tech.hybriq.adbuddy.ui

import android.graphics.BitmapFactory
import android.util.Base64
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.AssistChip
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withLink
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ChatMessage
import tech.hybriq.adbuddy.chat.Composer
import tech.hybriq.adbuddy.chat.Links
import tech.hybriq.adbuddy.chat.Outgoing
import tech.hybriq.adbuddy.chat.SendStatus
import tech.hybriq.adbuddy.chat.SessionLabel
import tech.hybriq.adbuddy.chat.TimelineItem
import tech.hybriq.adbuddy.files.DownloadState
import tech.hybriq.adbuddy.files.Draft
import tech.hybriq.adbuddy.files.DraftStatus
import tech.hybriq.adbuddy.term.QuickKey
import tech.hybriq.adbuddy.ui.theme.AdbuddyPreviews
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme
import tech.hybriq.adbuddy.ui.theme.BubbleShapes

/**
 * The chat's bar: Material 3's TopAppBar has a fixed height and does not clip its title, so a title and a pill that wrap at
 * large text stick out of it. This one is at least as tall as the standard bar and grows with its content; the status bar's
 * inset goes on top as in TopAppBar. TOP_BAR_TAG marks it for the screen tests.
 */
@Composable
private fun ChatHeaderBar(title: @Composable () -> Unit, actions: @Composable RowScope.() -> Unit) {
    Surface(color = MaterialTheme.colorScheme.surface, modifier = Modifier.fillMaxWidth().testTag(TOP_BAR_TAG)) {
        Row(
            Modifier.windowInsetsPadding(TopAppBarDefaults.windowInsets).heightIn(min = AdbuddyTopBarSpec.NORMAL).padding(start = 16.dp, end = 4.dp, top = 4.dp, bottom = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(Modifier.weight(1f).padding(end = 4.dp)) { title() }
            actions()
        }
    }
}

/** Header: the agent (and what it does, only on a live, unpaused link), the link's pill; the shield (always), terminal, menu. */
@Composable
fun ChatHeader(
    state: BridgeState,
    cut: Boolean,
    terminalShown: Boolean,
    onToggleTerminal: () -> Unit,
    onPause: (Boolean) -> Unit,
    onCut: () -> Unit,
    onRestore: () -> Unit,
    onActivity: () -> Unit,
    onSettings: () -> Unit,
    onNewConversation: () -> Unit,
    configured: Boolean = true,
    yoloOn: Boolean = false,
    onYolo: () -> Unit = {},
) {
    var menu by remember { mutableStateOf(false) }
    val status = LinkStatus.of(state, cut, configured)
    val agent = state.sessionState.agent.ifEmpty { stringResource(R.string.app_name) }
    // The agent's state only on a live, unpaused link: never "Ready" beside a pause or a cut (bug 9).
    val agentState = when (SessionLabel.of(state).takeIf { status == LinkStatus.CONNECTED }) {
        SessionLabel.WORKING -> R.string.session_working
        SessionLabel.WAITING -> R.string.session_waiting
        SessionLabel.IDLE -> R.string.session_idle
        SessionLabel.STOPPED -> R.string.session_stopped
        else -> null
    }
    val title = if (agentState == null) agent else stringResource(R.string.title_agent_state, agent, stringResource(agentState))
    ChatHeaderBar(
        title = {
            // Wraps (title up to 3 lines, the pill freely) instead of cutting: at 200 % text the room beside three buttons is narrow.
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(title, style = MaterialTheme.typography.titleLarge, maxLines = 3, overflow = TextOverflow.Ellipsis)
                StatusPill(status, state.hostName, remote = RemoteUi.treatAsRemote(state.remote))
            }
        },
        actions = {
            // Always here (spec §5): cutting is the real guarantee and never hides in a menu. Two actions, never a toggle.
            val isCut = status == LinkStatus.CUT
            // No covered-touch filter here: a cut forced by a covered tap is harmless; restoring goes through a dialog with its guard.
            IconButton(onClick = if (isCut) onRestore else onCut) {
                Icon(
                    painterResource(if (isCut) R.drawable.ic_shield_off else R.drawable.ic_shield),
                    contentDescription = stringResource(if (isCut) R.string.action_restore else R.string.action_cut),
                    tint = if (isCut) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.error,
                )
            }
            IconButton(onClick = onToggleTerminal) {
                Icon(
                    if (terminalShown) Icons.Default.KeyboardArrowUp else Icons.Default.KeyboardArrowDown,
                    contentDescription = stringResource(if (terminalShown) R.string.chat_terminal_hide else R.string.chat_terminal_show),
                )
            }
            Box {
                IconButton(onClick = { menu = true }) {
                    Icon(Icons.Default.MoreVert, contentDescription = stringResource(R.string.action_menu))
                }
                DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                    ObscuredTouchGuard()
                    PauseButton.of(state)?.let { button ->
                        DropdownMenuItem(text = { Text(stringResource(button.text)) }, onClick = { menu = false; onPause(button == PauseButton.PAUSE) })
                    }
                    // Always, and it only cuts (cut() is idempotent): the safe way out never changes meaning.
                    DropdownMenuItem(text = { Text(stringResource(R.string.action_cut)) }, onClick = { menu = false; onCut() })
                    DropdownMenuItem(text = { Text(stringResource(R.string.menu_activity)) }, onClick = { menu = false; onActivity() })
                    DropdownMenuItem(text = { Text(stringResource(R.string.menu_settings)) }, onClick = { menu = false; onSettings() })
                    DropdownMenuItem(
                        text = { Text(stringResource(if (yoloOn) R.string.yolo_menu_off else R.string.yolo_menu_on)) },
                        onClick = { menu = false; onYolo() },
                    )
                    DropdownMenuItem(text = { Text(stringResource(R.string.menu_new_conversation)) }, onClick = { menu = false; onNewConversation() })
                }
            }
        },
    )
}

/** One message: mine to the right, the agent's to the left; text with links, attachment cards, and Retry for a failed one. */
@Composable
fun MessageRow(
    item: TimelineItem,
    downloads: Map<String, DownloadState>,
    onLink: (String) -> Unit,
    onAttachment: (Attachment) -> Unit,
    onRetry: (String) -> Unit,
    onDiscard: (String) -> Unit,
) {
    when (item) {
        is TimelineItem.Host -> Bubble(item.msg.role == "user", item.msg.text, item.msg.attachments, downloads, onLink, onAttachment, markdown = item.msg.role == "assistant")
        is TimelineItem.Mine -> {
            val o = item.out
            val failed = o.status == SendStatus.FAILED
            Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.End) {
                Bubble(
                    mine = true, text = o.text, attachments = o.attachments, downloads = downloads, onLink = onLink, onAttachment = onAttachment,
                    failed = failed, modifier = Modifier.alpha(if (failed) 1f else 0.7f),
                )
                if (failed) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(stringResource(R.string.chat_not_sent), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.error)
                        TextButton(onClick = { onRetry(o.id) }) { Text(stringResource(R.string.chat_retry)) }
                        TextButton(onClick = { onDiscard(o.id) }) { Text(stringResource(R.string.chat_discard)) }
                    }
                } else {
                    Text(stringResource(R.string.chat_sending), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
    }
}

@Composable
private fun Bubble(
    mine: Boolean,
    text: String,
    attachments: List<Attachment>,
    downloads: Map<String, DownloadState>,
    onLink: (String) -> Unit,
    onAttachment: (Attachment) -> Unit,
    modifier: Modifier = Modifier,
    failed: Boolean = false,
    markdown: Boolean = false,
) {
    val container = when {
        failed -> MaterialTheme.colorScheme.errorContainer
        mine -> MaterialTheme.colorScheme.primaryContainer
        else -> MaterialTheme.colorScheme.surfaceContainerHigh
    }
    val content = when {
        failed -> MaterialTheme.colorScheme.onErrorContainer
        mine -> MaterialTheme.colorScheme.onPrimaryContainer
        else -> MaterialTheme.colorScheme.onSurface
    }
    Row(Modifier.fillMaxWidth(), horizontalArrangement = if (mine) Arrangement.End else Arrangement.Start) {
        Surface(color = container, contentColor = content, shape = if (mine) BubbleShapes.mine else BubbleShapes.agent, modifier = modifier.widthIn(max = 340.dp)) {
            Column(Modifier.padding(horizontal = 12.dp, vertical = 8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (text.isNotEmpty()) if (markdown) MarkdownText(text, onLink) else LinkedText(text, onLink)
                for (a in attachments) AttachmentCard(a, downloads[a.id]) { onAttachment(a) }
            }
        }
    }
}

/** The text with its links as tappable spans (LinkAnnotation.Clickable; Links.find); a tap asks onLink(url), which shows the domain first. */
@Composable
fun LinkedText(text: String, onLink: (String) -> Unit, modifier: Modifier = Modifier) {
    val linkStyle = TextLinkStyles(SpanStyle(color = MaterialTheme.colorScheme.primary, textDecoration = TextDecoration.Underline))
    val annotated = remember(text, linkStyle) {
        buildAnnotatedString {
            var at = 0
            for (s in Links.find(text)) {
                append(text.substring(at, s.start))
                withLink(LinkAnnotation.Clickable(tag = s.url, styles = linkStyle, linkInteractionListener = { onLink(s.url) })) {
                    append(text.substring(s.start, s.end))
                }
                at = s.end
            }
            append(text.substring(at))
        }
    }
    Text(annotated, modifier, style = MaterialTheme.typography.bodyLarge)
}

/** A file: name, size, thumbnail (the base64 JPEG of Attachment.thumb, decoded with BitmapFactory, never a remote image) and its state: tap to download / progress / tap to open. */
@Composable
fun AttachmentCard(a: Attachment, state: DownloadState?, onTap: () -> Unit) {
    // Never a remote image: the thumbnail is the bytes the host sent, or nothing.
    val thumb = remember(a.thumb) {
        if (a.thumb.isEmpty()) null else try {
            val b = Base64.decode(a.thumb, Base64.DEFAULT)
            BitmapFactory.decodeByteArray(b, 0, b.size)?.asImageBitmap()
        } catch (e: IllegalArgumentException) {
            null
        }
    }
    val tapLabel = stringResource(
        when (state) {
            null -> R.string.file_action_download
            is DownloadState.Running -> R.string.file_action_cancel
            is DownloadState.Done -> R.string.file_action_open
            is DownloadState.Failed -> R.string.file_action_retry
        },
    )
    Surface(
        color = MaterialTheme.colorScheme.surface,
        contentColor = MaterialTheme.colorScheme.onSurface,
        shape = MaterialTheme.shapes.medium,
        modifier = Modifier.fillMaxWidth().clickable(onClickLabel = tapLabel, role = Role.Button, onClick = onTap),
    ) {
        Column {
            if (thumb != null) {
                Image(thumb, contentDescription = a.name, contentScale = ContentScale.Fit, modifier = Modifier.fillMaxWidth().heightIn(max = 200.dp))
            }
            Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(a.name, style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium, maxLines = 2, overflow = TextOverflow.Ellipsis)
                Text(android.text.format.Formatter.formatShortFileSize(LocalContext.current, a.size), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                when (state) {
                    null -> Text(stringResource(R.string.file_tap_download), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.primary)
                    is DownloadState.Running -> {
                        val p = if (state.size > 0) (state.done.toFloat() / state.size).coerceIn(0f, 1f) else 0f
                        LinearProgressIndicator(progress = { p }, modifier = Modifier.fillMaxWidth())
                        Text(stringResource(R.string.file_downloading, (p * 100).toInt()), style = MaterialTheme.typography.labelMedium)
                        Text(stringResource(R.string.file_tap_cancel), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.primary)
                    }
                    is DownloadState.Done -> Text(stringResource(R.string.file_open), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.primary)
                    // The host's own text when it refused; a lost connection has none, and tapping goes on.
                    is DownloadState.Failed -> Text(
                        state.text.ifEmpty { stringResource(R.string.file_failed) },
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
        }
    }
}

@Composable
fun DraftChips(drafts: List<Draft>, onRemove: (String) -> Unit, onRetry: (String) -> Unit) {
    if (drafts.isEmpty()) return
    Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 12.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        for (d in drafts) {
            val failed = d.status == DraftStatus.FAILED
            Surface(
                shape = MaterialTheme.shapes.small,
                color = if (failed) MaterialTheme.colorScheme.errorContainer else MaterialTheme.colorScheme.surfaceVariant,
                contentColor = if (failed) MaterialTheme.colorScheme.onErrorContainer else MaterialTheme.colorScheme.onSurfaceVariant,
            ) {
                Row(Modifier.padding(start = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                    // The retry tap is on the label only: the X is its sibling, so tapping it can never retry.
                    Column(
                        Modifier.widthIn(max = 180.dp)
                            .then(if (failed) Modifier.clickable(role = Role.Button, onClick = { onRetry(d.key) }) else Modifier)
                            .padding(vertical = 8.dp),
                    ) {
                        Text(d.name, style = MaterialTheme.typography.labelLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        if (failed) {
                            Text(
                                d.error.ifEmpty { stringResource(R.string.draft_failed) },
                                style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.error,
                                maxLines = 3, overflow = TextOverflow.Ellipsis,
                            )
                        }
                        if (d.status == DraftStatus.UPLOADING) {
                            // Size 0 is a file still being prepared (or a screenshot being taken): an unknown amount, not an empty one.
                            if (d.size > 0) LinearProgressIndicator(progress = { (d.progress.toFloat() / d.size).coerceIn(0f, 1f) }, modifier = Modifier.fillMaxWidth(), trackColor = MaterialTheme.colorScheme.surface)
                            else LinearProgressIndicator(modifier = Modifier.fillMaxWidth(), trackColor = MaterialTheme.colorScheme.surface)
                        }
                    }
                    IconButton(onClick = { onRemove(d.key) }, modifier = Modifier.size(48.dp)) {
                        Icon(Icons.Default.Close, contentDescription = stringResource(R.string.chat_discard))
                    }
                }
            }
        }
    }
}

/**
 * + (photo, file, screenshot), the text field (up to 5 lines) and Send; Send is `enabled = Composer.canSend(text, drafts)`.
 * While the agent works ([working]) and nothing is being written (no text, no drafts), the button is Stop instead and calls
 * [onStop]; it then rests a few seconds, because agent.interrupt has no reply and a second tap would send a second Esc.
 */
@Composable
fun InputBar(
    text: String,
    onText: (String) -> Unit,
    drafts: List<Draft>,
    onPhoto: () -> Unit,
    onFile: () -> Unit,
    onCapture: () -> Unit,
    onSend: () -> Unit,
    working: Boolean = false,
    onStop: () -> Unit = {},
) {
    var menu by remember { mutableStateOf(false) }
    Row(Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp), verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        Box {
            IconButton(onClick = { menu = true }) { Icon(Icons.Default.Add, contentDescription = stringResource(R.string.chat_attach)) }
            DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                ObscuredTouchGuard()
                DropdownMenuItem(text = { Text(stringResource(R.string.chat_photo)) }, onClick = { menu = false; onPhoto() })
                DropdownMenuItem(text = { Text(stringResource(R.string.chat_file)) }, onClick = { menu = false; onFile() })
                DropdownMenuItem(text = { Text(stringResource(R.string.chat_capture)) }, onClick = { menu = false; onCapture() })
            }
        }
        OutlinedTextField(
            value = text,
            onValueChange = onText,
            modifier = Modifier.weight(1f),
            placeholder = { Text(stringResource(R.string.chat_hint)) },
            maxLines = 5,
            shape = MaterialTheme.shapes.extraLarge,
        )
        val canSend = Composer.canSend(text, drafts)
        val stopLabel = stringResource(R.string.chat_stop)
        // Anything being written (text, or a draft even if still uploading) keeps Send: Stop is only for an empty bar, so a person
        // composing never interrupts the agent by mistake.
        val hasContent = text.isNotBlank() || drafts.isNotEmpty()
        // After a tap the button rests a moment: agent.interrupt has no reply, so a second tap would send a second Esc.
        var stopping by remember(working) { mutableStateOf(false) }
        LaunchedEffect(stopping) { if (stopping) { delay(3_000); stopping = false } }
        if (working && !hasContent) {
            FilledIconButton(onClick = { stopping = true; onStop() }, enabled = !stopping, modifier = Modifier.semantics { contentDescription = stopLabel }) {
                // A square, the usual "stop": Icons.Default.Close already means "discard" in the draft chips, and Stop is in icons-extended (not in the app).
                Box(Modifier.size(14.dp).background(LocalContentColor.current))
            }
        } else {
            FilledIconButton(onClick = onSend, enabled = canSend) {
                Icon(Icons.AutoMirrored.Filled.Send, contentDescription = stringResource(R.string.chat_send))
            }
        }
    }
}

/**
 * Esc, Ctrl+C, up, down, Tab, Enter, 1, 2, 3 (QuickKey.entries), horizontally scrollable. Each key refuses a tap while
 * another app's window covers it ("1" would answer "1. Yes" to the agent's permission prompt on the PC); the rest of
 * the chat keeps working under the overlay.
 */
@Composable
fun QuickKeysRow(enabled: Boolean = true, onKey: (QuickKey) -> Unit) {
    Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 8.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        for (k in QuickKey.entries) AssistChip(onClick = { onKey(k) }, enabled = enabled, modifier = Modifier.refuseObscuredTouches(), label = { Text(k.label) })
    }
}

/** Over the terminal while there is no channel to the PC: what is typed would not arrive. */
@Composable
fun TerminalOfflineNotice(modifier: Modifier = Modifier) {
    Surface(
        color = MaterialTheme.colorScheme.errorContainer, contentColor = MaterialTheme.colorScheme.onErrorContainer,
        modifier = modifier.fillMaxWidth().semantics { liveRegion = LiveRegionMode.Polite },
    ) {
        Text(stringResource(R.string.terminal_offline), style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(horizontal = 12.dp, vertical = 4.dp))
    }
}

/** "Open <domain>?": the domain in bold and the full URL under it, Open and Cancel. */
@Composable
fun LinkDialog(url: String, domain: String, onOpen: () -> Unit, onDismiss: () -> Unit) {
    GuardedAlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.link_open_title, domain), fontWeight = FontWeight.Bold) },
        text = {
            Text(url, style = MaterialTheme.typography.bodySmall, maxLines = 6, overflow = TextOverflow.Ellipsis)
        },
        confirmButton = { TextButton(onClick = onOpen) { Text(stringResource(R.string.link_open)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } },
    )
}

private val sampleAttachment = Attachment("0".repeat(32), "report.pdf", "application/pdf", 245_000, "x")

@AdbuddyPreviews
@Composable
private fun ChatHeaderPreview() {
    AdbuddyTheme(dynamic = false) {
        ChatHeader(
            BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc"), cut = false, terminalShown = true,
            onToggleTerminal = {}, onPause = {}, onCut = {}, onRestore = {}, onActivity = {}, onSettings = {}, onNewConversation = {},
        )
    }
}

@AdbuddyPreviews
@Composable
private fun MessageRowPreview() {
    AdbuddyTheme(dynamic = false) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            MessageRow(TimelineItem.Host(ChatMessage("1", "user", "Turn the volume down")), emptyMap(), {}, {}, {}, {})
            MessageRow(
                TimelineItem.Host(ChatMessage("2", "assistant", "Done. See https://example.com/volume for details.", listOf(sampleAttachment))),
                emptyMap(), {}, {}, {}, {},
            )
            MessageRow(TimelineItem.Mine(Outgoing("c1", "Sending now", emptyList(), SendStatus.SENDING)), emptyMap(), {}, {}, {}, {})
            MessageRow(TimelineItem.Mine(Outgoing("c2", "Could not go", emptyList(), SendStatus.FAILED)), emptyMap(), {}, {}, {}, {})
        }
    }
}

@AdbuddyPreviews
@Composable
private fun LinkedTextPreview() {
    AdbuddyTheme(dynamic = false) { LinkedText("Open https://example.com/a, then (https://b.org).", onLink = {}, modifier = Modifier.padding(12.dp)) }
}

@AdbuddyPreviews
@Composable
private fun AttachmentCardPreview() {
    AdbuddyTheme(dynamic = false) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            AttachmentCard(sampleAttachment, null) {}
            AttachmentCard(sampleAttachment, DownloadState.Running(100_000, 245_000)) {}
            AttachmentCard(sampleAttachment, DownloadState.Failed("")) {}
        }
    }
}

@AdbuddyPreviews
@Composable
private fun DraftChipsPreview() {
    AdbuddyTheme(dynamic = false) {
        DraftChips(
            listOf(
                Draft("a", "photo.jpg", "image/jpeg", 1000, 400, DraftStatus.UPLOADING),
                Draft("b", "notes.txt", "text/plain", 10, 0, DraftStatus.FAILED),
            ),
            onRemove = {}, onRetry = {},
        )
    }
}

@AdbuddyPreviews
@Composable
private fun InputBarPreview() {
    AdbuddyTheme(dynamic = false) { InputBar("Hello", {}, emptyList(), {}, {}, {}, {}) }
}

@AdbuddyPreviews
@Composable
private fun QuickKeysRowPreview() {
    AdbuddyTheme(dynamic = false) { QuickKeysRow {} }
}

@AdbuddyPreviews
@Composable
private fun LinkDialogPreview() {
    AdbuddyTheme(dynamic = false) { LinkDialog("https://example.com/some/long/path?q=1", "example.com", {}, {}) }
}
