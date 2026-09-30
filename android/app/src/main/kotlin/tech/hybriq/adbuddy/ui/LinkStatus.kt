package tech.hybriq.adbuddy.ui

import androidx.annotation.StringRes
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState

/**
 * The state line of the home screen and of the service notification (spec §6).
 * Pure, so it is unit-tested; both places read it from here so they never
 * disagree.
 */
enum class LinkStatus(@param:StringRes val text: Int) {
    CUT(R.string.status_cut),
    UPDATE_APP(R.string.status_update_app),
    UPDATE_HOST(R.string.status_update_host),
    PAUSED(R.string.status_paused),
    CONNECTED(R.string.status_connected),
    CONNECTING(R.string.status_connecting),
    WAITING(R.string.status_waiting),
    ;

    companion object {
        /**
         * cut: adb access is off on this phone (Cut access); it says so over
         * anything else, since the PC cannot connect while it lasts. Paused
         * only while connected: the pause of a lost link is not news.
         */
        fun of(state: BridgeState, cut: Boolean): LinkStatus = when {
            cut -> CUT
            state.connection == BridgeState.Connection.INCOMPATIBLE && state.outdated == BridgeState.Outdated.APP -> UPDATE_APP
            state.connection == BridgeState.Connection.INCOMPATIBLE -> UPDATE_HOST
            state.connection == BridgeState.Connection.CONNECTED && state.sessionState.paused -> PAUSED
            state.connection == BridgeState.Connection.CONNECTED -> CONNECTED
            state.connection == BridgeState.Connection.CONNECTING -> CONNECTING
            else -> WAITING
        }
    }
}

/** Pause or Resume: offered only while the host is connected, after what the host says (session.state). */
enum class PauseButton(@param:StringRes val text: Int) {
    PAUSE(R.string.action_pause),
    RESUME(R.string.action_resume),
    ;

    companion object {
        fun of(state: BridgeState): PauseButton? = when {
            state.connection != BridgeState.Connection.CONNECTED -> null
            state.sessionState.paused -> RESUME
            else -> PAUSE
        }
    }
}

/** What the service notification shows: it is posted again only when this changes. */
data class StatusView(val status: LinkStatus, val host: String, val button: PauseButton?) {
    companion object {
        fun of(state: BridgeState, cut: Boolean): StatusView {
            val status = LinkStatus.of(state, cut)
            return StatusView(status, if (status == LinkStatus.CONNECTED) state.hostName else "", PauseButton.of(state))
        }
    }
}
