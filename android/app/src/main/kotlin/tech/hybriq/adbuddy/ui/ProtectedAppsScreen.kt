package tech.hybriq.adbuddy.ui

import android.widget.Toast
import androidx.activity.compose.BackHandler
import androidx.annotation.StringRes
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.Saver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.a11y.ProtectedApps
import tech.hybriq.adbuddy.a11y.UiHelpers
import tech.hybriq.adbuddy.ui.ProtectedAppsUi.Reason
import tech.hybriq.adbuddy.ui.theme.AdbuddyPreviews
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The selection survives a rotation; null is "still loading the apps". */
private val SelectionSaver = Saver<Set<String>?, ArrayList<String>>(save = { sel -> sel?.let { ArrayList(it) } }, restore = { it.toSet() })

/**
 * The user confirms or edits which apps are protected (M5c), in the first-run
 * steps (step != null) and in Settings. Saving makes the list theirs: the
 * proposals of M5a never touch it again, and the host gets it at once.
 */
@Composable
fun ProtectedAppsScreen(app: AdbuddyApp, step: OnboardingStep?, onBack: () -> Unit, onDone: () -> Unit, onSkip: () -> Unit) {
    val context = LocalContext.current
    val protectedApps = remember(context) { ProtectedApps.of(context) }
    // Another window drawn over the screen must not get the user's tap on a button that grants or saves.
    val view = LocalView.current
    DisposableEffect(view) {
        view.rootView.filterTouchesWhenObscured = true
        onDispose {}
    }
    var load by remember { mutableStateOf<ProtectedAppsUi.LoadState>(ProtectedAppsUi.LoadState.Loading) }
    var attempt by rememberSaveable { mutableIntStateOf(0) }
    var selection by rememberSaveable(stateSaver = SelectionSaver) { mutableStateOf<Set<String>?>(null) }
    var query by rememberSaveable { mutableStateOf("") }
    var askDiscard by rememberSaveable { mutableStateOf(false) }

    // Everything the system and the prefs give is read off the main thread; the screen is not usable until it is all there.
    LaunchedEffect(attempt) {
        load = ProtectedAppsUi.LoadState.Loading
        load = withContext(Dispatchers.IO) {
            ProtectedAppsUi.load {
                val sms = ProtectedApps.defaultSms(context)
                ProtectedAppsUi.Loaded(ProtectedApps.launcherApps(context), protectedApps.list(), protectedApps.confirmed, ProtectedApps.Heuristic.receivers(sms))
            }
        }
    }

    val ready = (load as? ProtectedAppsUi.LoadState.Ready)?.data
    val confirmed = ready?.confirmed ?: false
    // What is checked when the screen opens: the saved list, or the proposal while it is not confirmed.
    val initial = remember(ready) { ready?.let { ProtectedAppsUi.initialSelection(it.installed, it.saved, it.confirmed, it.receivers) } }
    LaunchedEffect(initial) {
        if (selection == null && initial != null) selection = initial
    }

    val current = selection
    val dirty = ready != null && current != null && ProtectedAppsUi.dirty(current, ready.saved)
    val rows = remember(ready, initial, current, query) {
        if (ready == null || current == null || initial == null) emptyList()
        else ProtectedAppsUi.rows(ready.installed, initial, current, ready.receivers, query)
    }
    val savedSent = stringResource(R.string.protected_saved_sent)
    val savedLater = stringResource(R.string.protected_saved_later)
    fun save() {
        if (ready == null) return
        val list = ProtectedAppsUi.result(selection ?: return)
        protectedApps.confirm(list)
        val sent = UiHelpers.publishProtectedApps(app)
        Toast.makeText(context, if (sent) savedSent else savedLater, if (sent) Toast.LENGTH_SHORT else Toast.LENGTH_LONG).show()
        onDone()
    }

    if (step == null) BackHandler { if (dirty) askDiscard = true else onBack() } else BackHandler(enabled = step.previous() != null, onBack = onBack)

    ProtectedAppsContent(
        step = step,
        loading = load is ProtectedAppsUi.LoadState.Loading || (ready != null && current == null),
        error = load is ProtectedAppsUi.LoadState.Error,
        onRetry = { attempt++ },
        rows = rows,
        selectedCount = current?.size ?: 0,
        proposal = !confirmed,
        query = query,
        onQuery = { query = it },
        onToggle = { pkg, on -> selection = ProtectedAppsUi.toggle(selection.orEmpty(), pkg, on) },
        canSave = ProtectedAppsUi.canSave(ready != null && current != null, step != null, dirty, confirmed),
        onSave = ::save,
        onBack = { if (dirty) askDiscard = true else onBack() },
        onSkip = onSkip,
    )

    if (askDiscard) {
        AlertDialog(
            onDismissRequest = { askDiscard = false },
            title = { Text(stringResource(R.string.protected_discard_title)) },
            text = { Text(stringResource(R.string.protected_discard_body)) },
            confirmButton = { TextButton(onClick = { askDiscard = false; onBack() }) { Text(stringResource(R.string.protected_discard_confirm)) } },
            dismissButton = { TextButton(onClick = { askDiscard = false }) { Text(stringResource(R.string.action_cancel)) } },
        )
    }
}

