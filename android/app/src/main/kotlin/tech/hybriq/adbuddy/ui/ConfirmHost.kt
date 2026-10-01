package tech.hybriq.adbuddy.ui

import android.app.KeyguardManager
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import kotlinx.coroutines.delay
import tech.hybriq.adbuddy.bridge.Bridge
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.confirm.ConfirmUi
import tech.hybriq.adbuddy.confirm.HostClock

/** The red questions, over whichever screen is showing: they never wait for the person to open the right one. */
@Composable
fun ConfirmHost(
    state: BridgeState,
    bridge: Bridge,
    clock: HostClock,
    focusConfirm: String?,
    closed: Set<String>,
    onClose: (String) -> Unit,
) {
    val context = LocalContext.current
    // The clock the questions are judged by (the host's): it moves on at each
    // expiry, so a question left open on screen loses its buttons when it expires.
    var now by remember { mutableLongStateOf(clock.now()) }
    LaunchedEffect(state.pending) {
        while (true) {
            now = clock.now()
            delay(ConfirmUi.nextExpiry(state.pending, now) ?: break)
        }
    }
    val question = ConfirmUi.question(state, focusConfirm, now, closed)
    if (question != null) {
        ConfirmDialog(question, ConfirmUi.status(question, state, now), clock, onClose = { onClose(question.id) }) { decision ->
            // The same rule as the notification buttons (ConfirmReceiver), with the
            // current state and time, not those of this composition: a second quick
            // tap sees the first answer (answering), and the delay stops while the
            // phone sleeps. Allowing needs the phone unlocked here too.
            val t = clock.now()
            val locked = context.getSystemService(KeyguardManager::class.java).isDeviceLocked
            if (ConfirmUi.canSend(bridge.state.value, question.id, decision, t, locked)) {
                bridge.reply(question.id, decision)
            } else {
                now = t
            }
        }
    }
}
