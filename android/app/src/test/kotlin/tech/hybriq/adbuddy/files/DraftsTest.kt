package tech.hybriq.adbuddy.files

import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.FakeChannel
import tech.hybriq.adbuddy.bridge.Protocol

class DraftsTest {
    private val id = "00112233445566778899aabbccddeeff"
    private val ch = FakeChannel()
    private var n = 0
    // Unconfined: with the FakeChannel answering at once, each upload ends inside add().
    private val drafts = Drafts(CoroutineScope(Dispatchers.Unconfined), Uploader(ch, pause = {}, newId = { id }), Captures(ch)) { "k${++n}" }
    private val att = """{"attachment":{"id":"$id","name":"a.bin","mime":"image/png","size":3,"sha256":"x"}}"""

    private fun hostOk() {
        ch.handler = { s, cb ->
            when (s.type) {
                Protocol.ATTACH_BEGIN -> cb(FakeChannel.reply(Protocol.ATTACH_READY, """{"id":"$id","offset":0}""", "r"))
                Protocol.ATTACH_END -> cb(FakeChannel.reply(Protocol.ATTACH_DONE, att, "r"))
                Protocol.CAPTURE_REQUEST -> cb(FakeChannel.reply(Protocol.CAPTURE_READY, att, "r"))
            }
            true
        }
    }

    @Test fun anUploadEndsDoneWithItsAttachment() {
        hostOk()
        val key = drafts.add(BytesSource(byteArrayOf(1, 2, 3), "a.bin"))
        val d = drafts.items.value.single()
        assertEquals(key, d.key)
        assertEquals(DraftStatus.DONE, d.status)
        assertEquals(id, d.attachment?.id)
    }

    @Test fun aFailedUploadShowsTheHostsTextAndRetryTriesAgain() = runBlocking {
        ch.handler = { s, cb -> if (s.type == Protocol.ATTACH_BEGIN) cb(FakeChannel.reply(Protocol.ERROR, """{"code":"too_large","text":"Too big"}""", "r")); true }
        val seen = mutableListOf<String>()
        val job = launch(Dispatchers.Unconfined, start = CoroutineStart.UNDISPATCHED) { drafts.errors.collect { seen += it } }
        val key = drafts.add(BytesSource(byteArrayOf(1, 2, 3)))
        assertEquals(DraftStatus.FAILED, drafts.items.value.single().status)
        assertEquals("Too big", drafts.items.value.single().error)
        assertEquals(listOf("Too big"), seen)
        hostOk()
        drafts.retry(key)
        job.cancel()
        assertEquals(DraftStatus.DONE, drafts.items.value.single().status)
    }

    @Test fun aCaptureBecomesADoneDraft() {
        hostOk()
        drafts.capture()
        val d = drafts.items.value.single()
        assertEquals(DraftStatus.DONE, d.status)
        assertEquals("image/png", d.mime)
        assertEquals(Protocol.CAPTURE_REQUEST, ch.sent.single().type)
    }

    @Test fun aFailedCaptureFailsTheDraft() {
        ch.handler = { _, cb -> cb(FakeChannel.reply(Protocol.ERROR, """{"code":"failed","text":"Access is cut"}""", "r")); true }
        drafts.capture()
        assertEquals("Access is cut", drafts.items.value.single().error)
    }

    @Test fun removeAndClearDropDrafts() {
        hostOk()
        val a = drafts.add(BytesSource(byteArrayOf(1)))
        drafts.add(BytesSource(byteArrayOf(2)))
        drafts.remove(a)
        assertEquals(1, drafts.items.value.size)
        drafts.clear()
        assertTrue(drafts.items.value.isEmpty())
    }

    @Test fun aReservedDraftBlocksSendUntilItsCopyIsBound() {
        hostOk()
        val key = drafts.reserve()
        assertEquals(DraftStatus.UPLOADING, drafts.items.value.single().status)
        drafts.bind(key, BytesSource(byteArrayOf(1, 2, 3), "a.bin"))
        val d = drafts.items.value.single()
        assertEquals(key, d.key)
        assertEquals("a.bin", d.name)
        assertEquals(DraftStatus.DONE, d.status)
    }

