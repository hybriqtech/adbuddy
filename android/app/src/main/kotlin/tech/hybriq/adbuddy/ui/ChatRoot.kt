package tech.hybriq.adbuddy.ui

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.provider.Settings
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.SnackbarResult
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.merge
import kotlinx.coroutines.launch
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.chat.Composer
import tech.hybriq.adbuddy.control.CutAccess
import tech.hybriq.adbuddy.files.Downloads
import tech.hybriq.adbuddy.files.DownloadState
import tech.hybriq.adbuddy.files.FileIntents

/** The app's main screen: the chat, the activity panel, and the red questions over both. */
@Composable
fun ChatRoot(app: AdbuddyApp, focusConfirm: String?, closed: Set<String>, onClose: (String) -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val state by app.bridge.state.collectAsStateWithLifecycle()
    val outgoing by app.chat.outgoing.collectAsStateWithLifecycle()
    val drafts by app.drafts.items.collectAsStateWithLifecycle()
    val downloads by app.downloads.states.collectAsStateWithLifecycle()
    // Follows both settings, whoever changes them (this screen, the Settings app, adb).
    val cut by remember(context) { CutAccess.observe(context) }
        .collectAsStateWithLifecycle(initialValue = CutAccess.isCut(context.contentResolver))
    val snackbar = remember { SnackbarHostState() }
    var showActivity by rememberSaveable { mutableStateOf(false) }
    val attachFailed = stringResource(R.string.attach_failed)
    val cutFailed = stringResource(R.string.cut_failed)
    val restoreFailed = stringResource(R.string.restore_failed)
    val cutRestored = stringResource(R.string.cut_restored)
    val cutDone = stringResource(R.string.cut_done)
    val developerOptions = stringResource(R.string.action_developer_options)

    // What the host says (notice) and what it refuses (chat, uploads), as the host wrote it.
    LaunchedEffect(app) {
        merge(app.notices, app.chat.errors, app.drafts.errors).collect { snackbar.showSnackbar(it) }
    }

    // Photos and files go through the system pickers: no storage permission asked.
    fun attach(uri: Uri) = app.attachUri(uri) { text -> scope.launch { snackbar.showSnackbar(text.ifEmpty { attachFailed }) } }
    val photos = rememberLauncherForActivityResult(ActivityResultContracts.PickMultipleVisualMedia(Composer.MAX_ATTACHMENTS)) { uris -> uris.forEach(::attach) }
    val files = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri -> if (uri != null) attach(uri) }

    // Images the agent sends download by themselves (up to the limit of the mode); the rest waits for a tap.
    val downloadsNow by rememberUpdatedState(downloads)
    LaunchedEffect(state.chat.messages, state.remote.enabled) {
        for (m in state.chat.messages) {
            if (m.role != "assistant") continue
            for (a in m.attachments) {
                if (downloadsNow[a.id] == null && Downloads.autoLimit(a, remote = state.remote.enabled)) app.downloads.get(a)
            }
        }
    }

    val actions = remember(app) {
        ChatActions(
            send = { text ->
                app.chat.send(text, app.drafts.items.value.mapNotNull { it.attachment })
                app.drafts.clear()
            },
            retry = app.chat::retry,
            discard = app.chat::discard,
            pickPhotos = { photos.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)) },
            pickFile = { files.launch(arrayOf("*/*")) },
            capture = app.drafts::capture,
            removeDraft = app.drafts::remove,
            retryDraft = app.drafts::retry,
            attachment = { a -> onAttachment(context, app, a) },
            pause = app.bridge::pause,
            toggleCut = {
                val wasCut = CutAccess.isCut(context.contentResolver)
                val ok = if (wasCut) CutAccess.restore(context) else CutAccess.cut(context)
                val message = when {
                    !ok && wasCut -> restoreFailed
                    !ok -> cutFailed
                    wasCut -> cutRestored
                    else -> cutDone
                }
                scope.launch {
                    // Without the permission (or if the change did not stick), the person does it by hand.
                    val r = snackbar.showSnackbar(message, actionLabel = if (ok) null else developerOptions)
                    if (r == SnackbarResult.ActionPerformed) openDeveloperOptions(context)
                }
            },
            openActivity = { showActivity = true },
            newConversation = app.chat::newConversation,
        )
    }

    if (showActivity) {
        BackHandler { showActivity = false }
        ActivityScreen(state, onBack = { showActivity = false })
    } else {
        ChatScreen(state, outgoing, drafts, downloads, cut, app.terminal, snackbar, actions)
    }
    // After both screens, so a question shows wherever the person is.
    ConfirmHost(state, app.bridge, app.clock, focusConfirm, closed, onClose)
}

/** Tap on an attachment: download it, cancel the download, or open the file (and share it when no app opens it). */
private fun onAttachment(context: Context, app: AdbuddyApp, a: Attachment) {
    when (val d = app.downloads.states.value[a.id]) {
        null, is DownloadState.Failed -> app.downloads.get(a)
        is DownloadState.Running -> app.downloads.cancel(a.id)
        is DownloadState.Done -> if (!FileIntents.view(context, d.file, a.mime)) FileIntents.share(context, d.file, a.mime)
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
