package tech.hybriq.adbuddy.ui.theme

import android.content.Context
import androidx.core.content.edit

/** Settings, "Use wallpaper colors" (spec R6): off unless the person turns it on. */
class ThemePrefs(context: Context) {
    private val prefs = context.getSharedPreferences("theme", Context.MODE_PRIVATE)
    var dynamic: Boolean
        get() = prefs.getBoolean("dynamic", false)
        set(value) = prefs.edit { putBoolean("dynamic", value) }
}
