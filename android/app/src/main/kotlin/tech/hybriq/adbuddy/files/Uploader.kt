package tech.hybriq.adbuddy.files

import java.io.InputStream
import java.security.SecureRandom
import kotlin.coroutines.resume
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.delay
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.serialization.json.JsonElement
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.AttachBegin
import tech.hybriq.adbuddy.bridge.AttachDone
import tech.hybriq.adbuddy.bridge.AttachEnd
import tech.hybriq.adbuddy.bridge.AttachReady
import tech.hybriq.adbuddy.bridge.Channel
import tech.hybriq.adbuddy.bridge.ErrorCode
import tech.hybriq.adbuddy.bridge.ErrorReply
import tech.hybriq.adbuddy.bridge.Frame
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.Protocol

/** A file ready to upload: its name, size and SHA-256 are known (the app hashes it before it begins). */
interface UploadSource : AutoCloseable {
    val name: String
    val mime: String
    val size: Long
    val sha256: String

    /** The bytes from offset on. */
    fun open(offset: Long): InputStream

    override fun close() {}
}

/** The upload cannot go on. code: a protocol error code, or "connection" after repeated losses; text: the host's, maybe empty. */
class UploadFailed(val code: String, val text: String) : Exception("$code: $text")

/**
 * Uploads one file (protocol spec §6.1): attach.begin, binary frames from the
 * offset the host has, attach.end. A lost connection begins again with the
 * same id and goes on from the host's offset; errors of the host are final.
 */
class Uploader(
    private val channel: Channel,
    private val pause: suspend (ms: Long) -> Unit = { delay(it) },
    val newId: () -> String = { Frame.idString(ByteArray(Frame.ID_LEN).also { SecureRandom().nextBytes(it) }) },
) {
    private sealed interface Round {
        class Done(val attachment: Attachment) : Round
        class Retry(val progressed: Boolean) : Round
    }

    suspend fun upload(src: UploadSource, id: String = newId(), onProgress: (Long) -> Unit = {}): Attachment {
        var failures = 0
        while (true) {
            if (!channel.connected.value) channel.connected.first { it }
            when (val r = round(src, id, onProgress)) {
                is Round.Done -> return r.attachment
                is Round.Retry -> {
                    failures = if (r.progressed) 1 else failures + 1
                    if (failures > MAX_FAILURES) throw UploadFailed("connection", "")
                    pause(RETRY_MS)
                }
            }
        }
    }

    private suspend fun round(src: UploadSource, id: String, onProgress: (Long) -> Unit): Round {
        val begin = call(Protocol.ATTACH_BEGIN, Protocol.json.encodeToJsonElement(AttachBegin.serializer(), AttachBegin(id, src.name, src.mime, src.size, src.sha256)))
            ?: return Round.Retry(false)
        val ready = expect<AttachReady>(begin, Protocol.ATTACH_READY)
        if (ready.offset !in 0..src.size) throw UploadFailed(ErrorCode.INVALID, "")
        var off = ready.offset
        var progressed = false
        onProgress(off)
        if (off < src.size) {
            src.open(off).use { input ->
                val buf = ByteArray(Frame.MAX_DATA)
                while (off < src.size) {
                    currentCoroutineContext().ensureActive()
                    // OkHttp closes the socket past 16 MiB queued: wait for room (protocol spec §6.4).
                    while (channel.queuedBytes() > HIGH_WATER) {
                        pause(WAIT_MS)
                        if (!channel.connected.value) return Round.Retry(progressed)
                    }
                    val n = input.read(buf, 0, minOf(buf.size.toLong(), src.size - off).toInt())
                    if (n <= 0) throw UploadFailed(ErrorCode.FAILED, "") // the file changed under the upload
                    if (!channel.sendFrame(id, off, buf.copyOf(n))) return Round.Retry(progressed)
                    off += n
                    progressed = true
                    onProgress(off)
                }
            }
        }
        val end = call(Protocol.ATTACH_END, Protocol.json.encodeToJsonElement(AttachEnd.serializer(), AttachEnd(id))) ?: return Round.Retry(progressed)
        return Round.Done(expect<AttachDone>(end, Protocol.ATTACH_DONE).attachment)
    }

    private inline fun <reified T> expect(m: Message, type: String): T {
        if (m.type == Protocol.ERROR) {
            val e = Protocol.payload<ErrorReply>(m)
            throw UploadFailed(e?.code ?: ErrorCode.FAILED, e?.text ?: "")
        }
        if (m.type != type) throw UploadFailed(ErrorCode.INVALID, "")
        return Protocol.payload<T>(m) ?: throw UploadFailed(ErrorCode.INVALID, "")
    }

    /** The answer to a request, or null when the connection ended first or it could not be sent. */
    private suspend fun call(type: String, payload: JsonElement): Message? = suspendCancellableCoroutine { cont ->
        val rid = channel.request(type, payload) { m -> if (cont.isActive) cont.resume(m) }
        if (rid == null && cont.isActive) cont.resume(null)
    }

    private companion object {
        const val HIGH_WATER = 4L * 1024 * 1024
        const val WAIT_MS = 50L
        const val RETRY_MS = 1_000L
        const val MAX_FAILURES = 5
    }
}
