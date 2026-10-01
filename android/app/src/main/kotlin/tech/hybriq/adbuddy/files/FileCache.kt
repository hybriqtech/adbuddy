package tech.hybriq.adbuddy.files

import java.io.File
import tech.hybriq.adbuddy.bridge.Attachment

/**
 * The downloaded files, in the app's cache (spec M3 §6.4). The name is the
 * attachment's id plus its cleaned name: nothing the host or the agent says
 * can place a file outside the folder. Past maxBytes the oldest go first.
 */
class FileCache(private val dir: File, private val maxBytes: Long = 500L * 1024 * 1024) {
    init { dir.mkdirs() }

    fun finalFile(a: Attachment) = File(dir, a.id + "-" + FileNames.clean(a.name))
    fun partFile(a: Attachment) = File(dir, a.id + ".part")

    fun has(a: Attachment): Boolean = finalFile(a).let { it.isFile && it.length() == a.size }

    /** A file just used or opened is the newest. */
    fun touch(f: File) { f.setLastModified(System.currentTimeMillis()) }

    fun trim() {
        // A .part is a download in progress (or one to resume): never trimmed from under it.
        val files = dir.listFiles()?.filter { it.isFile && !it.name.endsWith(".part") } ?: return
        var total = files.sumOf { it.length() }
        for (f in files.sortedBy { it.lastModified() }) {
            if (total <= maxBytes) break
            total -= f.length()
            f.delete()
        }
    }
}
