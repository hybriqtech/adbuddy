package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import tech.hybriq.adbuddy.R

/** Notifications are off or silenced: the agent's questions would only be seen with the app open. Informs and opens the system settings. */
@Composable
fun NotificationsBanner(onOpenSettings: () -> Unit, modifier: Modifier = Modifier) {
    val scheme = MaterialTheme.colorScheme
    Surface(color = scheme.errorContainer, contentColor = scheme.onErrorContainer, modifier = modifier.fillMaxWidth()) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 8.dp)) {
            // Only the text is the live region: the button stays an actionable node of its own.
            Text(
                stringResource(R.string.notifications_blocked), style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
            )
            TextButton(onClick = onOpenSettings) { Text(stringResource(R.string.notifications_open_settings)) }
        }
    }
}
