package tech.hybriq.adbuddy.ui

import androidx.activity.compose.BackHandler
import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Button
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.listener.NotificationAccess
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import tech.hybriq.adbuddy.listener.NotificationAccess.Effective
import tech.hybriq.adbuddy.listener.NotificationAccess.Listed
import tech.hybriq.adbuddy.listener.NotificationAccess.State
import tech.hybriq.adbuddy.listener.NotificationAccessStatus
import tech.hybriq.adbuddy.listener.NotificationConsent
import tech.hybriq.adbuddy.ui.theme.AdbuddyPreviews
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * Whether ADBuddy may read notifications, the shortcut to the system's own screen where the user allows it,
 * and the user's own confirmation (M5c): the first-run step (step != null) and Settings. The system's state
 * has three values and an unknown one is never shown as allowed nor as denied: it starts unknown until the
 * system answers. Allowed in the system is not enough for the agent: the person confirms here, with a tap
 * that nothing else makes for them (NotificationConsent); if the system revokes the access, the mark goes too.
 */
@Composable
fun NotificationAccessScreen(step: OnboardingStep?, onBack: () -> Unit, onDone: () -> Unit, onSkip: () -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    // Another window drawn over the screen must not get the user's tap on a button that grants or saves.
    val view = LocalView.current
    DisposableEffect(view) {
        view.rootView.filterTouchesWhenObscured = true
        onDispose {}
    }
    val consent = remember(context) { NotificationConsent.of(context) }
    // Refreshing the system's answer after a click that could not confirm: the flow is read again.
    var refresh by remember { mutableIntStateOf(0) }
    // Null until the system / the file answer: nothing is shown as allowed or denied before that. The mark is
    // observed (the listener clears it behind the screen's back) and read again each time the screen starts.
    val systemState by remember(context, refresh) { NotificationAccessStatus.observe(context) }.collectAsStateWithLifecycle(initialValue = null)
    val markState by remember(consent) { consent.observe() }.collectAsStateWithLifecycle(initialValue = null)
    val state = systemState ?: State.UNKNOWN
    val confirmed = markState == true
    // Observing never confirms; it only takes the mark away when the system says the access is gone.
    LaunchedEffect(systemState, markState) {
        if (systemState == State.NOT_GRANTED && markState == true) withContext(Dispatchers.IO) { consent.revoke() }
    }
    if (step == null) BackHandler(onBack = onBack) else BackHandler(enabled = step.previous() != null, onBack = onBack)
    NotificationAccessContent(
        step = step,
        checking = systemState == null || markState == null,
        confirmed = confirmed,
        effective = NotificationAccess.effective(state, confirmed),
        onOpen = { NotificationAccessStatus.openSystemSettings(context) },
        onConfirm = {
            // The only place that sets the mark: a tap, with the system saying "allowed" now, at the click.
            if (state == State.GRANTED) {
                scope.launch {
                    val allowedNow = withContext(Dispatchers.IO) { NotificationAccessStatus.listed(context) == Listed.YES }
                    if (allowedNow) withContext(Dispatchers.IO) { consent.confirm() } else refresh++
                }
            }
        },
        onWithdraw = { scope.launch { withContext(Dispatchers.IO) { consent.revoke() } } },
        onBack = onBack,
        onDone = onDone,
        onSkip = onSkip,
    )
}

