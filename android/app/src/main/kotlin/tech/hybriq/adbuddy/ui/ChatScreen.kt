package tech.hybriq.adbuddy.ui

import android.content.ActivityNotFoundException
import android.content.Intent
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.core.net.toUri
import kotlinx.coroutines.launch
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ChatMessage
import tech.hybriq.adbuddy.bridge.ChatState
import tech.hybriq.adbuddy.chat.ChatController
import tech.hybriq.adbuddy.chat.Links
import tech.hybriq.adbuddy.chat.Outgoing
import tech.hybriq.adbuddy.chat.SessionLabel
import tech.hybriq.adbuddy.chat.Timeline
import tech.hybriq.adbuddy.chat.ToolStrip
import tech.hybriq.adbuddy.files.DownloadState
import tech.hybriq.adbuddy.files.Draft
import tech.hybriq.adbuddy.term.TerminalFeed
import tech.hybriq.adbuddy.term.TerminalView
import tech.hybriq.adbuddy.ui.theme.AdbuddyPreviews
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** What the screen can do; ChatRoot fills it from the app's controllers, a preview with empty lambdas. */
class ChatActions(
    val send: (String) -> Unit,
    val retry: (String) -> Unit,
    val discard: (String) -> Unit,
    val pickPhotos: () -> Unit,
    val pickFile: () -> Unit,
    val capture: () -> Unit,
    val removeDraft: (String) -> Unit,
    val retryDraft: (String) -> Unit,
    val attachment: (Attachment) -> Unit,
    val pause: (Boolean) -> Unit,
    val cut: () -> Unit,
    val restore: () -> Unit,
    val openActivity: () -> Unit,
    val openSettings: () -> Unit,
    val newConversation: () -> Unit,
    val interrupt: () -> Unit,
    val openYolo: () -> Unit = {},
) {
    companion object {
        val None = ChatActions({}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {})
    }

    /** Test seam: the same actions with another cut / restore. */
    fun copyWith(cut: () -> Unit = this.cut, restore: () -> Unit = this.restore) =
        ChatActions(send, retry, discard, pickPhotos, pickFile, capture, removeDraft, retryDraft, attachment, pause, cut, restore, openActivity, openSettings, newConversation, interrupt, openYolo)
}

