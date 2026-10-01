package tech.hybriq.adbuddy.files

import java.io.ByteArrayInputStream
import java.io.InputStream
import java.security.MessageDigest
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import tech.hybriq.adbuddy.bridge.FakeChannel
import tech.hybriq.adbuddy.bridge.Protocol

class BytesSource(val data: ByteArray, override val name: String = "a.bin", override val mime: String = "application/octet-stream") : UploadSource {
    override val size = data.size.toLong()
    override val sha256 = MessageDigest.getInstance("SHA-256").digest(data).joinToString("") { "%02x".format(it) }
    override fun open(offset: Long): InputStream = ByteArrayInputStream(data).also { it.skip(offset) }
}

class UploaderTest {
    private val id = "00112233445566778899aabbccddeeff"
    private val ch = FakeChannel()
    private val pauses = mutableListOf<Long>()
    private val up = Uploader(ch, pause = { pauses += it }, newId = { id })
    private val data = ByteArray(600 * 1024) { (it % 251).toByte() } // 3 frames: 256 + 256 + 88 KiB
    private val att = """{"attachment":{"id":"$id","name":"a.bin","mime":"application/octet-stream","size":${data.size},"sha256":"x"}}"""

    private fun ready(offset: Long) = FakeChannel.reply(Protocol.ATTACH_READY, """{"id":"$id","offset":$offset}""", "r")
    private fun done() = FakeChannel.reply(Protocol.ATTACH_DONE, att, "r")
    private fun error(code: String) = FakeChannel.reply(Protocol.ERROR, """{"code":"$code","text":"nope"}""", "r")

    /** A host that answers begin with ready(offset) and end with done. */
    private fun host(offset: () -> Long = { 0 }) {
        ch.handler = { s, cb ->
            when (s.type) {
                Protocol.ATTACH_BEGIN -> cb(ready(offset()))
                Protocol.ATTACH_END -> cb(done())
            }
            true
        }
    }

    @Test fun uploadsInFramesOf256KiBAndReturnsTheAttachment() = runBlocking {
        host()
        val progress = mutableListOf<Long>()
        val a = up.upload(BytesSource(data), onProgress = { progress += it })
        assertEquals(id, a.id)
        assertEquals(listOf(0L, 256L * 1024, 512L * 1024), ch.frames.map { it.second })
        assertEquals(listOf(256 * 1024, 256 * 1024, 88 * 1024), ch.frames.map { it.third.size })
        assertTrue(ch.frames.all { it.first == id })
        assertEquals(data.toList(), ch.frames.flatMap { it.third.toList() })
        assertEquals(data.size.toLong(), progress.last())
        val begin = Protocol.json.decodeFromJsonElement(tech.hybriq.adbuddy.bridge.AttachBegin.serializer(), ch.sent[0].payload)
        assertEquals(BytesSource(data).sha256, begin.sha256)
        assertEquals(listOf(Protocol.ATTACH_BEGIN, Protocol.ATTACH_END), ch.sent.map { it.type })
    }

    @Test fun resumesFromTheOffsetTheHostHas() = runBlocking {
        host { 256L * 1024 }
        up.upload(BytesSource(data))
        assertEquals(listOf(256L * 1024, 512L * 1024), ch.frames.map { it.second })
        assertEquals(data.copyOfRange(256 * 1024, data.size).toList(), ch.frames.flatMap { it.third.toList() })
    }

    @Test fun aLostAnswerToEndBeginsAgainAndTheHostSaysItIsDone() = runBlocking {
        var begins = 0
        ch.handler = { s, cb ->
            when (s.type) {
                Protocol.ATTACH_BEGIN -> { begins++; cb(ready(if (begins == 1) 0 else data.size.toLong())) }
                Protocol.ATTACH_END -> if (ch.sent.count { it.type == Protocol.ATTACH_END } == 1) cb(null) else cb(done())
            }
            true
        }
        val a = up.upload(BytesSource(data))
        assertEquals(id, a.id)
        assertEquals(2, begins)
        assertEquals("the second round sends no bytes", 3, ch.frames.size)
        assertTrue(pauses.isNotEmpty())
    }

    @Test fun aRefusedFrameRetriesFromWhatTheHostHas() = runBlocking {
        var begins = 0
        ch.refuseFramesFrom = 1
        ch.handler = { s, cb ->
            when (s.type) {
                Protocol.ATTACH_BEGIN -> { begins++; if (begins == 2) ch.refuseFramesFrom = -1; cb(ready(if (begins == 1) 0 else 256L * 1024)) }
                Protocol.ATTACH_END -> cb(done())
            }
            true
        }
        up.upload(BytesSource(data))
        assertEquals(2, begins)
        assertEquals(listOf(0L, 256L * 1024, 512L * 1024), ch.frames.map { it.second })
    }

    @Test fun errorsOfTheHostAreFinal() = runBlocking {
        ch.handler = { s, cb -> if (s.type == Protocol.ATTACH_BEGIN) cb(error("too_large")); true }
        try { up.upload(BytesSource(data)); fail() } catch (e: UploadFailed) { assertEquals("too_large", e.code); assertEquals("nope", e.text) }
        ch.handler = { s, cb -> if (s.type == Protocol.ATTACH_BEGIN) cb(ready(0)) else cb(error("checksum")); true }
        try { up.upload(BytesSource(data)); fail() } catch (e: UploadFailed) { assertEquals("checksum", e.code) }
    }

    @Test fun anImpossibleOffsetIsInvalid() = runBlocking {
        host { data.size + 1L }
        try { up.upload(BytesSource(data)); fail() } catch (e: UploadFailed) { assertEquals("invalid", e.code) }
    }

    @Test fun waitsWhileTheSocketQueueIsHigh() = runBlocking {
        host()
        ch.queued = 5L * 1024 * 1024
        val slow = Uploader(ch, pause = { pauses += it; ch.queued = 0 }, newId = { id })
        slow.upload(BytesSource(data))
        assertTrue(pauses.isNotEmpty())
        assertEquals(3, ch.frames.size)
    }

    @Test fun givesUpAfterRepeatedLostConnections() = runBlocking {
        ch.handler = { _, cb -> cb(null); true }
        try { up.upload(BytesSource(data)); fail() } catch (e: UploadFailed) { assertEquals("connection", e.code) }
    }

    @Test fun anEmptyFileHasNoFrames() = runBlocking {
        host()
        up.upload(BytesSource(ByteArray(0)))
        assertEquals(0, ch.frames.size)
        assertEquals(listOf(Protocol.ATTACH_BEGIN, Protocol.ATTACH_END), ch.sent.map { it.type })
    }
}
