package tech.hybriq.adbuddy.files

import java.io.File

/**
 * Knobs for the emulator tests, honoured only by a debuggable app (the one
 * the tests install, where `run-as` can already write to the app's data). A
 * release is not debuggable: nothing here reaches it, whatever the file says.
 */
object DebugOverrides {
    /** Name, in the app's files folder, of the file that holds the cache cap in bytes. */
    const val CACHE_CAP_FILE = "debug-cache-cap"

    /** The cap in [file], or null when the app is not [debuggable] or the file is missing or not a positive number. */
    fun cacheCap(debuggable: Boolean, file: File): Long? {
        if (!debuggable) return null
        return runCatching { file.readText().trim().toLong() }.getOrNull()?.takeIf { it > 0 }
    }
}
