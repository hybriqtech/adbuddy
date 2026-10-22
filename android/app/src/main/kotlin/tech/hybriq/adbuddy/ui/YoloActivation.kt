package tech.hybriq.adbuddy.ui

import android.os.SystemClock
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.LifecycleResumeEffect
import kotlinx.coroutines.delay
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.confirm.ConfirmUi
import tech.hybriq.adbuddy.yolo.YoloUi

private val YoloUi.Choice.label: Int
    get() = when (this) {
        YoloUi.Choice.QUARTER -> R.string.yolo_for_15
        YoloUi.Choice.HOUR -> R.string.yolo_for_60
        YoloUi.Choice.SESSION -> R.string.yolo_for_session
    }

/**
 * The warning that turns YOLO on (spec 2026-10-18, App Android). It lists what is skipped, makes the person choose a
 * duration (none is chosen for them), and its button sends nothing until the choice is made, the PC is connected, the
 * phone is unlocked (read again at the tap: [isLocked]) and the button has been shown for ConfirmUi.ARMING_MS
 * (counted again each time the app comes back to the front): the same discipline as Allow in a red question.
 * [elapsed] is the phone's elapsed time (a parameter for the tests).
 */
@Composable
fun YoloActivationDialog(
    connected: Boolean,
    isLocked: () -> Boolean,
    onConfirm: (YoloUi.Choice) -> Unit,
    onDismiss: () -> Unit,
    elapsed: () -> Long = SystemClock::elapsedRealtime,
) {
    var picked by rememberSaveable { mutableStateOf<String?>(null) }
    val choice = picked?.let { name -> YoloUi.Choice.entries.firstOrNull { it.name == name } }
    var shownAt by remember { mutableLongStateOf(elapsed()) }
    var tick by remember { mutableIntStateOf(0) }
    LifecycleResumeEffect(Unit) {
        shownAt = elapsed()
        onPauseOrDispose {}
    }
    // A tick when the wait is over, so the button is drawn enabled.
    LaunchedEffect(shownAt) {
        delay(ConfirmUi.ARMING_MS + 50)
        tick++
    }
    val armed = YoloUi.mayConfirm(choice, shownAt, elapsed() + tick * 0L, locked = false, connected = connected)
    GuardedAlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.yolo_dialog_title)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(stringResource(R.string.yolo_dialog_intro))
                for (id in listOf(R.string.yolo_skip_confirmations, R.string.yolo_skip_blocks, R.string.yolo_skip_journal, R.string.yolo_skip_protected, R.string.yolo_skip_permissions)) {
                    Text(stringResource(id), style = MaterialTheme.typography.bodyMedium)
                }
                Text(stringResource(R.string.yolo_pause_note), style = MaterialTheme.typography.bodyMedium)
                Text(stringResource(R.string.yolo_auth_note), style = MaterialTheme.typography.bodyMedium)
                Text(stringResource(R.string.yolo_choose), style = MaterialTheme.typography.titleSmall)
                for (c in YoloUi.Choice.entries) {
                    Row(
                        Modifier.fillMaxWidth().selectable(selected = c == choice, role = Role.RadioButton, onClick = { picked = c.name }).minimumInteractiveComponentSize(),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        RadioButton(selected = c == choice, onClick = null)
                        Text(stringResource(c.label), Modifier.padding(start = 12.dp))
                    }
                }
            }
        },
        confirmButton = {
            TextButton(
                enabled = armed,
                onClick = {
                    // Everything is decided again with the clock and the lock of this moment.
                    if (choice != null && YoloUi.mayConfirm(choice, shownAt, elapsed(), isLocked(), connected)) onConfirm(choice)
                },
            ) { Text(stringResource(R.string.yolo_confirm)) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } },
    )
}
