package tech.hybriq.adbuddy.util

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent

/**
 * Starts the first of [intents] the system can open (each as a new task); false when none can be.
 * Only ActivityNotFoundException moves on to the next one.
 */
internal fun Context.startFirst(vararg intents: Intent): Boolean {
    for (intent in intents) {
        try {
            startActivity(intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
            return true
        } catch (_: ActivityNotFoundException) {
            // Try the next, more general, screen.
        }
    }
    return false
}
