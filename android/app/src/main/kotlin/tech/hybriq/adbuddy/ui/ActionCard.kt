package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.painter.Painter
import androidx.compose.ui.graphics.vector.rememberVectorPainter
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/**
 * The card of HomeNotice: icon, title, one line and at most one button. QUESTIONS' button only reopens the closed
 * red questions (their dialog arms and makes you read again): nothing here answers anything (spec §8.4).
 */
@Composable
fun ActionCard(notice: HomeNotice, pendingQuestions: Int, onAction: () -> Unit, modifier: Modifier = Modifier) {
    val scheme = MaterialTheme.colorScheme
    val extra = AdbuddyTheme.colors
    val neutral = notice == HomeNotice.NOT_SET_UP || notice == HomeNotice.WAITING
    val (container, content) = when (notice) {
        HomeNotice.QUESTIONS, HomeNotice.CUT, HomeNotice.UPDATE_APP, HomeNotice.UPDATE_HOST -> scheme.errorContainer to scheme.onErrorContainer
        HomeNotice.NOTIFICATIONS, HomeNotice.PAUSED -> extra.warningContainer to extra.onWarningContainer
        HomeNotice.NOT_SET_UP, HomeNotice.WAITING -> scheme.surfaceContainerHigh to scheme.onSurface
    }
    // Own shield vectors (task 6) for the security notices; only Info/Notifications come from material-icons-core.
    val icon: Painter = when (notice) {
        HomeNotice.CUT -> painterResource(R.drawable.ic_shield_off)
        HomeNotice.NOTIFICATIONS -> rememberVectorPainter(Icons.Default.Notifications)
        HomeNotice.NOT_SET_UP, HomeNotice.WAITING -> rememberVectorPainter(Icons.Default.Info)
        else -> painterResource(R.drawable.ic_shield)
    }
    val title = when (notice) {
        HomeNotice.QUESTIONS -> pluralStringResource(R.plurals.confirm_waiting, pendingQuestions, pendingQuestions)
        HomeNotice.CUT -> stringResource(R.string.card_cut_title)
        HomeNotice.UPDATE_APP -> stringResource(R.string.card_update_app_title)
        HomeNotice.UPDATE_HOST -> stringResource(R.string.card_update_host_title)
        HomeNotice.NOT_SET_UP -> stringResource(R.string.card_not_set_up_title)
        HomeNotice.NOTIFICATIONS -> stringResource(R.string.card_notifications_title)
        HomeNotice.PAUSED -> stringResource(R.string.card_paused_title)
        HomeNotice.WAITING -> stringResource(R.string.card_waiting_title)
    }
    val body: String? = when (notice) {
        HomeNotice.QUESTIONS -> null
        HomeNotice.CUT -> stringResource(R.string.card_cut_body)
        HomeNotice.UPDATE_APP -> stringResource(R.string.status_update_app)
        HomeNotice.UPDATE_HOST -> stringResource(R.string.status_update_host)
        HomeNotice.NOT_SET_UP -> stringResource(R.string.status_not_set_up)
        HomeNotice.NOTIFICATIONS -> stringResource(R.string.notifications_blocked)
        HomeNotice.PAUSED -> stringResource(R.string.status_paused)
        HomeNotice.WAITING -> stringResource(R.string.status_waiting)
    }
    val action: Int? = when (notice) {
        HomeNotice.QUESTIONS -> R.string.confirm_waiting_show
        HomeNotice.CUT -> R.string.action_restore
        HomeNotice.NOTIFICATIONS -> R.string.notifications_open_settings
        HomeNotice.PAUSED -> R.string.action_resume
        HomeNotice.WAITING -> R.string.card_waiting_action
        else -> null
    }
    Surface(color = container, contentColor = content, shape = MaterialTheme.shapes.medium, modifier = modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp)) {
        Row(Modifier.padding(16.dp), horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.Top) {
            Icon(icon, contentDescription = null, tint = if (neutral) scheme.primary else LocalContentColor.current)
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                // Only the texts are the live region: the button stays an actionable node of its own.
                Column(Modifier.semantics(mergeDescendants = true) { liveRegion = LiveRegionMode.Polite }) {
                    Text(title, style = MaterialTheme.typography.titleSmall, modifier = Modifier.semantics { heading() })
                    if (body != null) Text(body, style = MaterialTheme.typography.bodyMedium)
                }
                if (action != null) {
                    // Only the button refuses covered touches (spec §8.5): no window-wide guard on a screen.
                    FilledTonalButton(onClick = onAction, modifier = Modifier.padding(top = 4.dp).refuseObscuredTouches()) { Text(stringResource(action)) }
                }
            }
        }
    }
}
