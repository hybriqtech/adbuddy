package tech.hybriq.adbuddy.control

/**
 * Cut access and its undo (spec v1 §4.6; spec 2026-10-18 R1-R4), over any settings store, so it is unit-tested.
 * Cutting turns off wireless debugging and then adb debugging (S1: both together close adbd and tcpip mode), after
 * saving what they were; restoring puts those values back as they were, never wireless debugging on blindly.
 *
 * "Cut" is OUR cut: the saved values are the record of a cut ADBuddy made and read back. Without it nothing is cut,
 * whatever is read (on the user's Pixel, Android 17, the app read adb_enabled=0 while adb worked: Task 1's diagnosis).
 * A readable setting at 1 means someone gave access back by hand: the record goes. An unreadable one proves nothing.
 *
 * False readings (addendum 2026-10-19, R16-R19): Android 17 gives apps adb_enabled=0 while `adb shell` says 1, and
 * reads adb_wifi_enabled right. A 1 read is always believed (the false readings are 0); a 0 is doubted only when the
 * rest contradicts it (adbZeroIsDoubtful). Then cut() saves adb_enabled=1 and marks the record (ADB_DOUBTED), and
 * restore() takes a 0 read after writing 1 as done only with that mark. A put() that says false is a failed write.
 * So on Android 17 cut() is confirmed by put() and by wireless debugging reading 0, not by adb_enabled: the screen
 * checks the cut afterwards (CutConfirm, R22) and V2 of the addendum measures it from the shell.
 */
class AccessSwitch(private val settings: Settings, private val saved: Saved) {
    /** Settings.Global: get is null for a setting never written and THROWS SecurityException when it cannot be read; put throws without WRITE_SECURE_SETTINGS and returns false when the write failed (Settings.Global.putInt). */
    interface Settings {
        fun get(name: String): Int?
        fun put(name: String, value: Int): Boolean
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

    /** true once wireless debugging reads back 0 and adb_enabled does not read back 1 (R18); false without the permission, if a change did not stick or cannot be read back. */
    fun cut(): Boolean {
        forgetIfRestored() // a record that a readable setting contradicts is stale: do not keep it
        // Cutting again keeps the values of the first cut.
        val savedNow = !hasRecord()
        if (savedNow) {
            val adb = read(ADB_ENABLED, missing = 1)
            val wifi = read(ADB_WIFI_ENABLED, missing = 0)
            // A 0 that wireless debugging contradicts is Android 17's false reading, not the setting (R16);
            // the record says so, for restore() to read back past it (R19).
            val doubted = adb != null && adbZeroIsDoubtful(adb, wifi)
            saved.put(ADB_ENABLED, if (adb == null || doubted) 1 else adb)
            saved.put(ADB_WIFI_ENABLED, wifi ?: 0)
            saved.put(ADB_DOUBTED, if (doubted) 1 else null)
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
        // Only wireless debugging saved: a record of a build before the addendum, which kept Android 17's false 0 (R23).
        val legacy = savedAdb == 0 && savedWifi == 1
        val adb = if (legacy) 1 else savedAdb
        val doubted = legacy || saved.get(ADB_DOUBTED) == 1
        if (!write(ADB_ENABLED to adb, ADB_WIFI_ENABLED to savedWifi, doubted = doubted)) return false
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
        saved.put(ADB_DOUBTED, null)
    }

    /** Writes in order and reads back; false as soon as Android refuses one (an exception, or put() saying false: R18). */
    private fun write(vararg changes: Pair<String, Int>, doubted: Boolean = false): Boolean = try {
        val puts = changes.map { (name, value) -> settings.put(name, value) }
        puts.all { it } &&
            changes.all { (name, value) -> readsBackAs(name, value, doubted) }
    } catch (e: SecurityException) {
        false
    }

    /**
     * Whether [name] reads back as [value] after writing it (never written = 0; unreadable never passes, R2).
     * adb_enabled=1 also passes with a 0 when [doubted]: the record says the 1 was saved over a doubtful 0 (R19), so
     * Android 17's false 0 cannot say the write did not stick. Without that mark a 0 is believed.
     * adb_enabled=0 still fails with a 1: a 1 read is never false (R18).
     */
    private fun readsBackAs(name: String, value: Int, doubted: Boolean): Boolean {
        val now = read(name, missing = 0) ?: return false
        if (now == value) return true
        return doubted && name == ADB_ENABLED && value == 1 && now == 0
    }

    companion object {
        const val ADB_ENABLED = "adb_enabled" // Settings.Global.ADB_ENABLED
        const val ADB_WIFI_ENABLED = "adb_wifi_enabled" // Settings.Global.ADB_WIFI_ENABLED is hidden

        /** In the record only (never a setting): 1 when the adb_enabled saved as 1 had been read as a doubtful 0 (R19). */
        const val ADB_DOUBTED = "adb_enabled_doubted"

        /**
         * A 0 read for adb_enabled that the rest contradicts (addendum 2026-10-19, R16-R17): wireless debugging reads
         * 1, or a channel with the PC is alive (it goes through adbd). Detected by the contradiction, never by the
         * Android version. A 1 is never doubted, and a 0 with nothing against it is believed.
         */
        fun adbZeroIsDoubtful(adb: Int?, wifi: Int?, channelUp: Boolean = false): Boolean = adb == 0 && (wifi == 1 || channelUp)
    }
}
