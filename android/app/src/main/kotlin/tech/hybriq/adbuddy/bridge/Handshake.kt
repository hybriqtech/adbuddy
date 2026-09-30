package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.Serializable

/** The handshake the host writes on stdin (`content write …/handshake`, spec §5.2). */
@Serializable
data class Handshake(val token: String, val port: Int, val host: String = "", val nonce: String) {
    // Keeps the token and the nonce out of logs and crash reports.
    override fun toString() = "Handshake(token=<redacted>, port=$port, host=$host, nonce=<redacted>)"
}

object HandshakeParser {
    private const val MAX_BYTES = 4096
    private val TOKEN = Regex("^[0-9a-f]{64}$")
    private val HOST = Regex("^[A-Za-z0-9 ._-]{1,40}$")
    private val NONCE = Regex("^[0-9a-f]{32}$")

    /** Reads at most MAX_BYTES + 1 bytes (InputStream.readNBytes needs API 33). */
    fun readLimited(input: java.io.InputStream): ByteArray {
        val out = java.io.ByteArrayOutputStream()
        val buf = ByteArray(1024)
        while (out.size() <= MAX_BYTES) {
            val n = input.read(buf)
            if (n < 0) break
            out.write(buf, 0, n)
        }
        return out.toByteArray()
    }

    /** Returns the handshake, or null when anything is off: then the app keeps the previous one. */
    fun parse(bytes: ByteArray): Handshake? {
        if (bytes.size > MAX_BYTES) return null
        val h = try {
            Protocol.json.decodeFromString(Handshake.serializer(), bytes.decodeToString())
        } catch (e: IllegalArgumentException) {
            return null
        }
        if (!TOKEN.matches(h.token) || h.port !in 1024..65535 || !NONCE.matches(h.nonce)) return null
        val name = h.host.trim()
        return h.copy(host = if (HOST.matches(name)) name else "PC")
    }
}

/** Only the shell user (adb) may talk to the provider (spec §5.2). */
object Caller {
    /** android.os.Process.SHELL_UID, a hidden constant. */
    const val SHELL_UID = 2000

    fun isShell(uid: Int): Boolean = uid == SHELL_UID
}
