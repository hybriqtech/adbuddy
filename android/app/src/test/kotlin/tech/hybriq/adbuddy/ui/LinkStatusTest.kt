package tech.hybriq.adbuddy.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Test
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.JournalEntry
import tech.hybriq.adbuddy.bridge.SessionState

class LinkStatusTest {
    private val paused = SessionState(paused = true)
    private fun s(c: BridgeState.Connection, p: Boolean = false, outdated: BridgeState.Outdated = BridgeState.Outdated.NONE) =
        BridgeState(connection = c, hostName = "pc", sessionState = SessionState(paused = p), outdated = outdated)

    @Test fun pausedOnlyWhileConnected() {
        assertEquals(LinkStatus.PAUSED, LinkStatus.of(s(BridgeState.Connection.CONNECTED, p = true), cut = false))
        assertEquals("the pause of a lost link is not news", LinkStatus.WAITING, LinkStatus.of(s(BridgeState.Connection.WAITING, p = true), cut = false))
        assertEquals(LinkStatus.CONNECTING, LinkStatus.of(s(BridgeState.Connection.CONNECTING, p = true), cut = false))
    }

    @Test fun eachConnectionHasItsText() {
        assertEquals(LinkStatus.CONNECTED, LinkStatus.of(s(BridgeState.Connection.CONNECTED), cut = false))
        assertEquals(LinkStatus.CONNECTING, LinkStatus.of(s(BridgeState.Connection.CONNECTING), cut = false))
        assertEquals(LinkStatus.WAITING, LinkStatus.of(s(BridgeState.Connection.WAITING), cut = false))
        assertEquals(LinkStatus.UPDATE_APP, LinkStatus.of(s(BridgeState.Connection.INCOMPATIBLE, outdated = BridgeState.Outdated.APP), cut = false))
        assertEquals(LinkStatus.UPDATE_HOST, LinkStatus.of(s(BridgeState.Connection.INCOMPATIBLE, outdated = BridgeState.Outdated.HOST), cut = false))
    }

    @Test fun aCutAccessSaysSoWhateverTheConnection() {
        for (c in BridgeState.Connection.entries) {
            assertEquals(c.name, LinkStatus.CUT, LinkStatus.of(s(c, p = true), cut = true))
        }
    }

    @Test fun pauseAndResumeOnlyWhileConnected() {
        assertEquals(PauseButton.PAUSE, PauseButton.of(s(BridgeState.Connection.CONNECTED)))
        assertEquals(PauseButton.RESUME, PauseButton.of(s(BridgeState.Connection.CONNECTED, p = true)))
        assertNull(PauseButton.of(s(BridgeState.Connection.WAITING, p = true)))
        assertNull(PauseButton.of(s(BridgeState.Connection.CONNECTING)))
    }

    @Test fun theStatusNotificationChangesOnlyWithWhatItShows() {
        val a = s(BridgeState.Connection.CONNECTED)
        val b = a.copy(lastSeq = 9, journal = listOf(JournalEntry(id = "j_1")))
        assertEquals(StatusView.of(a, cut = false), StatusView.of(b, cut = false))
        assertNotEquals(StatusView.of(a, cut = false), StatusView.of(a.copy(sessionState = paused), cut = false))
        assertNotEquals(StatusView.of(a, cut = false), StatusView.of(a.copy(hostName = "other"), cut = false))
        assertNotEquals(StatusView.of(a, cut = false), StatusView.of(a, cut = true))
    }
}
