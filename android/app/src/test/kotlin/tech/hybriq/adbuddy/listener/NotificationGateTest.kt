package tech.hybriq.adbuddy.listener

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.FakePrefs
import tech.hybriq.adbuddy.listener.NotificationAccess.Listed

class NotificationGateTest {
    private val one = RawNotification("k", "com.whatsapp", "Ana", "hola", 0L, ongoing = false, secret = false)
    private var sourceCalls = 0
    private var revoked = 0

    private fun gate(confirmed: Boolean, listed: Listed?, source: List<RawNotification>? = listOf(one)) =
        NotificationGate.read({ confirmed }, { listed }, { revoked++ }, { sourceCalls++; source })

    @Test fun withoutTheUsersConfirmationItNeverReadsEvenIfTheSystemSaysAllowed() {
        assertEquals(NotificationRead.NotConfirmed, gate(confirmed = false, listed = Listed.YES))
        assertEquals(NotificationRead.NotConfirmed, gate(confirmed = false, listed = Listed.UNKNOWN))
        assertEquals(NotificationRead.NotConfirmed, gate(confirmed = false, listed = null))
        assertEquals("the listener is not even asked", 0, sourceCalls)
    }

    @Test fun confirmedAndAllowedReads() {
        assertEquals(NotificationRead.Ok(listOf(one)), gate(confirmed = true, listed = Listed.YES))
        assertEquals(1, sourceCalls)
    }

    @Test fun aRevokedAccessIsNoAccessAndInvalidatesTheMark() {
        assertEquals(NotificationRead.NoAccess, gate(confirmed = true, listed = Listed.NO))
        assertEquals(1, revoked)
        assertEquals(NotificationRead.NoAccess, gate(confirmed = false, listed = Listed.NO))
        assertEquals(0, sourceCalls)
    }

    @Test fun whatCannotBeToldIsNoAccessAndKeepsTheMark() {
        assertEquals(NotificationRead.NoAccess, gate(confirmed = true, listed = Listed.UNKNOWN))
        assertEquals(NotificationRead.NoAccess, gate(confirmed = true, listed = null))
        assertEquals(0, revoked)
        assertEquals(0, sourceCalls)
    }

    @Test fun anUnboundListenerIsNoAccess() {
        assertEquals(NotificationRead.NoAccess, gate(confirmed = true, listed = Listed.YES, source = null))
        assertEquals(0, revoked)
    }

    @Test fun theMarkPersistsAndRevokingTheAccessNeedsANewConfirmation() {
        val prefs = FakePrefs()
        assertFalse(NotificationConsent(prefs).confirmed())
        NotificationConsent(prefs).confirm() // the button of the screen
        val consent = NotificationConsent(prefs) // a new process reads it from the same preferences
        assertTrue(consent.confirmed())
        fun read(listed: Listed) = NotificationGate.read(consent::confirmed, { listed }, consent::revoke) { listOf(one) }
        assertEquals(NotificationRead.Ok(listOf(one)), read(Listed.YES))
        assertEquals(NotificationRead.NoAccess, read(Listed.NO)) // the user revokes in the system
        assertFalse(consent.confirmed())
        assertEquals("allowing again is not enough", NotificationRead.NotConfirmed, read(Listed.YES))
        consent.confirm()
        assertEquals(NotificationRead.Ok(listOf(one)), read(Listed.YES))
    }

    @Test fun unbindingTheListenerClearsTheMarkAtOnceWhoeverIsLooking() {
        val prefs = FakePrefs()
        val consent = NotificationConsent(prefs)
        consent.confirm()
        NotificationGate.onUnbound(consent::revoke) // the system revoked the access: no screen is open, no read is asked
        assertFalse(NotificationConsent(prefs).confirmed())
        // The agent turns the switch back on: still not enough.
        assertEquals(NotificationRead.NotConfirmed, NotificationGate.read(consent::confirmed, { Listed.YES }, consent::revoke) { listOf(one) })
        assertEquals("nothing was read", 0, sourceCalls)
        NotificationGate.onUnbound(consent::revoke) // already clear: nothing breaks
        assertFalse(consent.confirmed())
    }

    // Accepted risk (ruling 15): this looks for `.confirm()` and `::confirm`, so `with(consent) { confirm() }`
    // gets round it. The Opus review reads the screen by hand; the test catches the plain mistakes.
    @Test fun nothingButTheButtonSetsTheMark() {
        // The gate and the status never confirm: confirm() is called from the confirm button only.
        val calls = java.io.File("src/main/kotlin").walkTopDown().filter { it.isFile && it.extension == "kt" }
            .filter { Regex("\\.confirm\\(\\)|::confirm\\b").containsMatchIn(it.readText()) }.map { it.name }.toSet()
        assertTrue(calls.toString(), setOf("NotificationAccessScreen.kt", "NotificationConsent.kt").containsAll(calls))
    }
}
