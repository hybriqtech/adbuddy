package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.PermissionResolved
import tech.hybriq.adbuddy.confirm.HostClock
import tech.hybriq.adbuddy.yolo.YoloUi
import tech.hybriq.adbuddy.yolo.YoloUi.Kind

const val YOLO_BAND_TAG = "yolo_band"
const val YOLO_APPROVALS_TAG = "yolo_approvals"

/**
 * The fixed band over every screen while YOLO is on (spec 2026-10-18): the countdown, or "until the session ends", and
 * Turn off, which asks nothing. It has the alert colour, and it also draws under the status bar (the screens below it
 * are told the bar is taken: ChatRoot). TalkBack reads a fixed sentence, not the seconds.
 */
@Composable
fun YoloBand(view: YoloUi.View, onOff: () -> Unit, modifier: Modifier = Modifier) {
    if (!view.active) return
    val scheme = MaterialTheme.colorScheme
    val spoken = stringResource(R.string.yolo_band_spoken)
    Surface(color = scheme.error, contentColor = scheme.onError, modifier = modifier.fillMaxWidth().testTag(YOLO_BAND_TAG)) {
        Row(
            Modifier.windowInsetsPadding(WindowInsets.statusBars).padding(start = 16.dp, end = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(Icons.Default.Warning, contentDescription = null, modifier = Modifier.size(18.dp))
            Text(
                if (view.kind == Kind.SESSION) stringResource(R.string.yolo_band_session) else stringResource(R.string.yolo_band_timed, YoloUi.countdown(view.msLeft)),
                modifier = Modifier.weight(1f).padding(start = 8.dp).semantics { contentDescription = spoken },
                style = MaterialTheme.typography.labelLarge,
            )
            TextButton(onClick = onOff, colors = ButtonDefaults.textButtonColors(contentColor = scheme.onError)) { Text(stringResource(R.string.yolo_band_off)) }
        }
    }
}

/** The band of a screen tree: the one that reads the clock (rememberYoloView ticks), so only the band recomposes each second. */
@Composable
fun YoloBand(state: BridgeState, clock: HostClock, onOff: () -> Unit, modifier: Modifier = Modifier) = YoloBand(rememberYoloView(state, clock), onOff, modifier)

/** What YOLO approved on its own (the agent's permission questions), in the chat: the last three. */
@Composable
fun YoloApprovals(approvals: List<PermissionResolved>, modifier: Modifier = Modifier) {
    Column(modifier.fillMaxWidth().testTag(YOLO_APPROVALS_TAG), verticalArrangement = Arrangement.spacedBy(2.dp)) {
        for (a in approvals.takeLast(3)) {
            Text(
                if (a.summary.isEmpty()) stringResource(R.string.yolo_approved_unnamed) else stringResource(R.string.yolo_approved, a.summary),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.error,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}