/** The conversation, the agent's terminal folded above it, and the input (spec M3 section 7.2). */
@Composable
fun ChatScreen(
    state: BridgeState,
    outgoing: List<Outgoing>,
    drafts: List<Draft>,
    downloads: Map<String, DownloadState>,
    cut: Boolean,
    terminal: TerminalFeed?,
    snackbar: SnackbarHostState,
    actions: ChatActions,
    /** The agent's permission cards are the controller's: with none (a preview) there are none. */
    chat: ChatController? = null,
    hostNow: () -> Long = { 0L },
    /** The host has handed this phone a link: without it (and without a channel) the banner says to run adbuddy setup. */
    configured: Boolean = true,
    /** False in the app (R15) and in the bubble; the default stays true for direct callers and previews. The terminal starts folded and opens with the arrow. */
    terminalOpenByDefault: Boolean = true,
    /** Notifications off or the confirm/ask channel silenced: the banner says so and opens the system settings. */
    notificationsBlocked: Boolean = false,
    onOpenNotificationSettings: () -> Unit = {},
    /** Red questions the person closed and that can still be answered: the notice says so and "Show" reopens them. */
    pendingQuestions: Int = 0,
    onShowQuestions: () -> Unit = {},
) {
    val context = LocalContext.current
    // Read in composition (lint: configuration-aware), used by the "How to connect" button.
    val guideUrl = stringResource(R.string.setup_guide_url)
    var terminalShown by rememberSaveable { mutableStateOf(terminalOpenByDefault) }
    var text by rememberSaveable { mutableStateOf("") }
    var link by remember { mutableStateOf<Pair<String, String>?>(null) }
    var askNew by rememberSaveable { mutableStateOf(false) }
    var askRestore by rememberSaveable { mutableStateOf(false) }
    val items = remember(state.chat, state.sessionState.conversation, outgoing) { Timeline.of(state.chat, state.sessionState.conversation, outgoing) }
    val tools = remember(state.chat, state.sessionState.state) { ToolStrip.of(state.chat, state.sessionState.state) }
    val approvals = state.chat.yoloApprovals
    val listState = rememberLazyListState()
    // The strip is the last item: the list follows it too, so what the agent is doing is in view.
    val rows = items.size + (if (tools.isEmpty()) 0 else 1) + (if (approvals.isEmpty()) 0 else 1)
    // Follows what arrives only while the reader is at the end. New rows go below and do not move the viewport, so "was at
    // the end" is "the row that was last is (at least partly) in the viewport": a last bubble that grew in place now
    // ends below it, and its reader was at the end all the same. A drop in rows (the tool strip going) is not news, and
    // news is a new LAST MESSAGE (its key), not more rows: the strip appearing is not news and the strip going while a
    // message arrives (same row count) is. Coming back from Settings the chat recomposes with seen = 0 and jumps to
    // the end, as it already did.
    // Two changes in a row evaluate the viewport mid-animation: if the new row is already visible its index is >= seen - 1
    // and it counts as "at the end" (no state to get stuck); a reader who went up has a smaller last visible index.
    var seen by remember { mutableIntStateOf(0) }
    var seenKey by remember { mutableStateOf<String?>(null) }
    var unseen by remember { mutableStateOf(false) }
    // Outside the notice: hiding it must not cancel the scroll its tap started.
    val scope = rememberCoroutineScope()
    val lastKey = items.lastOrNull()?.key
    LaunchedEffect(rows, lastKey) {
        val info = listState.layoutInfo
        val wasAtEnd = seen == 0 || (info.visibleItemsInfo.lastOrNull()?.index ?: -1) >= seen - 1
        val grew = lastKey != null && lastKey != seenKey
        seen = rows // before animating: a cancelled animation does not leave the old count
        seenKey = lastKey
        if (rows == 0) return@LaunchedEffect
        if (wasAtEnd) {
            unseen = false
            listState.animateScrollToItem(rows - 1)
        } else if (grew) {
            unseen = true
        }
    }
    LaunchedEffect(listState) { snapshotFlow { !listState.canScrollForward }.collect { if (it) unseen = false } }
    val terminalOn = terminalShown && terminal != null
    val status = LinkStatus.of(state, cut, configured)
    // The dialog only exists while the state is CUT: if the cut ends elsewhere (Settings, the notification) it closes.
    LaunchedEffect(status) { if (status != LinkStatus.CUT) askRestore = false }
    // Reconnecting flips WAITING -> CONNECTING -> WAITING every retry: the banner keeps the last state that had one during
    // CONNECTING, so it neither leaves and returns (the list jumping, TalkBack announcing it again) nor shows on a cold start.
    var lastBanner by remember { mutableStateOf<LinkStatus?>(null) }
    val bannerStatus = when {
        status.bannerWorthy -> status
        status == LinkStatus.CONNECTING -> lastBanner
        else -> null
    }
    // Only what the retry really alternates (WAITING, NOT_SET_UP) is kept: a cut, an outdated side or a pause can end and
    // go straight to CONNECTING, and its banner would then say something that no longer holds.
    SideEffect { lastBanner = bannerStatus.takeIf { it == LinkStatus.WAITING || it == LinkStatus.NOT_SET_UP } }

    val connected = state.isConnected
    Scaffold(
        topBar = {
            ChatHeader(
                state, cut, terminalShown, onToggleTerminal = { terminalShown = !terminalShown }, onPause = actions.pause,
                onCut = actions.cut, onRestore = { askRestore = true }, onActivity = actions.openActivity, onSettings = actions.openSettings,
                onNewConversation = { askNew = true }, configured = configured,
                yoloOn = state.yolo?.active == true, onYolo = actions.openYolo,
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { inner ->
        Column(Modifier.padding(inner).consumeWindowInsets(inner).imePadding().fillMaxSize()) {
            HomeNotice.of(bannerStatus, pendingQuestions, notificationsBlocked)?.let { notice ->
                ActionCard(notice, pendingQuestions, onAction = {
                    when (notice) {
                        HomeNotice.QUESTIONS -> onShowQuestions()
                        HomeNotice.CUT -> askRestore = true
                        HomeNotice.NOTIFICATIONS -> onOpenNotificationSettings()
                        HomeNotice.PAUSED -> actions.pause(false)
                        HomeNotice.WAITING -> try {
                            context.startActivity(Intent(Intent.ACTION_VIEW, guideUrl.toUri()).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                        } catch (e: ActivityNotFoundException) { /* no browser */ }
                        else -> {}
                    }
                })
            }
            if (terminalOn) {
                Box(Modifier.weight(0.35f).fillMaxSize()) {
                    TerminalView(terminal!!, onLink = { u, d -> link = u to d }, Modifier.fillMaxSize())
                    if (!connected) TerminalOfflineNotice(Modifier.align(Alignment.TopCenter))
                }
            }
            val weight = if (terminalOn) 0.65f else 1f
            if (items.isEmpty() && tools.isEmpty() && approvals.isEmpty() && (status == LinkStatus.CONNECTED || status == LinkStatus.PAUSED)) {
                ChatEmptyState(paused = status == LinkStatus.PAUSED, onSuggestion = { text = it }, modifier = Modifier.weight(weight).fillMaxSize())
            } else {
                Box(Modifier.weight(weight).fillMaxSize()) {
                    LazyColumn(
                        state = listState,
                        modifier = Modifier.fillMaxSize(),
                        contentPadding = androidx.compose.foundation.layout.PaddingValues(12.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        items(items, key = { it.key }) { item ->
                            MessageRow(
                                item, downloads,
                                onLink = { u -> Links.domain(u)?.let { link = u to it } },
                                onAttachment = actions.attachment, onRetry = actions.retry, onDiscard = actions.discard,
                            )
                        }
                        if (tools.isNotEmpty()) item(key = "tools") { ToolStripView(tools) }
                        if (approvals.isNotEmpty()) item(key = "yolo_approvals") { YoloApprovals(approvals) }
                    }
                    if (unseen) {
                        FilledTonalButton(
                            onClick = { unseen = false; scope.launch { listState.animateScrollToItem(rows - 1) } },
                            modifier = Modifier.align(Alignment.BottomCenter).padding(bottom = 8.dp),
                        ) { Text(stringResource(R.string.chat_new_messages)) }
                    }
                }
            }
            if (terminalOn) QuickKeysRow(enabled = connected) { terminal!!.input(it.bytes) }
            if (chat != null) {
                PermissionCards(state.chat.permissions, state.isConnected, hostNow, chat, agent = state.sessionState.agent)
            }
            DraftChips(drafts, actions.removeDraft, actions.retryDraft)
            InputBar(
                text, { text = it }, drafts, actions.pickPhotos, actions.pickFile, actions.capture,
                onSend = { actions.send(text); text = "" },
                working = SessionLabel.of(state) == SessionLabel.WORKING, onStop = actions.interrupt,
            )
        }
    }

    link?.let { (url, domain) ->
        LinkDialog(
            url, domain,
            onOpen = {
                link = null
                val i = Intent(Intent.ACTION_VIEW, url.toUri()).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                try { context.startActivity(i) } catch (e: ActivityNotFoundException) { /* no browser: nothing to open it with */ }
            },
            onDismiss = { link = null },
        )
    }
    if (askRestore && status == LinkStatus.CUT) RestoreAccessDialog(onRestore = { askRestore = false; actions.restore() }, onDismiss = { askRestore = false })
    if (askNew) {
        GuardedAlertDialog(
            onDismissRequest = { askNew = false },
            title = { Text(stringResource(R.string.new_conversation_title)) },
            text = {
                Text(stringResource(R.string.new_conversation_body))
            },
            confirmButton = { TextButton(onClick = { askNew = false; actions.newConversation() }) { Text(stringResource(R.string.new_conversation_confirm)) } },
            dismissButton = { TextButton(onClick = { askNew = false }) { Text(stringResource(R.string.action_cancel)) } },
        )
    }
}

@AdbuddyPreviews
@Composable
private fun ChatScreenPreview() {
    AdbuddyTheme(dynamic = false) {
        ChatScreen(
            state = BridgeState(
                connection = BridgeState.Connection.CONNECTED,
                hostName = "my-pc",
                chat = ChatState(
                    messages = listOf(
                        ChatMessage("1", "user", "Turn the volume down"),
                        ChatMessage("2", "assistant", "Done. The media volume is now at 20%."),
                    ),
                ),
            ),
            outgoing = emptyList(), drafts = emptyList(), downloads = emptyMap(), cut = false, terminal = null,
            snackbar = remember { SnackbarHostState() }, actions = ChatActions.None,
        )
    }
}
