package tech.hybriq.adbuddy.chat

import tech.hybriq.adbuddy.files.Draft
import tech.hybriq.adbuddy.files.DraftStatus

/** When the Send button works (protocol spec §9 limits; spec M3 §6.4: sending waits for every upload). */
object Composer {
    const val MAX_TEXT_BYTES = 64 * 1024
    const val MAX_ATTACHMENTS = 10

    fun canSend(text: String, drafts: List<Draft>): Boolean {
        if (drafts.size > MAX_ATTACHMENTS || drafts.any { it.status != DraftStatus.DONE }) return false
        if (text.toByteArray().size > MAX_TEXT_BYTES) return false
        return text.isNotBlank() || drafts.isNotEmpty()
    }
}
