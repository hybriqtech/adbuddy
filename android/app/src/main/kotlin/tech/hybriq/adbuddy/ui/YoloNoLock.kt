package tech.hybriq.adbuddy.ui

import android.os.SystemClock
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.LifecycleResumeEffect
import kotlinx.coroutines.delay
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.confirm.ConfirmUi

/**
 * The last step of the activation on a phone with no secure lock (ruling R20, the person's decision): it says that
 * nothing proves it is them, and that an agent already working on the phone could finish the activation by tapping, even
 * after YOLO expired; the button waits ConfirmUi.ARMING_MS from when it is shown (again each time the app comes back to
 * the front), as Allow does. [onConfirm] is called only by an armed tap; [onDismiss] by Cancel or by leaving the dialog.
 * [elapsed] is the phone's elapsed time (a parameter for the tests).
 */
@Composable
fun YoloNoLockDialog(onConfirm: () -> Unit, onDismiss: () -> Unit, elapsed: () -> Long = SystemClock::elapsedRealtime) {
    var shownAt by remember { mutableLongStateOf(elapsed()) }
    var tick by remember { mutableIntStateOf(0) }
    LifecycleResumeEffect(Unit) {
        shownAt = elapsed()
        onPauseOrDispose {}
    }
    LaunchedEffect(shownAt) {
        delay(ConfirmUi.ARMING_MS + 50)
        tick++
    }
    val armed = ConfirmUi.armed(shownAt, elapsed() + tick * 0L)
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.yolo_nolock_title)) },
        text = {
            ObscuredTouchGuard()
            Column(Modifier.verticalScroll(rememberScrollState())) { Text(stringResource(R.string.yolo_nolock_body)) }
        },
        confirmButton = {
            // Decided again with the clock of this moment, as the activation dialog does.
            TextButton(enabled = armed, onClick = { if (ConfirmUi.armed(shownAt, elapsed())) onConfirm() }) {
                Text(stringResource(R.string.yolo_nolock_confirm))
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } },
    )
}
