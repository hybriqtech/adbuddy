package tech.hybriq.adbuddy.ui

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.provider.Settings
import android.widget.Toast
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
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.merge
import kotlinx.coroutines.launch
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.BridgePrefs
import tech.hybriq.adbuddy.bridge.Empty
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.chat.Composer
import tech.hybriq.adbuddy.control.CutAccess
import tech.hybriq.adbuddy.files.Downloads
import tech.hybriq.adbuddy.files.DownloadState
import tech.hybriq.adbuddy.files.FileIntents

/**
 * The app's screens: the first-run steps until they are done (only when firstRun: the chat bubble passes
 * false and never shows them), then the chat, the activity panel and Settings (with the protected apps and
 * the notification access under it), and the red questions over all of them.
 */
@Composable
fun ChatRoot(app: AdbuddyApp, focusConfirm: String?, closed: Set<String>, firstRun: Boolean = true, onClose: (String) -> Unit) {
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
    val onboarding = remember(context) { Onboarding.of(context) }
    var step by rememberSaveable { mutableStateOf<OnboardingStep?>(OnboardingStep.initial(firstRun, onboarding.done())) }
    var route by rememberSaveable { mutableStateOf(Route.CHAT) }
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
    // Unknown remote mode (null) counts as remote: the tighter limit, never a false "off".
    val remoteMode = RemoteUi.treatAsRemote(state.remote)
    LaunchedEffect(state.chat.messages, remoteMode) {
        for (m in state.chat.messages) {
            if (m.role != "assistant") continue
            for (a in m.attachments) {
                if (downloadsNow[a.id] == null && Downloads.autoLimit(a, remote = remoteMode)) app.downloads.get(a)
            }
        }
    }

    val actions = remember(app) {
        ChatActions(
            send = { text ->
                // Only what this message carries goes: a file picked after the button was drawn stays in the draft.
                val sent = app.drafts.items.value
                app.chat.send(text, sent.mapNotNull { it.attachment })
                sent.forEach { app.drafts.remove(it.key) }
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
            openActivity = { route = Route.ACTIVITY },
            openSettings = { route = Route.SETTINGS },
            newConversation = app.chat::newConversation,
            interrupt = { app.bridge.send(Protocol.AGENT_INTERRUPT, Protocol.json.encodeToJsonElement(Empty.serializer(), Empty)) },
        )
    }

    val holder = rememberSaveableStateHolder()
    val configured = remember(state.connection) { BridgePrefs(context).link != null }
    val back = { route = route.parent ?: Route.CHAT }
    val first = step
    if (first != null) {
        val finish = { onboarding.finish(); step = null }
        when (first) {
            OnboardingStep.PROTECTED_APPS ->
                ProtectedAppsScreen(app, first, onBack = { step = first.previous() }, onDone = { step = first.next() }, onSkip = { step = first.next() })
            OnboardingStep.NOTIFICATION_ACCESS ->
                NotificationAccessScreen(first, onBack = { step = first.previous() }, onDone = finish, onSkip = finish)
        }
    } else {
        when (route) {
            // The chat leaves the composition under Settings and Activity: its state (the text being written,
            // the terminal, the scroll) is kept aside and given back. Attachments and the session live in app.drafts/app.chat.
            Route.CHAT -> holder.SaveableStateProvider("chat") {
                ChatScreen(state, outgoing, drafts, downloads, cut, app.terminal, snackbar, actions, app.chat, app.clock::now, configured = configured) {
                    RemoteCard(state, app.bridge, app.clock, onMessage = { text -> scope.launch { snackbar.showSnackbar(text) } })
                }
            }
            Route.ACTIVITY -> {
                BackHandler(onBack = back)
                ActivityScreen(state, onBack = back)
            }
            Route.SETTINGS -> {
                BackHandler(onBack = back)
                SettingsScreen(app, state, onOpen = { route = it }, onBack = back, onMessage = { Toast.makeText(context, it, Toast.LENGTH_SHORT).show() })
            }
            Route.PROTECTED_APPS -> ProtectedAppsScreen(app, null, onBack = back, onDone = back, onSkip = back)
            Route.NOTIFICATION_ACCESS -> NotificationAccessScreen(null, onBack = back, onDone = back, onSkip = back)
        }
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