/** The screen without state, so it has previews. */
@Composable
fun NotificationAccessContent(
    step: OnboardingStep?,
    checking: Boolean,
    confirmed: Boolean,
    effective: Effective,
    onOpen: () -> Unit,
    onConfirm: () -> Unit,
    onWithdraw: () -> Unit,
    onBack: () -> Unit,
    onDone: () -> Unit,
    onSkip: () -> Unit,
) {
    Scaffold(
        topBar = {
            if (step == null) {
                AdbuddyTopBar(stringResource(R.string.access_title), onBack)
            }
        },
        bottomBar = {
            if (step != null) {
                Surface(tonalElevation = 3.dp) {
                    ActionFooter(
                        modifier = Modifier.navigationBarsPadding().padding(horizontal = 24.dp, vertical = 12.dp),
                        main = { m ->
                            // Nothing is on yet: "Done" would only look like it.
                            if (effective == Effective.ON) Button(onClick = onDone, modifier = m) { Text(stringResource(R.string.onboarding_done)) }
                            else SecondaryActionButton(stringResource(R.string.onboarding_skip), onSkip, m)
                        },
                    )
                }
            }
        },
    ) { inner ->
        Column(
            Modifier.fillMaxSize().padding(inner).verticalScroll(rememberScrollState()).padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            if (step != null) StepHeader(step, R.string.access_title, Icons.Default.Notifications)
            Text(stringResource(R.string.access_intro), style = MaterialTheme.typography.bodyMedium)
            if (checking) CheckingCard() else StateCard(effective)
            if (!checking) {
                // Confirming needs the system's "allowed" and no mark yet; withdrawing only takes permission away,
                // so it shows whenever the mark is there, even if the state is unknown.
                if (effective == Effective.NEEDS_CONFIRMATION) Button(onClick = onConfirm) { Text(stringResource(R.string.access_confirm)) }
                if (confirmed) OutlinedButton(onClick = onWithdraw) { Text(stringResource(R.string.access_withdraw)) }
            }
            FilledTonalButton(onClick = onOpen) { Text(stringResource(R.string.access_open)) }
            // The way out of the step says it is later, and where to turn it on.
            if (step != null && effective != Effective.ON) {
                Text(
                    stringResource(R.string.access_later_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (!checking && (effective == Effective.OFF || effective == Effective.UNKNOWN)) {
                Text(
                    stringResource(R.string.access_restricted_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Text(
                stringResource(R.string.access_codes_hint),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** Neither allowed nor denied: the system has not answered yet. */
@Composable
private fun CheckingCard() {
    val scheme = MaterialTheme.colorScheme
    Surface(modifier = Modifier.semantics(mergeDescendants = true) {}, color = scheme.surfaceVariant, contentColor = scheme.onSurfaceVariant, shape = MaterialTheme.shapes.medium) {
        Row(Modifier.fillMaxWidth().padding(16.dp), horizontalArrangement = Arrangement.spacedBy(16.dp), verticalAlignment = Alignment.Top) {
            Icon(Icons.Default.Refresh, contentDescription = null)
            Text(stringResource(R.string.access_state_checking), style = MaterialTheme.typography.titleMedium)
        }
    }
}

/** The four states, each with its own icon and words: colour is never the only difference. */
@Composable
private fun StateCard(effective: Effective) {
    val scheme = MaterialTheme.colorScheme
    val (icon, container, content) = when (effective) {
        Effective.ON -> Triple(Icons.Default.Check, scheme.primaryContainer, scheme.onPrimaryContainer)
        Effective.NEEDS_CONFIRMATION -> Triple(Icons.Default.Info, scheme.secondaryContainer, scheme.onSecondaryContainer)
        Effective.OFF -> Triple(Icons.Default.Close, scheme.surfaceVariant, scheme.onSurfaceVariant)
        Effective.UNKNOWN -> Triple(Icons.Default.Warning, scheme.errorContainer, scheme.onErrorContainer)
    }
    // One announcement for the icon-less words: title and detail read together by a screen reader.
    Surface(modifier = Modifier.semantics(mergeDescendants = true) {}, color = container, contentColor = content, shape = MaterialTheme.shapes.medium) {
        Row(Modifier.fillMaxWidth().padding(16.dp), horizontalArrangement = Arrangement.spacedBy(16.dp), verticalAlignment = Alignment.Top) {
            Icon(icon, contentDescription = null)
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(stringResource(stateTitle(effective)), style = MaterialTheme.typography.titleMedium)
                Text(stringResource(stateDetail(effective)), style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}

@StringRes
internal fun stateTitle(effective: Effective): Int = when (effective) {
    Effective.ON -> R.string.access_state_granted
    Effective.NEEDS_CONFIRMATION -> R.string.access_state_confirm
    Effective.OFF -> R.string.access_state_denied
    Effective.UNKNOWN -> R.string.access_state_unknown
}

@StringRes
private fun stateDetail(effective: Effective): Int = when (effective) {
    Effective.ON -> R.string.access_detail_granted
    Effective.NEEDS_CONFIRMATION -> R.string.access_detail_confirm
    Effective.OFF -> R.string.access_detail_denied
    Effective.UNKNOWN -> R.string.access_detail_unknown
}

@AdbuddyPreviews
@Composable
private fun NotificationAccessOnboardingPreview() {
    AdbuddyTheme(dynamic = false) { NotificationAccessContent(OnboardingStep.NOTIFICATION_ACCESS, false, false, Effective.NEEDS_CONFIRMATION, {}, {}, {}, {}, {}, {}) }
}

@AdbuddyPreviews
@Composable
private fun NotificationAccessUnknownPreview() {
    AdbuddyTheme(dynamic = false) { NotificationAccessContent(null, false, true, Effective.UNKNOWN, {}, {}, {}, {}, {}, {}) }
}

@AdbuddyPreviews
@Composable
private fun NotificationAccessCheckingPreview() {
    AdbuddyTheme(dynamic = false) { NotificationAccessContent(null, true, false, Effective.UNKNOWN, {}, {}, {}, {}, {}, {}) }
}
