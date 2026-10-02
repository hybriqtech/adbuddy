package tech.hybriq.adbuddy.ui

import android.content.Context
import android.content.SharedPreferences
import androidx.core.content.edit

/** The screens of the app after the first one (M5c): where Back goes from each. */
enum class Route {
    CHAT,
    ACTIVITY,
    SETTINGS,
    PROTECTED_APPS,
    NOTIFICATION_ACCESS,
    ;

    /** The screen Back returns to, or null when Back leaves the app. */
    val parent: Route?
        get() = when (this) {
            CHAT -> null
            ACTIVITY, SETTINGS -> CHAT
            PROTECTED_APPS, NOTIFICATION_ACCESS -> SETTINGS
        }
}

/**
 * The first-run steps after the consent (spec v1 §6, onboarding): the protected apps, then the access
 * to notifications (optional). Each step can be skipped; finishing or skipping the last one ends it.
 */
enum class OnboardingStep {
    PROTECTED_APPS,
    NOTIFICATION_ACCESS,
    ;

    /** 1-based, for "Step 1 of 2". */
    val number: Int get() = ordinal + 1

    /** The step after this one, or null when this was the last. */
    fun next(): OnboardingStep? = entries.getOrNull(ordinal + 1)

    /** The step before, or null for the first. */
    fun previous(): OnboardingStep? = entries.getOrNull(ordinal - 1)

    companion object {
        val total: Int get() = entries.size

        /** The step to open with: the first until the steps are done, and none at all for a screen that is not the first run (the bubble). */
        fun initial(firstRun: Boolean, done: Boolean): OnboardingStep? = if (firstRun && !done) PROTECTED_APPS else null
    }
}

/** Whether the first-run steps were done: they show once, until finished or skipped to the end. */
class Onboarding(private val prefs: SharedPreferences) {
    fun done(): Boolean = prefs.getBoolean(KEY_DONE, false)

    fun finish() {
        prefs.edit { putBoolean(KEY_DONE, true) }
    }

    companion object {
        private const val PREFS = "onboarding"
        private const val KEY_DONE = "done"

        fun of(context: Context) = Onboarding(context.getSharedPreferences(PREFS, Context.MODE_PRIVATE))
    }
}
