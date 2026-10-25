package tech.hybriq.adbuddy.ui

import androidx.compose.material3.AlertDialog
import androidx.compose.runtime.Composable
import androidx.compose.ui.window.DialogProperties

/**
 * The app's only AlertDialog. A dialog is its own window, so the screen's tapjacking flag does not reach it: the guard
 * goes first in its [text]. TouchGuardSourceTest fails when an AlertDialog is used anywhere else.
 */
@Composable
fun GuardedAlertDialog(
    onDismissRequest: () -> Unit,
    title: @Composable () -> Unit,
    text: @Composable () -> Unit,
    confirmButton: @Composable () -> Unit,
    dismissButton: (@Composable () -> Unit)? = null,
    properties: DialogProperties = DialogProperties(),
) {
    AlertDialog(
        onDismissRequest = onDismissRequest,
        title = title,
        text = {
            ObscuredTouchGuard()
            text()
        },
        confirmButton = confirmButton,
        dismissButton = dismissButton,
        properties = properties,
    )
}
