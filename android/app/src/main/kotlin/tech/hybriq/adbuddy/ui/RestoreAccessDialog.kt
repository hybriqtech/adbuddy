package tech.hybriq.adbuddy.ui

import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import tech.hybriq.adbuddy.R

/** Giving the PC its access back asks first and says what comes back (spec R5). Cutting never asks. */
@Composable
fun RestoreAccessDialog(onRestore: () -> Unit, onDismiss: () -> Unit) {
    GuardedAlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.restore_confirm_title)) },
        text = {
            Text(stringResource(R.string.restore_confirm_body))
        },
        confirmButton = { TextButton(onClick = onRestore) { Text(stringResource(R.string.restore_confirm_confirm)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } },
    )
}
