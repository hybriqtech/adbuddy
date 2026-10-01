package tech.hybriq.adbuddy.files

import java.util.UUID
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.sync.withPermit
import tech.hybriq.adbuddy.bridge.Attachment

enum class DraftStatus { UPLOADING, DONE, FAILED }

/** An attachment of the message being written: uploading, ready to send (attachment) or failed. */
data class Draft(
    val key: String,
    val name: String,
    val mime: String,
    val size: Long,
    val progress: Long,
    val status: DraftStatus,
    val attachment: Attachment? = null,
    /** The host's text when it refused; empty when there is none (a lost connection, an I/O error). */
    val error: String = "",
)

/**
 * The attachments of the message being written (spec M3 §6.4): at most
 * MAX_PARALLEL uploads at once (the host allows 4 per connection), each
 * with its progress. Sending waits for every draft to be DONE (Composer).
 */
class Drafts(
    private val scope: CoroutineScope,
    private val uploader: Uploader,
    private val captures: Captures,
    private val newKey: () -> String = { UUID.randomUUID().toString() },
) {
    private val _items = MutableStateFlow<List<Draft>>(emptyList())
    val items: StateFlow<List<Draft>> = _items.asStateFlow()

    private val _errors = MutableSharedFlow<String>(extraBufferCapacity = 8)

    /** The host's text of each refused upload or capture, for a snackbar. */
    val errors: SharedFlow<String> = _errors.asSharedFlow()

    private val gate = Semaphore(MAX_PARALLEL)
    private val lock = Any()
    private val jobs = HashMap<String, Job>()
    private val sources = HashMap<String, UploadSource>()
    private val captureKeys = HashSet<String>()
    private val ids = HashMap<String, String>() // a retry goes on from the host's offset: same id

    fun add(src: UploadSource): String {
        val key = newKey()
        synchronized(lock) { sources[key] = src } // before the draft is visible: remove() always finds the source
        _items.update { it + Draft(key, src.name, src.mime, src.size, 0, DraftStatus.UPLOADING) }
        start(key, src)
        return key
    }

    /**
     * A file picked and still being copied (up to 200 MiB, hashed on the way): its draft is visible, and Send
     * waits for it, before its source exists. bind() gives it the source; remove() before that drops it.
     */
    fun reserve(): String {
        val key = newKey()
        _items.update { it + Draft(key, RESERVED_NAME, "", 0, 0, DraftStatus.UPLOADING) }
        return key
    }

    /** The copy finished: the reserved draft uploads src. If it was removed meanwhile, the copy is closed. */
    fun bind(key: String, src: UploadSource) {
        val alive = synchronized(lock) {
            if (_items.value.any { it.key == key }) { sources[key] = src; true } else false
        }
        if (!alive) {
            src.close()
            return
        }
        update(key) { it.copy(name = src.name, mime = src.mime, size = src.size) }
        start(key, src)
    }

    fun capture() {
        val key = newKey()
        synchronized(lock) { captureKeys += key }
        _items.update { it + Draft(key, CAPTURE_NAME, "image/png", 0, 0, DraftStatus.UPLOADING) }
        takeCapture(key)
    }

    private fun takeCapture(key: String) {
        captures.take { a, text ->
            when {
                a != null -> update(key) { it.copy(name = a.name, mime = a.mime, size = a.size, progress = a.size, status = DraftStatus.DONE, attachment = a) }
                else -> fail(key, text.orEmpty())
            }
        }
    }

    fun retry(key: String) {
        val (src, isCapture) = synchronized(lock) { sources[key] to (key in captureKeys) }
        if (src == null && !isCapture) return
        var won = false
        _items.update { l ->
            won = false
            l.map { if (it.key == key && it.status == DraftStatus.FAILED) { won = true; it.copy(status = DraftStatus.UPLOADING, progress = 0, error = "") } else it }
        }
        if (!won) return // another retry (or a remove) got there first
        if (src != null) start(key, src) else takeCapture(key)
    }

    fun remove(key: String) {
        val (job, src) = synchronized(lock) { ids.remove(key); captureKeys.remove(key); jobs.remove(key) to sources.remove(key) }
        job?.cancel()
        src?.close()
        _items.update { l -> l.filterNot { it.key == key } }
    }

    /** After sending: nothing is kept. */
    fun clear() {
        _items.value.map { it.key }.forEach { remove(it) }
    }

    private fun start(key: String, src: UploadSource) {
        val job = scope.launch(start = CoroutineStart.LAZY) {
            gate.withPermit {
                try {
                    val id = synchronized(lock) { ids.getOrPut(key) { uploader.newId() } }
                    val a = uploader.upload(src, id, onProgress = { p -> update(key) { it.copy(progress = p) } })
                    update(key) { it.copy(progress = src.size, status = DraftStatus.DONE, attachment = a) }
                } catch (e: CancellationException) {
                    throw e
                } catch (e: UploadFailed) {
                    fail(key, e.text)
                } catch (e: Exception) { // the scope has no handler: an uncaught one would end the process
                    fail(key, "")
                }
            }
        }
        job.invokeOnCompletion { synchronized(lock) { if (jobs[key] === job) jobs.remove(key) } }
        val alive = synchronized(lock) { (sources[key] === src).also { if (it) jobs[key] = job } }
        if (alive) job.start() else job.cancel() // removed meanwhile: remove() already closed the source
    }

    private fun fail(key: String, text: String) {
        update(key) { it.copy(status = DraftStatus.FAILED, error = text) }
        if (text.isNotEmpty()) _errors.tryEmit(text)
    }

    private fun update(key: String, f: (Draft) -> Draft) = _items.update { l -> l.map { if (it.key == key) f(it) else it } }

    companion object {
        const val MAX_PARALLEL = 3
        const val CAPTURE_NAME = "screenshot.png"
        const val RESERVED_NAME = "…"
    }
}
