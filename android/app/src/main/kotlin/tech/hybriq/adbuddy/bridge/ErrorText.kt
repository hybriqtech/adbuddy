package tech.hybriq.adbuddy.bridge

import androidx.annotation.StringRes
import tech.hybriq.adbuddy.R

/** What the app shows of an [ErrorReply]: the host's `text` is English by protocol and detail for the agent and the log. */
typealias ErrorTexts = (ErrorReply) -> String

/**
 * The text of an ErrorReply in the language of the phone, by its `code` (protocol spec §4.1 and §8.1; M6c ruling 4).
 * Every ErrorCode has its error_<code> string (StringsParityTest checks it), and a code this app does not know shows
 * error_unknown with the host's text after it.
 */
object ErrorText {
    /** The way the controllers read an error when they have no Context (the tests'): the host's text, else the code. */
    val plain: ErrorTexts = { it.text.ifEmpty { it.code } }

    /** Only what the host wrote, maybe empty: the uploads' (the screen words the empty case itself). */
    val hostText: ErrorTexts = { it.text }

    @StringRes
    fun resFor(code: String): Int = when (code) {
        ErrorCode.UNSUPPORTED -> R.string.error_unsupported
        ErrorCode.INVALID -> R.string.error_invalid
        ErrorCode.NOT_FOUND -> R.string.error_not_found
        ErrorCode.NO_AGENT -> R.string.error_no_agent
        ErrorCode.BUSY -> R.string.error_busy
        ErrorCode.TOO_LARGE -> R.string.error_too_large
        ErrorCode.CHECKSUM -> R.string.error_checksum
        ErrorCode.FAILED -> R.string.error_failed
        ErrorCode.UNAVAILABLE -> R.string.error_unavailable
        ErrorCode.PROTECTED -> R.string.error_protected
        ErrorCode.EXPIRED -> R.string.error_expired
        ErrorCode.AMBIGUOUS -> R.string.error_ambiguous
        else -> R.string.error_unknown
    }

    /** The text for [e], with [getString] resolving the string in the language in force. For `failed` and an unknown code the host's own text follows, in parentheses and at most 200 characters. */
    fun text(getString: (Int) -> String, e: ErrorReply): String {
        val own = getString(resFor(e.code))
        val detail = e.text.trim().take(200)
        val withDetail = e.code == ErrorCode.FAILED || resFor(e.code) == R.string.error_unknown
        return if (withDetail && detail.isNotEmpty()) "$own ($detail)" else own
    }
}
