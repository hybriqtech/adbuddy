package tech.hybriq.adbuddy.chat

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.files.Draft
import tech.hybriq.adbuddy.files.DraftStatus

class ComposerTest {
    private fun d(st: DraftStatus) = Draft("k", "a", "image/png", 1, 1, st, Attachment("0".repeat(32), "a", "image/png", 1, "x"))

    @Test fun needsTextOrAnAttachment() {
        assertFalse(Composer.canSend("", emptyList()))
        assertFalse(Composer.canSend("  \n ", emptyList()))
        assertTrue(Composer.canSend("hi", emptyList()))
        assertTrue(Composer.canSend("", listOf(d(DraftStatus.DONE))))
    }

    @Test fun waitsForEveryUploadAndRefusesAFailedOne() {
        assertFalse(Composer.canSend("hi", listOf(d(DraftStatus.DONE), d(DraftStatus.UPLOADING))))
        assertFalse(Composer.canSend("hi", listOf(d(DraftStatus.FAILED))))
    }

    @Test fun respectsTheProtocolLimits() {
        assertTrue(Composer.canSend("a".repeat(Composer.MAX_TEXT_BYTES), emptyList()))
        assertFalse(Composer.canSend("a".repeat(Composer.MAX_TEXT_BYTES + 1), emptyList()))
        assertFalse("é counts as 2 bytes", Composer.canSend("é".repeat(Composer.MAX_TEXT_BYTES / 2 + 1), emptyList()))
        assertTrue(Composer.canSend("", List(Composer.MAX_ATTACHMENTS) { d(DraftStatus.DONE) }))
        assertFalse(Composer.canSend("", List(Composer.MAX_ATTACHMENTS + 1) { d(DraftStatus.DONE) }))
        assertEquals(10, Composer.MAX_ATTACHMENTS)
    }
}
