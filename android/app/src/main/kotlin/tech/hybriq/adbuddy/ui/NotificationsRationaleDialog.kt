package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.window.DialogProperties
import tech.hybriq.adbuddy.R

/** Title and body of the reason, apart from the dialog's window so they can be drawn and measured on their own. */
@Composable
fun NotificationsRationaleContent(modifier: Modifier = Modifier) {
    Column(modifier) {
        Text(stringResource(R.string.notifications_rationale_title), style = MaterialTheme.typography.headlineSmall)
        Text(stringResource(R.string.notifications_rationale_body), style = MaterialTheme.typography.bodyMedium)
    }
}

/** Why the app wants to notify, before the system asks. Back is "Not now"; a tap outside does nothing. */
@Composable
fun NotificationsRationaleDialog(onContinue: () -> Unit, onSkip: () -> Unit) {
    AlertDialog(
        onDismissRequest = onSkip,
        title = { Text(stringResource(R.string.notifications_rationale_title)) },
        text = { Text(stringResource(R.string.notifications_rationale_body)) },
        confirmButton = { TextButton(onClick = onContinue) { Text(stringResource(R.string.notifications_rationale_continue)) } },
        dismissButton = { TextButton(onClick = onSkip) { Text(stringResource(R.string.onboarding_skip)) } },
        properties = DialogProperties(dismissOnClickOutside = false),
    )
}
