package tech.hybriq.adbuddy.control

import android.content.ContentResolver
import android.content.Context
import android.os.Build
import android.provider.Settings
import android.util.Log

/**
 * What the app reads of the two debugging settings, and how (spec 2026-10-18 §3, diagnóstico abierto). On the user's
 * Pixel (Android 17) `adb shell` says adb_enabled=1 and the app showed "cut": `adb logcat -s ADBuddyAccess` says what
 * the app really gets. Only numbers and exception class names are logged: no PC name, no command, nothing from the host.
 */
object AccessDiag {
    const val TAG = "ADBuddyAccess"

    /** raw: Settings.Global.getString as is (null = never written); error: the exception's class name when it threw. */
    data class Reading(val name: String, val raw: String?, val error: String?)

    fun line(event: String, sdk: Int, readings: List<Reading>, saved: Map<String, Int?>): String = buildString {
        append(event).append(" sdk=").append(sdk)
        for (r in readings) {
            append(' ').append(r.name).append('=')
            append(
                when {
                    r.error != null -> "error:" + r.error
                    r.raw == null -> "null"
                    else -> "raw:" + r.raw.replace(Regex("[\r\n]+"), " ").take(16)
                },
            )
        }
        for ((name, value) in saved) append(" saved.").append(name).append('=').append(value?.toString() ?: "-")
    }

    fun read(resolver: ContentResolver, name: String): Reading = try {
        Reading(name, Settings.Global.getString(resolver, name), null)
    } catch (e: SecurityException) {
        Reading(name, null, e.javaClass.simpleName)
    }

    /** Logs one line; never throws (a diagnosis must not break the screen or the service). */
    fun log(context: Context, event: String) {
        try {
            val resolver = context.contentResolver
            val prefs = context.getSharedPreferences(CutAccess.PREFS, Context.MODE_PRIVATE)
            val names = listOf(AccessSwitch.ADB_ENABLED, AccessSwitch.ADB_WIFI_ENABLED)
            val saved = names.associateWith { if (prefs.contains(it)) prefs.getInt(it, 0) else null }
            Log.i(TAG, line(event, Build.VERSION.SDK_INT, names.map { read(resolver, it) }, saved))
        } catch (e: RuntimeException) {
            Log.w(TAG, "diagnosis failed: " + e.javaClass.simpleName)
        }
    }
}
