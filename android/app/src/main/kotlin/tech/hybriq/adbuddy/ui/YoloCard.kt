package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.confirm.HostClock
import tech.hybriq.adbuddy.yolo.YoloUi
import tech.hybriq.adbuddy.yolo.YoloUi.Kind

const val YOLO_CARD_TAG = "yolo_card"

/** The YOLO view now: it recomposes every second while there is a countdown (HostClock is not observable). */
@Composable
fun rememberYoloView(state: BridgeState, clock: HostClock): YoloUi.View {
    var tick by remember { mutableIntStateOf(0) }
    val counting = state.yolo?.let { it.active && !it.session } == true
    LaunchedEffect(counting) {
        while (counting) {
            delay(1000)
            tick++
        }
    }
    return YoloUi.view(state, clock.now() + tick * 0L) // reads tick so the view recomposes
}

/**
 * The YOLO switch of Settings. The logic is YoloUi; this only draws it. Turning it on only asks (the dialog is the
 * caller's and is what sends anything); turning it off is one tap, with no question: off is always safe.
 */
@Composable
fun YoloCard(state: BridgeState, clock: HostClock, onTurnOn: () -> Unit, onTurnOff: () -> Unit, modifier: Modifier = Modifier) {
    val view = rememberYoloView(state, clock)
    val connected = state.isConnected
    // Off stays available whatever the link says (the request may fail: the caller says so); on needs a known off and the PC.
    val enabled = view.active || YoloUi.canTurnOn(view, connected)
    ElevatedCard(modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp).testTag(YOLO_CARD_TAG)) {
        Row(
            Modifier
                .toggleable(value = view.active, enabled = enabled, role = Role.Switch, onValueChange = { if (it) onTurnOn() else onTurnOff() })
                .padding(12.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(stringResource(R.string.yolo_title), style = MaterialTheme.typography.titleMedium)
                val problem = view.active || view.kind == Kind.UNKNOWN || (!connected && !view.active)
                Text(
                    when {
                        view.kind == Kind.TIMED -> stringResource(R.string.yolo_on_left, YoloUi.countdown(view.msLeft))
                        view.kind == Kind.SESSION -> stringResource(R.string.yolo_on_session)
                        view.kind == Kind.UNKNOWN -> stringResource(R.string.yolo_unknown)
                        !connected -> stringResource(R.string.yolo_disconnected)
                        else -> stringResource(R.string.yolo_off)
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = if (problem) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Switch(checked = view.active, onCheckedChange = null, enabled = enabled, modifier = Modifier.minimumInteractiveComponentSize())
        }
    }
}