/** The screen without state, so it has previews: header, search and the rows, with its actions at the foot. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ProtectedAppsContent(
    step: OnboardingStep?,
    loading: Boolean,
    error: Boolean,
    onRetry: () -> Unit,
    rows: List<ProtectedAppsUi.Row>,
    selectedCount: Int,
    proposal: Boolean,
    query: String,
    onQuery: (String) -> Unit,
    onToggle: (String, Boolean) -> Unit,
    canSave: Boolean,
    onSave: () -> Unit,
    onBack: () -> Unit,
    onSkip: () -> Unit,
) {
    Scaffold(
        topBar = {
            if (step == null) {
                TopAppBar(
                    title = { Text(stringResource(R.string.protected_title)) },
                    navigationIcon = {
                        IconButton(onClick = onBack) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.action_back))
                        }
                    },
                )
            }
        },
        bottomBar = {
            Surface(tonalElevation = 3.dp) {
                // A column, so that large text never squeezes the buttons: the count, then the actions.
                Column(
                    Modifier.fillMaxWidth().navigationBarsPadding().padding(horizontal = 24.dp, vertical = 12.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    // Unknown (still loading) is not "0 apps": nothing is said until the list is there.
                    if (!loading && !error) {
                        Text(
                            stringResource(R.string.protected_count, selectedCount),
                            style = MaterialTheme.typography.labelLarge,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp, Alignment.End), verticalAlignment = Alignment.CenterVertically) {
                        if (step != null) OutlinedButton(onClick = onSkip, border = BorderStroke(1.dp, MaterialTheme.colorScheme.outline)) { Text(stringResource(R.string.onboarding_skip), color = MaterialTheme.colorScheme.onSurface) }
                        Button(onClick = onSave, enabled = canSave) {
                            Text(stringResource(if (proposal) R.string.protected_confirm else R.string.protected_save))
                        }
                    }
                }
            }
        },
    ) { inner ->
        LazyColumn(Modifier.fillMaxSize().padding(inner), contentPadding = PaddingValues(bottom = 16.dp)) {
            item {
                Column(Modifier.padding(horizontal = 24.dp, vertical = 8.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    if (step != null) StepHeader(step, R.string.protected_title)
                    Text(stringResource(R.string.protected_intro), style = MaterialTheme.typography.bodyMedium)
                    if (proposal) {
                        Surface(color = MaterialTheme.colorScheme.secondaryContainer, shape = MaterialTheme.shapes.medium) {
                            Text(
                                stringResource(R.string.protected_proposal),
                                Modifier.padding(16.dp),
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSecondaryContainer,
                            )
                        }
                    }
                    OutlinedTextField(
                        value = query,
                        onValueChange = onQuery,
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true,
                        leadingIcon = { Icon(Icons.Default.Search, contentDescription = null) },
                        label = { Text(stringResource(R.string.protected_search)) },
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                    )
                }
            }
            when {
                error -> item {
                    Column(Modifier.padding(24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                        Text(stringResource(R.string.protected_error), style = MaterialTheme.typography.bodyMedium)
                        Button(onClick = onRetry) { Text(stringResource(R.string.chat_retry)) }
                    }
                }
                loading -> item { Text(stringResource(R.string.protected_loading), Modifier.padding(24.dp), style = MaterialTheme.typography.bodyMedium) }
                rows.isEmpty() -> item { Text(stringResource(R.string.protected_empty), Modifier.padding(24.dp), style = MaterialTheme.typography.bodyMedium) }
                else -> items(rows, key = { it.pkg }) { row ->
                    ListItem(
                        modifier = Modifier.toggleable(value = row.checked, role = Role.Checkbox, onValueChange = { onToggle(row.pkg, it) }),
                        leadingContent = { Checkbox(checked = row.checked, onCheckedChange = null) },
                        headlineContent = { Text(row.knownName ?: row.label, maxLines = 2, overflow = TextOverflow.Ellipsis) },
                        supportingContent = {
                            Column {
                                Text(row.pkg, style = MaterialTheme.typography.labelSmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                when (row.reason) {
                                    Reason.CODES -> Text(stringResource(R.string.protected_reason_codes), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.primary)
                                    Reason.GUESS -> Text(stringResource(R.string.protected_reason_guess), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.primary)
                                    Reason.NONE -> Unit
                                }
                            }
                        },
                    )
                }
            }
        }
    }
}

/** The icon (the lock unless the step brings its own), "Step n of N" and the title that open a first-run step; shared with the notification access screen. */
@Composable
fun StepHeader(step: OnboardingStep, @StringRes title: Int, icon: ImageVector = Icons.Default.Lock) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Box(
            Modifier.size(56.dp).clip(CircleShape).background(MaterialTheme.colorScheme.primaryContainer),
            contentAlignment = Alignment.Center,
        ) {
            Icon(icon, contentDescription = null, tint = MaterialTheme.colorScheme.onPrimaryContainer)
        }
        Text(
            stringResource(R.string.step_progress, step.number, OnboardingStep.total),
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(stringResource(title), style = MaterialTheme.typography.headlineSmall)
    }
}

private val sampleRows = listOf(
    ProtectedAppsUi.Row("com.google.android.gm", "Gmail", true, Reason.CODES),
    ProtectedAppsUi.Row("com.example.mybank", "Mi Banco", true, Reason.GUESS),
    ProtectedAppsUi.Row("com.whatsapp", "WhatsApp", false, Reason.NONE),
)

@AdbuddyPreviews
@Composable
private fun ProtectedAppsOnboardingPreview() {
    AdbuddyTheme(dynamic = false) {
        ProtectedAppsContent(OnboardingStep.PROTECTED_APPS, false, false, {}, sampleRows, 2, true, "", {}, { _, _ -> }, true, {}, {}, {})
    }
}

@AdbuddyPreviews
@Composable
private fun ProtectedAppsSettingsPreview() {
    AdbuddyTheme(dynamic = false) {
        ProtectedAppsContent(null, false, false, {}, sampleRows, 2, false, "", {}, { _, _ -> }, false, {}, {}, {})
    }
}
