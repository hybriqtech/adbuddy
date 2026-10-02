package tech.hybriq.adbuddy.files

import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.CaptureReady
import tech.hybriq.adbuddy.bridge.Channel
import tech.hybriq.adbuddy.bridge.Empty
import tech.hybriq.adbuddy.bridge.ErrorReply
import tech.hybriq.adbuddy.bridge.ErrorText
import tech.hybriq.adbuddy.bridge.ErrorTexts
import tech.hybriq.adbuddy.bridge.Protocol

/** Asks the host for a screenshot of this phone (protocol spec §6.3); it comes back as an attachment already in inbox/. */
class Captures(
    private val channel: Channel,
    /** How the host refusals read on screen (ErrorText). */
    private val errorText: ErrorTexts = ErrorText.plain,
) {
    /**
     * onResult(attachment, null) on success; onResult(null, text) when the host
     * refused (its own text, maybe empty); onResult(null, null) when the
     * connection was lost. It runs once, on whichever thread answers.
     */
    fun take(onResult: (Attachment?, String?) -> Unit) {
        val rid = channel.request(Protocol.CAPTURE_REQUEST, Protocol.json.encodeToJsonElement(Empty.serializer(), Empty)) { m ->
            when {
                m == null -> onResult(null, null)
                m.type == Protocol.ERROR -> onResult(null, Protocol.payload<ErrorReply>(m)?.let(errorText) ?: "")
                else -> {
                    val a = if (m.type == Protocol.CAPTURE_READY) Protocol.payload<CaptureReady>(m)?.attachment else null
                    if (a != null) onResult(a, null) else onResult(null, "")
                }
            }
        }
        if (rid == null) onResult(null, null)
    }
}
