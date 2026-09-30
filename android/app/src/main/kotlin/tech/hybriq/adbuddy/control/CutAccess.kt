package tech.hybriq.adbuddy.control

import android.content.ContentResolver
import android.content.Context
import android.database.ContentObserver
import android.os.Handler
import android.os.Looper
import android.provider.Settings
import androidx.core.content.edit
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.distinctUntilChanged

/**
 * "Cut access" (spec §4.6): turns off wireless debugging and then USB/adb
 * debugging with WRITE_SECURE_SETTINGS, without needing the PC, and restores
 * what there was before. The logic is AccessSwitch; this binds it to
 * Settings.Global and to the app's private preferences.
 */
object CutAccess {
    private fun switch(context: Context): AccessSwitch {
        val resolver = context.contentResolver
        val prefs = context.getSharedPreferences("cut_access", Context.MODE_PRIVATE)
        return AccessSwitch(
            object : AccessSwitch.Settings {
                // Android 12+ may refuse to read a hidden setting to an app: then it counts as never written.
                override fun get(name: String): Int? = try {
                    Settings.Global.getString(resolver, name)?.toIntOrNull()
                } catch (e: SecurityException) {
                    null
                }
                override fun put(name: String, value: Int) {
                    Settings.Global.putInt(resolver, name, value)
                }
            },
            object : AccessSwitch.Saved {
                override fun get(name: String): Int? = if (prefs.contains(name)) prefs.getInt(name, 0) else null
                override fun put(name: String, value: Int?) = prefs.edit(commit = true) {
                    if (value == null) remove(name) else putInt(name, value)
                }
            },
        )
    }

    /** Returns false when the permission is missing or the change did not stick. */
    fun cut(context: Context): Boolean = switch(context).cut()

    /** Undoes cut(), with the values from before it. */
    fun restore(context: Context): Boolean = switch(context).restore()

    fun isCut(resolver: ContentResolver): Boolean =
        Settings.Global.getInt(resolver, Settings.Global.ADB_ENABLED, 1) == 0

    /**
     * isCut, now and on every change of either setting (by ADBuddy, the
     * Settings app or adb). Each time it also forgets the values from before a
     * cut that the person already undid by hand (AccessSwitch.forgetIfRestored).
     * The observer runs on the main thread, like cut() and restore() (the
     * screen's buttons), so it never sees a cut half done.
     */
    fun observe(context: Context): Flow<Boolean> = callbackFlow {
        val resolver = context.contentResolver
        val switch = switch(context)
        fun current(): Boolean {
            switch.forgetIfRestored()
            return isCut(resolver)
        }
        val observer = object : ContentObserver(Handler(Looper.getMainLooper())) {
            override fun onChange(selfChange: Boolean) {
                trySend(current())
            }
        }
        for (name in listOf(AccessSwitch.ADB_ENABLED, AccessSwitch.ADB_WIFI_ENABLED)) {
            resolver.registerContentObserver(Settings.Global.getUriFor(name), false, observer)
        }
        trySend(current())
        awaitClose { resolver.unregisterContentObserver(observer) }
    }.distinctUntilChanged()
}
