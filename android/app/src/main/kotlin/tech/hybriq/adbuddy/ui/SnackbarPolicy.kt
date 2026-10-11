package tech.hybriq.adbuddy.ui

import androidx.compose.material3.SnackbarDuration
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.SnackbarResult

/** How long a notice stays: long ones need time to be read, and one with an action waits for it. */
object SnackbarPolicy {
    const val LONG_FROM = 60

    fun duration(text: String, hasAction: Boolean): SnackbarDuration = when {
        hasAction -> SnackbarDuration.Indefinite
        text.length > LONG_FROM -> SnackbarDuration.Long
        else -> SnackbarDuration.Short
    }
}

/** Shows a notice with the duration the policy gives it; an endless one gets its own dismiss button. */
suspend fun SnackbarHostState.notify(text: String, actionLabel: String? = null): SnackbarResult {
    val duration = SnackbarPolicy.duration(text, actionLabel != null)
    return showSnackbar(
        message = text,
        actionLabel = actionLabel,
        withDismissAction = duration == SnackbarDuration.Indefinite,
        duration = duration,
    )
}
