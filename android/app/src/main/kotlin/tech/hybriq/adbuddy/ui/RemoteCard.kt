package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import java.time.Instant
import java.time.format.FormatStyle
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.Bridge
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ErrorReply
import tech.hybriq.adbuddy.bridge.ErrorText
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.RemoteState
import tech.hybriq.adbuddy.confirm.HostClock
import tech.hybriq.adbuddy.ui.RemoteUi.Kind

/** The state a remote.set left unknown: the remote value it left (by identity) while the host has not spoken again. */
private class UnknownMark(val remote: RemoteState?)

/**
 * The remote-mode switch (M5). The logic is RemoteUi; this only draws it.
 * Turning it on asks first (port 5555 open on any network); turning it off asks too, with
 * the note on losing access away from home.
 */
@Composable
fun RemoteCard(state: BridgeState, bridge: Bridge, clock: HostClock, onMessage: (String) -> Unit, modifier: Modifier = Modifier) {
    var pending by remember { mutableStateOf(false) }
    var mark by remember { mutableStateOf<UnknownMark?>(null) }
    var ask by remember { mutableStateOf<Boolean?>(null) } // the value being confirmed
    val latest by rememberUpdatedState(state) // the reply's mark takes the state of then, not of the tap
    // HostClock is not observable: a tick at the expiry recomposes the card so ON turns CLOSING by itself.
    var tick by remember { mutableIntStateOf(0) }
    val unknownSince = RemoteUi.unknownSince(mark != null, mark?.remote, state.remote)
    val view = RemoteUi.view(state, pending, unknownSince, Instant.ofEpochMilli(clock.now() + tick * 0L)) // reads tick so the card recomposes
    LaunchedEffect(view.expiresAt, view.kind, tick) {
        RemoteUi.msUntilClosing(if (view.kind == Kind.ON) view.expiresAt else null, clock.now())?.let { delay(it); tick++ }
    }
    val failed = stringResource(R.string.remote_failed)
    val resources = LocalResources.current

    fun send(enabled: Boolean) {
        pending = true
        val sent = bridge.request(Protocol.REMOTE_SET, RemoteUi.request(enabled)) { reply ->
            pending = false
            when {
                reply == null -> mark = UnknownMark(latest.remote) // the tunnel dropped (adbd restarts): unknown, not off
                reply.type == Protocol.ERROR -> onMessage(Protocol.payload<ErrorReply>(reply)?.let { ErrorText.text({ id -> resources.getString(id) }, it) } ?: failed)
            }
        }
        if (sent == null) {
            pending = false
            onMessage(failed)
        }
    }

    val on = view.kind == Kind.ON || view.kind == Kind.PENDING || view.kind == Kind.CLOSING

    ElevatedCard(modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp)) {
        Row(
            Modifier
                .then(
                    if (view.showSwitch) {
                        // The whole row toggles (a 52 dp switch is a small target) and TalkBack reads it as one switch named by the card.
                        // It only asks: the confirmation dialog below is what sends anything.
                        Modifier.toggleable(value = on, enabled = view.canToggle, role = Role.Switch, onValueChange = { ask = it })
                    } else {
                        Modifier
                    },
                )
                .padding(12.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(stringResource(R.string.remote_title), style = MaterialTheme.typography.titleMedium)
                val problem = view.kind == Kind.CLOSING || view.kind == Kind.UNKNOWN || view.kind == Kind.DISCONNECTED
                Text(
                    detail(view, clock),
                    style = MaterialTheme.typography.bodySmall,
                    color = if (problem) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (view.kind == Kind.ON && view.address.isNotEmpty()) {
                    Text(view.address, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            if (view.showSwitch) {
                // With no onCheckedChange the Switch drops its own 48 dp minimum: restoring it keeps the card as tall as before (the images do not change).
                Switch(checked = on, onCheckedChange = null, enabled = view.canToggle, modifier = Modifier.minimumInteractiveComponentSize())
            }
        }
    }

    ask?.let { enable ->
        AlertDialog(
            onDismissRequest = { ask = null },
            title = { Text(stringResource(if (enable) R.string.remote_enable_title else R.string.remote_disable_title)) },
            text = {
                ObscuredTouchGuard()
                Text(stringResource(if (enable) R.string.remote_enable_body else R.string.remote_disable_body))
            },
            confirmButton = {
                TextButton(onClick = { ask = null; send(enable) }) {
                    Text(stringResource(if (enable) R.string.remote_enable_confirm else R.string.remote_disable_confirm))
                }
            },
            dismissButton = { TextButton(onClick = { ask = null }) { Text(stringResource(R.string.action_cancel)) } },
        )
    }
}

@Composable
private fun detail(v: RemoteUi.RemoteView, clock: HostClock): String = when (v.kind) {
    Kind.OFF -> stringResource(R.string.remote_off)
    Kind.ON -> v.expiresAt?.let { stringResource(R.string.remote_until, localTime(it, clock)) } ?: stringResource(R.string.remote_on)
    Kind.CLOSING -> stringResource(R.string.remote_closing)
    Kind.PENDING -> stringResource(R.string.remote_pending)
    Kind.UNKNOWN -> stringResource(R.string.remote_unknown)
    Kind.DISCONNECTED -> stringResource(R.string.remote_disconnected)
}

/** expiresAt is in the host's clock; HostClock moves it to the phone's before it is shown as local time. */
private fun localTime(hostInstant: Instant, clock: HostClock): String =
    Formats.time(clock.toLocal(hostInstant.toEpochMilli()), FormatStyle.SHORT)
