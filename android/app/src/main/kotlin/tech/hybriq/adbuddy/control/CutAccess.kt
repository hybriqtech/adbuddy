package tech.hybriq.adbuddy.control

import android.content.ContentResolver
import android.provider.Settings

/**
 * "Cut access" (spec §4.6): turns off wireless debugging and then USB/adb
 * debugging with WRITE_SECURE_SETTINGS, without needing the PC. Validated in
 * S1: adb_wifi_enabled=0 alone does not close tcpip mode; both together do.
 */
object CutAccess {
    private const val ADB_WIFI_ENABLED = "adb_wifi_enabled" // Settings.Global.ADB_WIFI_ENABLED is hidden

    /** Returns false when the permission is missing (setup was not accepted). */
    fun cut(resolver: ContentResolver): Boolean = try {
        Settings.Global.putInt(resolver, ADB_WIFI_ENABLED, 0)
        Settings.Global.putInt(resolver, Settings.Global.ADB_ENABLED, 0)
        true
    } catch (e: SecurityException) {
        false
    }

    /** Undoes cut(). A tcpip port that was open comes back until the host runs `adb usb` (spec §4.6). */
    fun restore(resolver: ContentResolver): Boolean = try {
        Settings.Global.putInt(resolver, Settings.Global.ADB_ENABLED, 1)
        Settings.Global.putInt(resolver, ADB_WIFI_ENABLED, 1)
        true
    } catch (e: SecurityException) {
        false
    }

    fun isCut(resolver: ContentResolver): Boolean =
        Settings.Global.getInt(resolver, Settings.Global.ADB_ENABLED, 1) == 0
}
