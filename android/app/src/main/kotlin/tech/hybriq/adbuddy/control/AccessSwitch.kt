package tech.hybriq.adbuddy.control

/**
 * Cut access and its undo (spec §4.6), over any settings store, so it is
 * unit-tested. Cutting turns off wireless debugging and then adb debugging
 * (validated in S1: adb_wifi_enabled=0 alone does not close tcpip mode; both
 * together do), after saving what they were; restoring puts those values
 * back, never wireless debugging on blindly. Each change is read back before
 * it is reported done.
 */
class AccessSwitch(private val settings: Settings, private val saved: Saved) {
    /** Settings.Global: get is null for a setting never written; put throws SecurityException without WRITE_SECURE_SETTINGS. */
    interface Settings {
        fun get(name: String): Int?
        fun put(name: String, value: Int)
    }

    /** Where the values before the cut are kept (the app's private preferences); null removes. */
    interface Saved {
        fun get(name: String): Int?
        fun put(name: String, value: Int?)
    }

    fun isCut(): Boolean = settings.get(ADB_ENABLED) == 0

    /** true once both are read back as off; false without the permission or if a change did not stick. */
    fun cut(): Boolean {
        // Cutting again keeps the values of the first cut.
        if (!isCut()) {
            saved.put(ADB_ENABLED, settings.get(ADB_ENABLED) ?: 1)
            saved.put(ADB_WIFI_ENABLED, settings.get(ADB_WIFI_ENABLED) ?: 0)
        }
        return write(ADB_WIFI_ENABLED to 0, ADB_ENABLED to 0)
    }

    /**
     * Puts back what cut() saved; without it (cut outside ADBuddy) only adb
     * debugging comes back. A tcpip port that was open comes back too until
     * the host runs `adb usb` (spec §4.6).
     */
    fun restore(): Boolean {
        val adb = saved.get(ADB_ENABLED)?.takeIf { it != 0 } ?: 1
        val wifi = saved.get(ADB_WIFI_ENABLED) ?: 0
        if (!write(ADB_ENABLED to adb, ADB_WIFI_ENABLED to wifi)) return false
        saved.put(ADB_ENABLED, null)
        saved.put(ADB_WIFI_ENABLED, null)
        return true
    }

    /**
     * Forgets the values from before the cut once adb debugging is on again
     * without restore() (the person turned it on in Settings): a later cut
     * must not bring back what there was before the first one. Called on
     * every change of the settings (CutAccess.observe).
     */
    fun forgetIfRestored() {
        if (isCut()) return
        saved.put(ADB_ENABLED, null)
        saved.put(ADB_WIFI_ENABLED, null)
    }

    private fun write(vararg changes: Pair<String, Int>): Boolean {
        try {
            for ((name, value) in changes) settings.put(name, value)
        } catch (e: SecurityException) {
            return false
        }
        return changes.all { (name, value) -> (settings.get(name) ?: 0) == value }
    }

    companion object {
        const val ADB_ENABLED = "adb_enabled" // Settings.Global.ADB_ENABLED
        const val ADB_WIFI_ENABLED = "adb_wifi_enabled" // Settings.Global.ADB_WIFI_ENABLED is hidden
    }
}
