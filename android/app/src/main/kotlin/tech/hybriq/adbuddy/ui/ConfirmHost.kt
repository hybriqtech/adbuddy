package tech.hybriq.adbuddy.ui

import android.app.KeyguardManager
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
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
    ConfirmHost(state, { bridge.state.value }, { id, decision -> bridge.reply(id, decision) }, clock, focusConfirm, closed, onClose)
}

/**
 * [ConfirmHost] with the bridge as two functions, so a test can see what reaches it: [current] is the bridge's state
 * now (not this composition's), [reply] sends an answer.
 */
@Composable
internal fun ConfirmHost(
    state: BridgeState,
    current: () -> BridgeState,
    reply: (String, String) -> Unit,
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
    // The arming (ConfirmUi.ARMING_MS): Allow waits from when a question takes the dialog or becomes answerable, and
    // again each time the app comes to the front (ON_START, ON_RESUME: the screen woke, the person came back), so a
    // tap meant for what was there, or a burst as the dialog pops up, is not an answer. Deny never waits.
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val fronts = remember { mutableIntStateOf(0) }
    DisposableEffect(lifecycle) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_START || event == Lifecycle.Event.ON_RESUME) fronts.intValue++
        }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer) }
    }
    var armedFor by remember { mutableStateOf<Arming?>(null) }
    val question = ConfirmUi.question(state, focusConfirm, now, closed)
    val status = question?.let { ConfirmUi.status(it, state, now) }
    val front = fronts.intValue
    val open = status == ConfirmUi.Status.OPEN
    // Each start over of the arming is a new generation, counted as the dialog is composed (not when its coroutine
    // runs): from the first frame a question is answerable again (open, not open, open), an earlier arming is stale.
    val generations = remember { Generations() }
    val generation = remember(question?.id, front, open) { ++generations.last }
    LaunchedEffect(question?.id, front, open) {
        armedFor = null
        val id = question?.id ?: return@LaunchedEffect
        if (!open) return@LaunchedEffect
        delay(ConfirmUi.ARMING_MS)
        armedFor = Arming(id, front, generation)
    }
    // The seconds the dialog counts down: its own state, apart from `now` (status, canSend and the arming never read
    // it). It moves each second while a question is open.
    var displayNow by remember(question?.id) { mutableLongStateOf(clock.now()) }
    LaunchedEffect(question?.id, open) {
        while (open) {
            displayNow = clock.now()
            delay(1000)
        }
    }
    if (question != null && status != null) {
        val armedNow = armedFor == Arming(question.id, front, generation)
        ConfirmDialog(question, status, clock, armed = armedNow, now = displayNow, onClose = { onClose(question.id) }) { decision ->
            // The same rule as the notification buttons (ConfirmReceiver), with the
            // current state, time and arming, not those of this composition: a second
            // quick tap sees the first answer (answering), the delay stops while the
            // phone sleeps, and a tap as the app comes back to the front, before the
            // dialog is drawn again, finds it not armed. Allowing needs the phone unlocked here too.
            val t = clock.now()
            val locked = context.getSystemService(KeyguardManager::class.java).isDeviceLocked
            val armed = armedFor == Arming(question.id, fronts.intValue, generations.last)
            if (ConfirmUi.canSend(current(), question.id, decision, t, locked, armed, notification = false)) {
                reply(question.id, decision)
            } else {
                now = t
            }
        }
    }
}

/** What the dialog's Allow was armed for: this question, since the app's latest coming to the front, in this generation. */
private data class Arming(val id: String, val front: Int, val generation: Int)

/** The latest generation of the arming; not state: only the tap and the next start over read it. */
private class Generations {
    var last = 0
}
