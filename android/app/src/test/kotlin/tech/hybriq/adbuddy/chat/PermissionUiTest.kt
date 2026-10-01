package tech.hybriq.adbuddy.chat

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.PermissionRequest

class PermissionUiTest {
    private val t0 = java.time.Instant.parse("2026-10-01T10:00:00Z").toEpochMilli()

    private fun p(always: Boolean = true, expires: String = "2026-10-01T10:10:00Z") =
        PermissionRequest("p1", "Bash", "touch x", "touch x", expires, always)

    @Test fun secondsLeftCountDownAndStopAtZero() {
        assertEquals(600L, PermissionUi.secondsLeft(p(), t0))
        assertEquals(1L, PermissionUi.secondsLeft(p(), t0 + 599_001))
        assertEquals(0L, PermissionUi.secondsLeft(p(), t0 + 600_000))
        assertEquals(0L, PermissionUi.secondsLeft(p(), t0 + 9_999_999))
    }

    @Test fun anUnreadableLimitNeverExpiresHere() {
        val bad = p(expires = "mañana")
        assertNull(PermissionUi.secondsLeft(bad, t0))
        assertTrue(PermissionUi.canAnswer(bad, PermissionUi.ALLOW, true, t0 + 999_999_999))
        assertNull(PermissionUi.nextTickMs(listOf(bad), t0))
    }

    @Test fun answeringNeedsTheHostTheTimeAndAKnownDecision() {
        assertTrue(PermissionUi.canAnswer(p(), PermissionUi.ALLOW, true, t0))
        assertTrue(PermissionUi.canAnswer(p(), PermissionUi.DENY, true, t0))
        assertFalse("not connected", PermissionUi.canAnswer(p(), PermissionUi.ALLOW, false, t0))
        assertFalse("expired", PermissionUi.canAnswer(p(), PermissionUi.ALLOW, true, t0 + 600_000))
        assertFalse("made up", PermissionUi.canAnswer(p(), "maybe", true, t0))
        assertFalse("", PermissionUi.canAnswer(p(), "", true, t0))
    }

    @Test fun alwaysOnlyWhenTheHostOffersIt() {
        assertTrue(PermissionUi.canAnswer(p(always = true), PermissionUi.ALLOW_ALWAYS, true, t0))
        assertFalse(PermissionUi.canAnswer(p(always = false), PermissionUi.ALLOW_ALWAYS, true, t0))
        assertTrue(PermissionUi.canAnswer(p(always = false), PermissionUi.ALLOW, true, t0))
    }

    @Test fun theClockTicksOnlyWhileACardCountsDown() {
        assertEquals(1000L, PermissionUi.nextTickMs(listOf(p()), t0))
        assertNull(PermissionUi.nextTickMs(emptyList(), t0))
        assertNull(PermissionUi.nextTickMs(listOf(p()), t0 + 600_000))
    }

    @Test fun theReplyCarriesTheIdAndTheDecision() {
        val r = PermissionUi.reply(p(), PermissionUi.DENY)
        assertEquals("p1", r.id)
        assertEquals("deny", r.decision)
    }
}
