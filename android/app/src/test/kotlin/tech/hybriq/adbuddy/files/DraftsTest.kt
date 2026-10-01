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

    @Test fun removingAnUnknownKeyIsHarmless() {
        drafts.remove("nope")
        assertNull(drafts.items.value.firstOrNull())
    }
}
