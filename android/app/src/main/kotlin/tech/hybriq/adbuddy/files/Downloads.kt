package tech.hybriq.adbuddy.files

import java.io.File
import java.io.FileOutputStream
import java.security.MessageDigest
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.Channel
import tech.hybriq.adbuddy.bridge.Chunk
import tech.hybriq.adbuddy.bridge.ErrorReply
import tech.hybriq.adbuddy.bridge.FileCancel
import tech.hybriq.adbuddy.bridge.Frame
import tech.hybriq.adbuddy.bridge.FileGet
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.Protocol

sealed interface DownloadState {
    class Running(val done: Long, val size: Long) : DownloadState
    class Done(val file: File) : DownloadState

    /** text: the host's, or empty (a lost connection: what was downloaded stays, and tapping again goes on). */
    class Failed(val text: String) : DownloadState
}

/**
 * Downloads attachments (protocol spec §6.2): file.get from the bytes already
 * in `<id>.part`, each frame written at its offset (one that does not follow
 * what is there is dropped, as those of a cancelled download still arriving),
 * and at file.end the size and SHA-256 are checked. A wrong hash is asked once
 * more from 0. Frames arrive on OkHttp's reader thread; the hash is checked
 * off it.
 */
class Downloads(
    private val channel: Channel,
    private val cache: FileCache,
    private val scope: CoroutineScope,
    private val io: CoroutineDispatcher = Dispatchers.IO,
) {
    private class Active(val a: Attachment, var written: Long, val retried: Boolean)

    private val lock = Any()
    private val active = HashMap<String, Active>()
    private val _states = MutableStateFlow<Map<String, DownloadState>>(emptyMap())
    val states: StateFlow<Map<String, DownloadState>> = _states.asStateFlow()

    init { channel.listenFrames { onChunk(it) } }

    fun get(a: Attachment) = synchronized(lock) {
        // The id comes from the host and names files: 32 lower-case hex digits or nothing (never a path).
        if (Frame.parseId(a.id) == null) {
            set(a.id, DownloadState.Failed(""))
            return@synchronized
        }
        if (active.containsKey(a.id)) return@synchronized
        if (cache.has(a)) {
            cache.touch(cache.finalFile(a))
            set(a.id, DownloadState.Done(cache.finalFile(a)))
        } else {
            start(a, retried = false, fromZero = false)
        }
    }

    fun cancel(id: String) = synchronized(lock) {
        if (active.remove(id) != null) {
            channel.send(Protocol.FILE_CANCEL, Protocol.json.encodeToJsonElement(FileCancel.serializer(), FileCancel(id)))
            _states.update { it - id }
        }
    }

    /** Call it holding the lock. */
    private fun start(a: Attachment, retried: Boolean, fromZero: Boolean) {
        val part = cache.partFile(a)
        if (fromZero || (part.isFile && part.length() > a.size)) part.delete()
        val have = if (part.isFile) part.length() else 0L
        active[a.id] = Active(a, have, retried)
        set(a.id, DownloadState.Running(have, a.size))
        val rid = channel.request(Protocol.FILE_GET, Protocol.json.encodeToJsonElement(FileGet.serializer(), FileGet(a.id, have))) { m ->
            scope.launch(io) { finish(a.id, m) }
        }
        if (rid == null) {
            active.remove(a.id)
            set(a.id, DownloadState.Failed(""))
        }
    }

    private fun onChunk(c: Chunk) = synchronized(lock) {
        val act = active[c.id] ?: return@synchronized
        if (c.offset != act.written || act.written + c.data.size > act.a.size) return@synchronized
        try {
            FileOutputStream(cache.partFile(act.a), true).use { it.write(c.data) }
        } catch (e: java.io.IOException) {
            active.remove(c.id)
            set(c.id, DownloadState.Failed(""))
            return@synchronized
        }
        act.written += c.data.size
        set(c.id, DownloadState.Running(act.written, act.a.size))
    }

    private fun finish(id: String, m: Message?) {
        val act = synchronized(lock) { active[id] } ?: return
        val part = cache.partFile(act.a)
        when {
            m == null -> fail(id, "", keep = true)
            m.type == Protocol.ERROR -> fail(id, Protocol.payload<ErrorReply>(m)?.let { it.text.ifEmpty { it.code } } ?: "", keep = false)
            else -> {
                val ok = part.isFile && part.length() == act.a.size && sha256(part) == act.a.sha256
                synchronized(lock) {
                    if (active[id] !== act) return // cancelled meanwhile
                    when {
                        ok -> {
                            val f = cache.finalFile(act.a)
                            f.delete()
                            if (part.renameTo(f)) {
                                active.remove(id)
                                set(id, DownloadState.Done(f))
                                scope.launch(io) { cache.trim() }
                            } else {
                                fail(id, "", keep = false)
                            }
                        }
                        !act.retried -> { active.remove(id); start(act.a, retried = true, fromZero = true) } // wrong bytes: once more from 0
                        else -> fail(id, "", keep = false)
                    }
                }
            }
        }
    }

    private fun fail(id: String, text: String, keep: Boolean) = synchronized(lock) {
        val act = active.remove(id) ?: return@synchronized
        if (!keep) cache.partFile(act.a).delete()
        set(id, DownloadState.Failed(text))
    }

    private fun set(id: String, s: DownloadState) = _states.update { it + (id to s) }

    private fun sha256(f: File): String {
        val d = MessageDigest.getInstance("SHA-256")
        f.inputStream().use { i ->
            val buf = ByteArray(64 * 1024)
            while (true) { val n = i.read(buf); if (n < 0) break; d.update(buf, 0, n) }
        }
        return d.digest().joinToString("") { "%02x".format(it) }
    }

    companion object {
        private const val MIB = 1024L * 1024

        /** Images download by themselves up to 10 MiB, 5 MiB in remote mode (protocol spec §6.2); the rest waits for a tap. */
        fun autoLimit(a: Attachment, remote: Boolean): Boolean = a.mime.startsWith("image/") && a.size <= (if (remote) 5 else 10) * MIB
    }
}
