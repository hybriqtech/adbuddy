package tech.hybriq.adbuddy.files

import java.security.MessageDigest
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.Chunk
import tech.hybriq.adbuddy.bridge.FakeChannel
import tech.hybriq.adbuddy.bridge.FileGet
import tech.hybriq.adbuddy.bridge.Protocol

class DownloadsTest {
    @get:Rule val tmp = TemporaryFolder()

    private val ch = FakeChannel()
    private val id = "00112233445566778899aabbccddeeff"
    private val data = ByteArray(300) { it.toByte() }
    private fun sha(b: ByteArray) = MessageDigest.getInstance("SHA-256").digest(b).joinToString("") { "%02x".format(it) }
    private val att = Attachment(id, "photo.png", "image/png", data.size.toLong(), sha(data))
    private lateinit var cache: FileCache
    private lateinit var dl: Downloads

    private fun make() {
        cache = FileCache(tmp.root)
        dl = Downloads(ch, cache, CoroutineScope(Dispatchers.Unconfined), Dispatchers.Unconfined)
    }

    private fun getOf(i: Int) = Protocol.json.decodeFromJsonElement(FileGet.serializer(), ch.sent[i].payload)
    private fun end(size: Int = data.size) = FakeChannel.reply(Protocol.FILE_END, """{"id":"$id","size":$size}""", "r")

    /** A host that serves `content` from the offset asked, in 100-byte frames, then file.end. */
    private fun serve(content: () -> ByteArray = { data }) {
        ch.handler = { s, cb ->
            val g = Protocol.json.decodeFromJsonElement(FileGet.serializer(), s.payload)
            val c = content()
            var off = g.offset.toInt()
            while (off < c.size) { val n = minOf(100, c.size - off); ch.emitFrame(Chunk(id, off.toLong(), c.copyOfRange(off, off + n))); off += n }
            cb(end(c.size))
            true
        }
    }

    @Test fun aDownloadEndsInTheCacheWithTheRightBytes() {
        make(); serve()
        dl.get(att)
        val st = dl.states.value[id] as DownloadState.Done
        assertEquals(data.toList(), st.file.readBytes().toList())
        assertEquals(0L, getOf(0).offset)
        assertFalse(cache.partFile(att).exists())
    }

    @Test fun aCachedFileIsNotAskedAgain() {
        make(); serve()
        dl.get(att)
        val n = ch.sent.size
        dl.get(att)
        assertEquals(n, ch.sent.size)
        assertTrue(dl.states.value[id] is DownloadState.Done)
    }

    @Test fun aPartialFileResumesFromItsLength() {
        make()
        cache.partFile(att).writeBytes(data.copyOfRange(0, 100))
        serve()
        dl.get(att)
        assertEquals(100L, getOf(0).offset)
        assertEquals(data.toList(), (dl.states.value[id] as DownloadState.Done).file.readBytes().toList())
    }

    @Test fun framesThatDoNotFollowAreDropped() {
        make()
        ch.handler = { _, cb ->
            ch.emitFrame(Chunk(id, 50, data.copyOfRange(50, 100)))   // a gap: dropped
            ch.emitFrame(Chunk(id, 0, data.copyOfRange(0, 100)))
            ch.emitFrame(Chunk(id, 0, data.copyOfRange(0, 100)))     // a repeat: dropped
            ch.emitFrame(Chunk(id, 100, data.copyOfRange(100, 300)))
            cb(end()); true
        }
        dl.get(att)
        assertEquals(data.toList(), (dl.states.value[id] as DownloadState.Done).file.readBytes().toList())
    }

    @Test fun aBadHashIsAskedAgainFromZeroOnceThenFails() {
        make()
        var calls = 0
        serve { calls++; if (calls == 1) ByteArray(data.size) else data }
        dl.get(att)
        assertEquals(2, ch.sent.size)
        assertEquals("the retry starts from 0", 0L, getOf(1).offset)
        assertTrue(dl.states.value[id] is DownloadState.Done)

        tmp.root.listFiles()!!.forEach { it.delete() }
        ch.sent.clear(); make()
        serve { ByteArray(data.size) }
        dl.get(att)
        assertEquals(2, ch.sent.size)
        assertTrue(dl.states.value[id] is DownloadState.Failed)
        assertFalse(cache.partFile(att).exists())
    }

    @Test fun anErrorFailsAndDropsWhatWasWritten() {
        make()
        ch.handler = { _, cb -> ch.emitFrame(Chunk(id, 0, data.copyOfRange(0, 100))); cb(FakeChannel.reply(Protocol.ERROR, """{"code":"not_found","text":"Gone"}""", "r")); true }
        dl.get(att)
        assertEquals("Gone", (dl.states.value[id] as DownloadState.Failed).text)
        assertFalse(cache.partFile(att).exists())
    }

    @Test fun aLostConnectionKeepsThePartialForNextTime() {
        make()
        ch.handler = { _, cb -> ch.emitFrame(Chunk(id, 0, data.copyOfRange(0, 100))); cb(null); true }
        dl.get(att)
        assertTrue(dl.states.value[id] is DownloadState.Failed)
        assertEquals(100L, cache.partFile(att).length())
        serve()
        dl.get(att)
        assertEquals(100L, getOf(1).offset)
        assertTrue(dl.states.value[id] is DownloadState.Done)
    }

    @Test fun anIdThatIsNotHexIsRefusedAndTouchesNoFile() {
        make(); serve()
        dl.get(att.copy(id = "../../x"))
        assertEquals(0, ch.sent.size)
        assertTrue(dl.states.value["../../x"] is DownloadState.Failed)
        assertEquals(0, tmp.root.listFiles()!!.size)
    }

    @Test fun theLateAnswerOfACancelledDownloadDoesNotFailTheNewOne() {
        make()
        val answers = mutableListOf<(tech.hybriq.adbuddy.bridge.Message?) -> Unit>()
        ch.handler = { _, cb -> answers += cb; true }
        dl.get(att)
        dl.cancel(id)
        dl.get(att)
        answers[0](null) // the connection ended for the first request, after it was cancelled
        assertTrue(dl.states.value[id] is DownloadState.Running)
    }

    @Test fun autoLimitIsTenMiBOfImagesAndFiveInRemoteMode() {
        val mib = 1024L * 1024
        fun a(mime: String, size: Long) = Attachment(id, "x", mime, size, "x")
        assertTrue(Downloads.autoLimit(a("image/jpeg", 10 * mib), remote = false))
        assertFalse(Downloads.autoLimit(a("image/jpeg", 10 * mib + 1), remote = false))
        assertTrue(Downloads.autoLimit(a("image/png", 5 * mib), remote = true))
        assertFalse(Downloads.autoLimit(a("image/png", 5 * mib + 1), remote = true))
        assertFalse(Downloads.autoLimit(a("application/pdf", 1), remote = false))
    }
}