    @Test fun aCopyThatFinishesAfterTheDraftWasRemovedIsClosed() {
        hostOk()
        val key = drafts.reserve()
        drafts.remove(key)
        var closed = false
        val src = object : UploadSource {
            private val inner = BytesSource(byteArrayOf(1))
            override val name = inner.name
            override val mime = inner.mime
            override val size = inner.size
            override val sha256 = inner.sha256
            override fun open(offset: Long) = inner.open(offset)
            override fun close() { closed = true }
        }
        drafts.bind(key, src)
        assertTrue(closed)
        assertTrue(drafts.items.value.isEmpty())
        assertEquals("nothing was sent", 0, ch.sent.size)
    }

    @Test fun removingAnUnknownKeyIsHarmless() {
        drafts.remove("nope")
        assertNull(drafts.items.value.firstOrNull())
    }

    private class Tracked(val inner: BytesSource) : UploadSource by inner {
        var closed = 0
        override fun close() { closed++ }
    }

    private fun begins() = ch.sent.filter { it.type == Protocol.ATTACH_BEGIN }.map { Protocol.json.decodeFromJsonElement(tech.hybriq.adbuddy.bridge.AttachBegin.serializer(), it.payload).id }

    @Test fun removeAndClearCloseTheSource() {
        ch.handler = { _, _ -> true } // the host never answers: the uploads stay in flight
        val a = Tracked(BytesSource(byteArrayOf(1)))
        val b = Tracked(BytesSource(byteArrayOf(2)))
        val c = Tracked(BytesSource(byteArrayOf(3)))
        val ka = drafts.add(a)
        drafts.add(b)
        drafts.add(c)
        drafts.remove(ka)
        assertEquals(1, a.closed)
        drafts.clear()
        assertEquals(1, b.closed)
        assertEquals(1, c.closed)
    }

    @Test fun removeRightAfterAddClosesTheSourceAndSendsNothing() {
        val queue = ArrayDeque<Runnable>()
        val lazy = object : kotlinx.coroutines.CoroutineDispatcher() {
            override fun dispatch(context: kotlin.coroutines.CoroutineContext, block: Runnable) { queue += block }
        }
        val d = Drafts(CoroutineScope(lazy), Uploader(ch, pause = {}, newId = { id }), Captures(ch)) { "k${++n}" }
        val src = Tracked(BytesSource(byteArrayOf(1)))
        d.remove(d.add(src))
        while (queue.isNotEmpty()) queue.removeFirst().run()
        assertEquals(1, src.closed)
        assertTrue(ch.sent.isEmpty())
        assertTrue(d.items.value.isEmpty())
    }

    @Test fun twoRetriesInARowStartOneUpload() {
        ch.handler = { s, cb -> if (s.type == Protocol.ATTACH_BEGIN) cb(FakeChannel.reply(Protocol.ERROR, """{"code":"failed","text":"x"}""", "r")); true }
        val key = drafts.add(BytesSource(byteArrayOf(1)))
        assertEquals(DraftStatus.FAILED, drafts.items.value.single().status)
        ch.handler = { _, _ -> true } // the retry stays in flight
        drafts.retry(key)
        drafts.retry(key)
        assertEquals(2, begins().size)
        assertEquals(DraftStatus.UPLOADING, drafts.items.value.single().status)
    }

    @Test fun aRetryKeepsTheUploadId() {
        var k = 0
        val d = Drafts(CoroutineScope(Dispatchers.Unconfined), Uploader(ch, pause = {}, newId = { "%032x".format(++k) }), Captures(ch)) { "k${++n}" }
        ch.handler = { s, cb -> if (s.type == Protocol.ATTACH_BEGIN) cb(FakeChannel.reply(Protocol.ERROR, """{"code":"failed","text":"x"}""", "r")); true }
        val key = d.add(BytesSource(byteArrayOf(1)))
        d.retry(key)
        val ids = begins()
        assertEquals(2, ids.size)
        assertEquals(ids[0], ids[1])
    }

    @Test fun retryOfAFailedCaptureTakesAnotherOne() {
        ch.handler = { s, cb -> if (s.type == Protocol.CAPTURE_REQUEST) cb(FakeChannel.reply(Protocol.ERROR, """{"code":"failed","text":"Cut"}""", "r")); true }
        drafts.capture()
        val key = drafts.items.value.single().key
        assertEquals(DraftStatus.FAILED, drafts.items.value.single().status)
        hostOk()
        drafts.retry(key)
        assertEquals(DraftStatus.DONE, drafts.items.value.single().status)
        assertEquals(2, ch.sent.count { it.type == Protocol.CAPTURE_REQUEST })
    }
}
