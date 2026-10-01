package tech.hybriq.adbuddy.files

import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import java.io.File
import java.io.FileInputStream
import java.io.InputStream
import java.security.MessageDigest
import java.util.UUID

/**
 * A file picked on the phone, copied to the app's cache (the picker gives a
 * Uri, not a path, and the upload may outlive the permission) and hashed while
 * copying. Never log its name: it is the person's.
 */
class UriSource private constructor(
    private val file: File,
    override val name: String,
    override val mime: String,
    override val size: Long,
    override val sha256: String,
) : UploadSource {
    override fun open(offset: Long): InputStream = FileInputStream(file).also { it.channel.position(offset) }

    override fun close() {
        file.delete()
    }

    companion object {
        /** The most the host takes (spec v1 §5.3). */
        const val MAX_SIZE = 200L * 1024 * 1024

        /** Blocking: call it off the main thread. Throws UploadFailed("too_large", "") or IOException. */
        fun copy(context: Context, uri: Uri): UriSource {
            val resolver = context.contentResolver
            var name = "file"
            var declared = -1L
            resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE), null, null, null)?.use { c ->
                if (c.moveToFirst()) {
                    c.getString(0)?.let { name = it }
                    if (!c.isNull(1)) declared = c.getLong(1)
                }
            }
            if (declared > MAX_SIZE) throw UploadFailed("too_large", "")
            val dir = File(context.cacheDir, DIR).apply { mkdirs() }
            val out = File(dir, UUID.randomUUID().toString())
            val digest = MessageDigest.getInstance("SHA-256")
            var total = 0L
            try {
                val input = resolver.openInputStream(uri) ?: throw java.io.IOException("cannot open")
                input.use { i -> out.outputStream().use { o ->
                    val buf = ByteArray(64 * 1024)
                    while (true) {
                        val n = i.read(buf)
                        if (n < 0) break
                        total += n
                        if (total > MAX_SIZE) throw UploadFailed("too_large", "")
                        digest.update(buf, 0, n)
                        o.write(buf, 0, n)
                    }
                } }
            } catch (e: Exception) {
                out.delete()
                throw e
            }
            val mime = resolver.getType(uri) ?: "application/octet-stream"
            return UriSource(out, FileNames.clean(name), mime, total, digest.digest().joinToString("") { "%02x".format(it) })
        }

        /** What a dead process left behind. */
        fun clearCache(context: Context) {
            File(context.cacheDir, DIR).deleteRecursively()
        }

        private const val DIR = "uploads"
    }
}
