package tech.hybriq.adbuddy.ui

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import tech.hybriq.adbuddy.notify.startFirst

/**
 * Opens the system's page for this app's language (Android 13 and later: Settings, Apps, ADBuddy, Language; the
 * languages are the ones in res/xml/locales_config.xml). Below Android 13 the app follows the language of the system
 * and there is nothing to open. M5c puts it behind the language row of its Settings screen.
 */
object LanguageSettings {
    fun available(sdkInt: Int = Build.VERSION.SDK_INT): Boolean = sdkInt >= Build.VERSION_CODES.TIRAMISU

    /** True when the page was opened. */
    fun open(context: Context): Boolean {
        if (!available()) return false
        return context.startFirst(Intent(Settings.ACTION_APP_LOCALE_SETTINGS, Uri.fromParts("package", context.packageName, null)))
    }
}
