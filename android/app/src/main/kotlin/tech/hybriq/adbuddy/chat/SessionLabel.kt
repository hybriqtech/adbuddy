package tech.hybriq.adbuddy.chat

import tech.hybriq.adbuddy.bridge.BridgeState

/** What the header says the agent is doing: only while the host is connected. */
enum class SessionLabel {
    OFFLINE, WORKING, WAITING, IDLE, STOPPED;

    companion object {
        fun of(state: BridgeState): SessionLabel = when {
            !state.isConnected -> OFFLINE
            else -> when (state.sessionState.state) {
                "working" -> WORKING
                "waiting" -> WAITING
                "stopped" -> STOPPED
                else -> IDLE
            }
        }
    }
}
