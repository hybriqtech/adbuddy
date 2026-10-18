package tech.hybriq.adbuddy.control

/**
 * Cut access and its undo (spec v1 §4.6; spec 2026-10-18 R1-R4), over any settings store, so it is unit-tested.
 * Cutting turns off wireless debugging and then adb debugging (S1: both together close adbd and tcpip mode), after
 * saving what they were; restoring puts those values back as they were, never wireless debugging on blindly.
 *
 * "Cut" is OUR cut: the saved values are the record of a cut ADBuddy made and read back. Without it nothing is cut,
 * whatever is read (on the user's Pixel, Android 17, the app read adb_enabled=0 while adb worked: Task 1's diagnosis).
 * A readable setting at 1 means someone gave access back by hand: the record goes. An unreadable one proves nothing.
 */
class AccessSwitch(private val settings: Settings, private val saved: Saved) {
    /** Settings.Global: get is null for a setting never written and THROWS SecurityException when it cannot be read; put throws without WRITE_SECURE_SETTINGS. */
    interface Settings {
        fun get(name: String): Int?
        fun put(name: String, value: Int)
    }

    /** Where the values before the cut are kept (the app's private preferences); null removes. */
    interface Saved {
        fun get(name: String): Int?
        fun put(name: String, value: Int?)
    }

    /** The value, with a never-written setting as [missing]; null when it cannot be read. */
    private fun read(name: String, missing: Int): Int? = try {
        settings.get(name) ?: missing
    } catch (e: SecurityException) {
        null
    }

    private fun hasRecord(): Boolean = saved.get(ADB_ENABLED) != null && saved.get(ADB_WIFI_ENABLED) != null

    /** A readable setting says access is on (USB or wireless debugging at 1). adb_enabled never written counts as on (getInt(…, 1) of old). */
    private fun readAsOn(): Boolean = read(ADB_ENABLED, missing = 1) == 1 || read(ADB_WIFI_ENABLED, missing = 0) == 1

    fun isCut(): Boolean = hasRecord() && !readAsOn()

    /** true once both are read back as off; false without the permission, if a change did not stick or cannot be read back. */
    fun cut(): Boolean {
        // Cutting again keeps the values of the first cut.
        val savedNow = !hasRecord()
        if (savedNow) {
            saved.put(ADB_ENABLED, read(ADB_ENABLED, missing = 1) ?: 1)
            saved.put(ADB_WIFI_ENABLED, read(ADB_WIFI_ENABLED, missing = 0) ?: 0)
        }
        val ok = write(ADB_WIFI_ENABLED to 0, ADB_ENABLED to 0)
        if (!ok && savedNow) forget() // a cut that did not stick must not leave a record that says "cut"
        return ok
    }

    /**
     * Puts back what cut() saved, as it was (adb_enabled=0 included). Without a record: access already on is left as
     * it is (wireless debugging is never turned off), otherwise only adb debugging comes back and wireless debugging is
     * not touched (its value is unknown or already off). A record of both off is never guessed: nothing is written, the
     * record stays and needsManualRestore() says so (the screen sends the person to Developer options). A tcpip port
     * that was open comes back too until the host runs `adb usb` (spec §4.6).
     */
    fun restore(): Boolean {
        val savedAdb = saved.get(ADB_ENABLED)
        val savedWifi = saved.get(ADB_WIFI_ENABLED)
        if (savedAdb == null || savedWifi == null) {
            if (readAsOn()) return true
            return write(ADB_ENABLED to 1)
        }
        if (savedAdb == 0 && savedWifi == 0) return false
        if (!write(ADB_ENABLED to savedAdb, ADB_WIFI_ENABLED to savedWifi)) return false
        forget()
        return true
    }

    /** Our record says both were off before the cut (or they were read so): restore() will not guess what to turn on. */
    fun needsManualRestore(): Boolean = saved.get(ADB_ENABLED) == 0 && saved.get(ADB_WIFI_ENABLED) == 0

    /**
     * Forgets the record once a readable setting says access is on again without restore() (the person turned it on
     * in Settings): a later cut must not bring back what there was before the first one. Called on every change of
     * the settings (CutAccess.observe). An unreadable setting keeps the record.
     */
    fun forgetIfRestored() {
        if (hasRecord() && readAsOn()) forget()
    }

    private fun forget() {
        saved.put(ADB_ENABLED, null)
        saved.put(ADB_WIFI_ENABLED, null)
    }

    private fun write(vararg changes: Pair<String, Int>): Boolean = try {
        for ((name, value) in changes) settings.put(name, value)
        changes.all { (name, value) -> (settings.get(name) ?: 0) == value }
    } catch (e: SecurityException) {
        false
    }

    companion object {
        const val ADB_ENABLED = "adb_enabled" // Settings.Global.ADB_ENABLED
        const val ADB_WIFI_ENABLED = "adb_wifi_enabled" // Settings.Global.ADB_WIFI_ENABLED is hidden
    }
}
