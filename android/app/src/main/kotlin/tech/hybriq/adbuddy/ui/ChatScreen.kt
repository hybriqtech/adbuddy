package tech.hybriq.adbuddy.ui

import android.content.ActivityNotFoundException
import android.content.Intent
import androidx.compose.foundation.interaction.DragInteraction
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
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
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
    val toggleCut: () -> Unit,
    val openActivity: () -> Unit,
    val openSettings: () -> Unit,
    val newConversation: () -> Unit,
) {
    companion object {
        val None = ChatActions({}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {})
    }
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
    /** The remote-mode switch (RemoteCard), shown above the conversation; a preview passes an empty one. */
    remoteCard: @Composable () -> Unit,
) {
    val context = LocalContext.current
    var terminalShown by rememberSaveable { mutableStateOf(true) }
    var text by rememberSaveable { mutableStateOf("") }
    var link by remember { mutableStateOf<Pair<String, String>?>(null) }
    var askNew by rememberSaveable { mutableStateOf(false) }
    val items = Timeline.of(state.chat, state.sessionState.conversation, outgoing)
    val tools = ToolStrip.of(state.chat, state.sessionState.state)
    val listState = rememberLazyListState()
    // The strip is the last item: the list follows it too, so what the agent is doing is in view.
    val rows = items.size + if (tools.isEmpty()) 0 else 1
    // Follows what arrives only while the reader is at the end. New rows go below and do not move the viewport, so "was at
    // the end" is "the row that was last is (at least partly) in the viewport": a last bubble that grew in place now
    // ends below it, and its reader was at the end all the same. A drop in rows (the tool strip going) is not news, and
    // news is a new LAST MESSAGE (its key), not more rows: the strip appearing is not news and the strip going while a
    // message arrives (same row count) is. Coming back from Settings the chat recomposes with seen = 0 and jumps to
    // the end, as it already did.
    // `following`: the effect decided to follow and its animation has not finished. Two changes in a row evaluate the
    // viewport while it is mid-animation, so while following the reader counts as at the end. It ends when the animation
    // completes or when the reader drags the list (a DragInteraction: programmatic scrolls emit none, unlike
    // isScrollInProgress, which an animation also sets).
    var seen by remember { mutableIntStateOf(0) }
    var seenKey by remember { mutableStateOf<String?>(null) }
    var following by remember { mutableStateOf(false) }
    var unseen by remember { mutableStateOf(false) }
    // Outside the notice: hiding it must not cancel the scroll its tap started.
    val scope = rememberCoroutineScope()
    val lastKey = items.lastOrNull()?.key
    LaunchedEffect(rows, lastKey) {
        val info = listState.layoutInfo
        val wasAtEnd = seen == 0 || following ||
            info.visibleItemsInfo.any { it.index == seen - 1 && it.offset < info.viewportEndOffset }
        val grew = lastKey != null && lastKey != seenKey
        seen = rows // before animating: a cancelled animation does not leave the old count
        seenKey = lastKey
        if (rows == 0) return@LaunchedEffect
        if (wasAtEnd) {
            unseen = false
            following = true
            listState.animateScrollToItem(rows - 1)
            following = false
        } else if (grew) {
            unseen = true
        }
    }
    LaunchedEffect(listState) { snapshotFlow { !listState.canScrollForward }.collect { if (it) unseen = false } }
    LaunchedEffect(listState) {
        listState.interactionSource.interactions.collect { if (it is DragInteraction.Start) following = false }
    }
    val terminalOn = terminalShown && terminal != null

    Scaffold(
        topBar = {
            ChatHeader(
                state, cut, terminalShown, onToggleTerminal = { terminalShown = !terminalShown }, onPause = actions.pause,
                onToggleCut = actions.toggleCut, onActivity = actions.openActivity, onSettings = actions.openSettings,
                onNewConversation = { askNew = true }, configured = configured,
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { inner ->
        Column(Modifier.padding(inner).consumeWindowInsets(inner).imePadding().fillMaxSize()) {
            LinkBanner(LinkStatus.of(state, cut, configured))
            remoteCard()
            if (terminalOn) {
                TerminalView(terminal!!, onLink = { u, d -> link = u to d }, Modifier.weight(0.45f).fillMaxSize())
            }
            val weight = if (terminalOn) 0.55f else 1f
            if (items.isEmpty() && tools.isEmpty() && LinkStatus.of(state, cut, configured) == LinkStatus.CONNECTED) {
                ChatEmptyState(Modifier.weight(weight).fillMaxSize())
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
                    }
                    if (unseen) {
                        FilledTonalButton(
                            onClick = { unseen = false; scope.launch { listState.animateScrollToItem(rows - 1) } },
                            modifier = Modifier.align(Alignment.BottomCenter).padding(bottom = 8.dp),
                        ) { Text(stringResource(R.string.chat_new_messages)) }
                    }
                }
            }
            if (terminalOn) QuickKeysRow { terminal!!.input(it.bytes) }
            if (chat != null) {
                PermissionCards(state.chat.permissions, state.connection == BridgeState.Connection.CONNECTED, hostNow, chat)
            }
            DraftChips(drafts, actions.removeDraft, actions.retryDraft)
            InputBar(
                text, { text = it }, drafts, actions.pickPhotos, actions.pickFile, actions.capture,
                onSend = { actions.send(text); text = "" },
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
    if (askNew) {
        AlertDialog(
            onDismissRequest = { askNew = false },
            title = { Text(stringResource(R.string.new_conversation_title)) },
            text = { Text(stringResource(R.string.new_conversation_body)) },
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
            remoteCard = {},
        )
    }
}
