package tech.hybriq.adbuddy.chat

import org.junit.Assert.assertEquals
import org.junit.Test
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.SessionState

class SessionLabelTest {
    private fun s(c: BridgeState.Connection, st: String) = BridgeState(connection = c, sessionState = SessionState(state = st))

    @Test fun theStateOnlyCountsWhileConnected() {
        assertEquals(SessionLabel.OFFLINE, SessionLabel.of(s(BridgeState.Connection.WAITING, "working")))
        assertEquals(SessionLabel.WORKING, SessionLabel.of(s(BridgeState.Connection.CONNECTED, "working")))
        assertEquals(SessionLabel.WAITING, SessionLabel.of(s(BridgeState.Connection.CONNECTED, "waiting")))
        assertEquals(SessionLabel.IDLE, SessionLabel.of(s(BridgeState.Connection.CONNECTED, "idle")))
        assertEquals(SessionLabel.STOPPED, SessionLabel.of(s(BridgeState.Connection.CONNECTED, "stopped")))
        assertEquals("an unknown state is not an alarm", SessionLabel.IDLE, SessionLabel.of(s(BridgeState.Connection.CONNECTED, "???")))
    }
}
