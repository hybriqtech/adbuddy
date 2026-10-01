package tech.hybriq.adbuddy.ui

import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.BridgeState.Connection
import tech.hybriq.adbuddy.bridge.RemoteState
import tech.hybriq.adbuddy.ui.RemoteUi.Kind

class RemoteUiTest {
    private val now = Instant.parse("2026-10-01T20:00:00Z")
    private val future = "2026-10-01T21:14:00Z"
    private val past = "2026-10-01T19:00:00Z"

    private fun state(remote: RemoteState?, connection: Connection = Connection.CONNECTED) =
        BridgeState(connection = connection, remote = remote)

    private fun view(s: BridgeState, pending: Boolean = false, unknownSince: Boolean = false) =
        RemoteUi.view(s, pending, unknownSince, now)

    @Test fun `not connected to the host is DISCONNECTED and cannot toggle`() {
        for (c in listOf(Connection.WAITING, Connection.CONNECTING, Connection.INCOMPATIBLE)) {
            val v = view(state(RemoteState(enabled = true, expiresAt = future), c))
            assertEquals(Kind.DISCONNECTED, v.kind)
            assertFalse(v.canToggle)
        }
    }

    @Test fun `connected with an unknown remote is UNKNOWN and cannot toggle`() {
        val v = view(state(null))
        assertEquals(Kind.UNKNOWN, v.kind)
        assertFalse(v.canToggle)
    }

    @Test fun `disabled is OFF and can toggle`() {
        val v = view(state(RemoteState(enabled = false)))
        assertEquals(Kind.OFF, v.kind)
        assertTrue(v.canToggle)
    }

    @Test fun `enabled with a future expiry is ON with address and expiry`() {
        val v = view(state(RemoteState(enabled = true, address = "100.64.0.5:5555", expiresAt = future)))
        assertEquals(Kind.ON, v.kind)
        assertEquals("100.64.0.5:5555", v.address)
        assertEquals(Instant.parse(future), v.expiresAt)
        assertTrue(v.canToggle)
    }

    @Test fun `enabled with a past expiry is CLOSING never OFF`() {
        val v = view(state(RemoteState(enabled = true, address = "100.64.0.5:5555", expiresAt = past)))
        assertEquals(Kind.CLOSING, v.kind)
        assertTrue(v.canToggle)
    }

    @Test fun `expiry exactly now is CLOSING`() {
        assertEquals(Kind.CLOSING, view(state(RemoteState(enabled = true, expiresAt = now.toString()))).kind)
    }

    @Test fun `unknownSince shows UNKNOWN even if remote has a value`() {
        for (r in listOf(RemoteState(enabled = false), RemoteState(enabled = true, expiresAt = future))) {
            val v = view(state(r), unknownSince = true)
            assertEquals(Kind.UNKNOWN, v.kind)
            assertFalse(v.canToggle)
        }
    }

    @Test fun `pending is PENDING and cannot toggle`() {
        val v = view(state(RemoteState(enabled = false)), pending = true)
        assertEquals(Kind.PENDING, v.kind)
        assertFalse(v.canToggle)
    }

    @Test fun `an unreadable expiry is ON without expiry and does not throw`() {
        for (bad in listOf("", "soon", "2026-13-45")) {
            val v = view(state(RemoteState(enabled = true, address = "100.64.0.5:5555", expiresAt = bad)))
            assertEquals(Kind.ON, v.kind)
            assertNull(v.expiresAt)
        }
    }

    @Test fun `request carries only enabled`() {
        assertEquals("""{"enabled":true}""", RemoteUi.request(true).toString())
        assertEquals("""{"enabled":false}""", RemoteUi.request(false).toString())
    }
}
