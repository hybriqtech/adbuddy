package tech.hybriq.adbuddy.ui

import androidx.annotation.StringRes
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.a11y.ProtectedApps
import tech.hybriq.adbuddy.listener.NotificationAccess
import tech.hybriq.adbuddy.listener.NotificationAccess.Effective
import tech.hybriq.adbuddy.listener.NotificationAccessStatus
import tech.hybriq.adbuddy.listener.NotificationConsent
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.ui.theme.AdbuddyPreviews
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme
import tech.hybriq.adbuddy.ui.theme.ThemePrefs

/**
 * Settings (spec v1 §6): what the user decides about the agent. M5c brings the protected apps and the
 * access to notifications, and the remote-mode switch moves here too (it stays in the chat as well).
 * Language, keeping debugging on and About belong to later milestones.
 */
@Composable
fun SettingsScreen(app: AdbuddyApp, state: BridgeState, onOpen: (Route) -> Unit, onBack: () -> Unit, onMessage: (String) -> Unit, onYolo: (Boolean) -> Unit = {}) {
    val context = LocalContext.current
    val consent = remember(context) { NotificationConsent.of(context) }
    // Read off the main thread; null until read. The system's state and the mark start unknown: nothing is
    // shown as allowed, denied or confirmed before they answer (and the mark is observed: the listener clears it).
    val protectedNow by produceState<Pair<Int, Boolean>?>(initialValue = null, context) {
        value = withContext(Dispatchers.IO) { ProtectedApps.of(context).let { it.list().size to it.confirmed } }
    }
    // Null until the system answers: that is "checking", not the system's own "unknown".
    val systemState by remember(context) { NotificationAccessStatus.observe(context) }.collectAsStateWithLifecycle(initialValue = null)
    val markState by remember(consent) { consent.observe() }.collectAsStateWithLifecycle(initialValue = null)
    val dynamic by app.dynamicColor.collectAsStateWithLifecycle()
    // Until the list is read (or the system and the mark answer) the rows say "checking": nothing is claimed.
    val system = systemState
    val mark = markState
    SettingsContent(
        protectedCount = protectedNow?.first,
        protectedConfirmed = protectedNow?.second == true,
        access = if (system == null || mark == null) null else NotificationAccess.effective(system, mark),
        onOpen = onOpen,
        onBack = onBack,
        remoteCard = { RemoteCard(state, app.bridge, app.clock, onMessage) },
        yoloCard = { YoloCard(state, app.clock, onTurnOn = { onYolo(true) }, onTurnOff = { onYolo(false) }) },
        languageAvailable = LanguageSettings.available(),
        onLanguage = { LanguageSettings.open(context) },
        dynamicColor = dynamic,
        onDynamicColor = { ThemePrefs(context).dynamic = it; app.dynamicColor.value = it },
    )
}

/** The screen without state, so it has previews. */
@Composable
fun SettingsContent(
    protectedCount: Int?,
    protectedConfirmed: Boolean,
    access: Effective?,
    onOpen: (Route) -> Unit,
    onBack: () -> Unit,
    remoteCard: @Composable () -> Unit,
    yoloCard: @Composable () -> Unit = {},
    languageAvailable: Boolean = false,
    onLanguage: () -> Unit = {},
    dynamicColor: Boolean = false,
    onDynamicColor: (Boolean) -> Unit = {},
) {
    Scaffold(
        topBar = {
            AdbuddyTopBar(stringResource(R.string.settings_title), onBack)
        },
    ) { inner ->
        Column(Modifier.padding(inner).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            // No ObscuredTouchGuard() here (spec section 8.5): it would mark the whole window of the activity, and nothing on this screen grants anything by a tap.
            SectionHeader(R.string.settings_section_security)
            SettingsRow(
                icon = { Icon(Icons.Default.Lock, contentDescription = null, tint = MaterialTheme.colorScheme.primary) },
                title = R.string.protected_title,
                summary = when {
                    protectedCount == null -> stringResource(R.string.access_state_checking)
                    protectedConfirmed -> stringResource(R.string.settings_protected_confirmed, protectedCount)
                    else -> stringResource(R.string.settings_protected_proposed, protectedCount)
                },
                // Not yet confirmed: the list is only a proposal, so it is flagged (never while still reading it).
                flagged = protectedCount != null && !protectedConfirmed,
                onClick = { onOpen(Route.PROTECTED_APPS) },
            )
            SettingsRow(
                icon = { Icon(Icons.Default.Notifications, contentDescription = null, tint = MaterialTheme.colorScheme.primary) },
                title = R.string.access_title,
                summary = stringResource(if (access == null) R.string.access_state_checking else stateTitle(access)),
                // Unknown, or allowed in the system but not yet confirmed here: the person has something to do.
                flagged = access == Effective.UNKNOWN || access == Effective.NEEDS_CONFIRMATION,
                onClick = { onOpen(Route.NOTIFICATION_ACCESS) },
            )
            remoteCard()
            yoloCard()
            SectionHeader(R.string.settings_section_app)
            if (languageAvailable) {
                SettingsRow(
                    icon = { Icon(Icons.Default.Settings, contentDescription = null, tint = MaterialTheme.colorScheme.primary) },
                    title = R.string.settings_language,
                    summary = stringResource(R.string.settings_language_hint),
                    flagged = false,
                    onClick = onLanguage,
                )
            }
            Row(
                Modifier.fillMaxWidth().toggleable(value = dynamicColor, role = Role.Switch, onValueChange = onDynamicColor).padding(horizontal = 16.dp, vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(Modifier.weight(1f)) {
                    Text(stringResource(R.string.settings_dynamic_color), style = MaterialTheme.typography.bodyLarge)
                    Text(stringResource(R.string.settings_dynamic_color_hint), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Switch(checked = dynamicColor, onCheckedChange = null)
            }
        }
    }
}

@Composable
private fun SectionHeader(@StringRes title: Int) {
    Text(
        stringResource(title), style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.padding(start = 16.dp, top = 16.dp, bottom = 4.dp).semantics { heading() },
    )
}

@Composable
private fun SettingsRow(icon: @Composable () -> Unit, @StringRes title: Int, summary: String, flagged: Boolean, onClick: () -> Unit) {
    ListItem(
        modifier = Modifier.clickable(role = Role.Button, onClick = onClick),
        leadingContent = icon,
        headlineContent = { Text(stringResource(title)) },
        supportingContent = {
            // Red is never the only sign: a warning icon goes with it.
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                if (flagged) Icon(Icons.Default.Warning, contentDescription = null, tint = MaterialTheme.colorScheme.error, modifier = Modifier.size(16.dp))
                Text(summary, color = if (flagged) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
            }
        },
    )
}

@AdbuddyPreviews
@Composable
private fun SettingsPreview() {
    AdbuddyTheme(dynamic = false) {
        SettingsContent(protectedCount = 12, protectedConfirmed = false, access = Effective.NEEDS_CONFIRMATION, onOpen = {}, onBack = {}, remoteCard = {})
    }
}
