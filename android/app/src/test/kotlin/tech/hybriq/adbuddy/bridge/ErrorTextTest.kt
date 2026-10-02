package tech.hybriq.adbuddy.bridge

import java.lang.reflect.Modifier
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.R

class ErrorTextTest {
    private val str: (Int) -> String = { "s$it" }

    private fun codes() = ErrorCode::class.java.declaredFields
        .filter { Modifier.isStatic(it.modifiers) && it.type == String::class.java }
        .map { it.get(null) as String }

    @Test fun everyCodeOfTheProtocolHasItsOwnString() {
        val codes = codes()
        assertTrue("ErrorCode changed shape: $codes", codes.size >= 12)
        val ids = codes.map { ErrorText.resFor(it) }
        assertTrue("a code with no string of its own: $codes", ids.none { it == R.string.error_unknown })
        assertEquals("two codes share a string", ids.size, ids.toSet().size)
    }

    @Test fun anUnknownCodeIsErrorUnknown() {
        assertEquals(R.string.error_unknown, ErrorText.resFor("a_code_of_the_future"))
        assertEquals(R.string.error_unknown, ErrorText.resFor(""))
    }

    @Test fun theHostTextIsShownForTheCodesWhoseCauseItExplains() {
        assertEquals("s${R.string.error_failed} (disk full)", ErrorText.text(str, ErrorReply(ErrorCode.FAILED, "disk full")))
        assertEquals("s${R.string.error_unknown} (new thing)", ErrorText.text(str, ErrorReply("new_code", "new thing")))
        assertEquals("s${R.string.error_busy} (Answer the pending question first.)", ErrorText.text(str, ErrorReply(ErrorCode.BUSY, "Answer the pending question first.")))
        assertEquals("s${R.string.error_unavailable} (Turn on Tailscale on the phone first)", ErrorText.text(str, ErrorReply(ErrorCode.UNAVAILABLE, "Turn on Tailscale on the phone first")))
        assertEquals("s${R.string.error_too_large} (The inbox is full (2 GiB))", ErrorText.text(str, ErrorReply(ErrorCode.TOO_LARGE, "The inbox is full (2 GiB)")))
    }

    @Test fun theCodesThatExplainThemselvesIgnoreTheHostText() {
        assertEquals("s${R.string.error_unsupported}", ErrorText.text(str, ErrorReply(ErrorCode.UNSUPPORTED, "x")))
        assertEquals("s${R.string.error_checksum}", ErrorText.text(str, ErrorReply(ErrorCode.CHECKSUM, "x")))
        assertEquals("s${R.string.error_no_agent}", ErrorText.text(str, ErrorReply(ErrorCode.NO_AGENT, "x")))
    }

    @Test fun anUnknownCodeWithNoTextShowsItsCodeCut() {
        assertEquals("s${R.string.error_unknown} (new_code)", ErrorText.text(str, ErrorReply("new_code", "")))
        assertEquals("s${R.string.error_unknown} (${"c".repeat(64)})", ErrorText.text(str, ErrorReply("c".repeat(100), "  ")))
        assertEquals("s${R.string.error_unknown}", ErrorText.text(str, ErrorReply("", "")))
    }

    @Test fun theCutNeverSplitsASurrogatePair() {
        val emoji = "😀"
        val out = ErrorText.text(str, ErrorReply(ErrorCode.FAILED, "x".repeat(199) + emoji + "tail"))
        assertEquals("s${R.string.error_failed} (${"x".repeat(199)})", out)
        val fits = ErrorText.text(str, ErrorReply(ErrorCode.FAILED, "x".repeat(198) + emoji))
        assertEquals("s${R.string.error_failed} (${"x".repeat(198)}$emoji)", fits)
    }

    @Test fun anEmptyHostTextAddsNothingAndALongOneIsCut() {
        assertEquals("s${R.string.error_failed}", ErrorText.text(str, ErrorReply(ErrorCode.FAILED, "")))
        assertEquals("s${R.string.error_failed}", ErrorText.text(str, ErrorReply(ErrorCode.FAILED, "   ")))
        val long = ErrorText.text(str, ErrorReply(ErrorCode.FAILED, "x".repeat(500)))
        assertEquals("s${R.string.error_failed} (${"x".repeat(200)})", long)
    }

    @Test fun theControllersDefaultsKeepTheirOldReading() {
        assertEquals("boom", ErrorText.plain(ErrorReply("failed", "boom")))
        assertEquals("failed", ErrorText.plain(ErrorReply("failed", "")))
        assertEquals("", ErrorText.hostText(ErrorReply("failed", "")))
        assertNotEquals("", ErrorText.hostText(ErrorReply("failed", "boom")))
    }
}
