package tech.hybriq.adbuddy.chat

import tech.hybriq.adbuddy.bridge.ChatState
import tech.hybriq.adbuddy.bridge.ToolActivity

/**
 * The agent's live tool activity under the conversation (spec M4 §7.2): what it
 * is doing now and the last few things it did, only while it works. Pure.
 * The host sends no conversation with a tool.activity, so what a finished turn
 * left is not shown again: with the agent idle, waiting or stopped the strip
 * is empty.
 */
object ToolStrip {
    const val SHOWN = 5

    fun of(chat: ChatState, sessionState: String): List<ToolActivity> {
        // "waiting" with a tool still running is the agent held on a question of its own.
        val live = sessionState == "working" || (sessionState == "waiting" && chat.tools.any { it.status == "running" })
        if (!live) return emptyList()
        return chat.tools.takeLast(SHOWN)
    }
}
