package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * The link in one pill under the agent's name (spec §5): colour and dot by state, the PC's name when connected, and
 * " · Remote" while remote mode is on or unknown (RemoteUi.treatAsRemote: never a false "off").
 */
@Composable
fun StatusPill(status: LinkStatus, host: String, remote: Boolean, modifier: Modifier = Modifier) {
    val scheme = MaterialTheme.colorScheme
    val extra = AdbuddyTheme.colors
    val (container, content, dot) = when (status) {
        LinkStatus.CONNECTED -> Triple(extra.successContainer, extra.onSuccessContainer, extra.success)
        LinkStatus.PAUSED -> Triple(extra.warningContainer, extra.onWarningContainer, extra.warning)
        LinkStatus.CUT, LinkStatus.UPDATE_APP, LinkStatus.UPDATE_HOST -> Triple(scheme.errorContainer, scheme.onErrorContainer, scheme.error)
        LinkStatus.CONNECTING, LinkStatus.WAITING, LinkStatus.NOT_SET_UP -> Triple(scheme.surfaceContainerHighest, scheme.onSurfaceVariant, scheme.outline)
    }
    val base = if (status == LinkStatus.CONNECTED && host.isNotEmpty()) stringResource(R.string.status_connected, host) else stringResource(status.shortText)
    val text = if (remote && (status == LinkStatus.CONNECTED || status == LinkStatus.PAUSED)) stringResource(R.string.status_pill_remote, base) else base
    Surface(color = container, contentColor = content, shape = CircleShape, modifier = modifier) {
        Row(Modifier.padding(horizontal = 10.dp, vertical = 4.dp), horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.size(8.dp).background(dot, CircleShape))
            Text(text, style = MaterialTheme.typography.labelLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
    }
}
