package tech.hybriq.adbuddy.listener

import org.junit.Assert.assertEquals
import org.junit.Test
import tech.hybriq.adbuddy.listener.NotificationAccess.Listed
import tech.hybriq.adbuddy.listener.NotificationAccess.State

class NotificationAccessTest {
    private val ours = "${NotificationAccess.PACKAGE}/${NotificationAccess.CLASS}"

    @Test fun theNamesAreTheRealListener() {
        assertEquals(NotificationAccess.CLASS, AgentNotificationListener::class.java.name)
        assertEquals(NotificationAccess.PACKAGE, AgentNotificationListener::class.java.packageName.substringBeforeLast(".listener"))
    }

    @Test fun listedFindsOurComponentInTheSettingsList() {
        assertEquals(Listed.YES, NotificationAccess.listed(ours))
        assertEquals(Listed.YES, NotificationAccess.listed("com.a.b/com.a.b.L:$ours:com.c.d/.Other"))
        // ".Class" is the short form of package.Class.
        assertEquals(Listed.YES, NotificationAccess.listed("tech.hybriq.adbuddy/.listener.AgentNotificationListener"))
    }

    @Test fun listedSaysNoWhenOthersAreEnabledButNotOurs() {
        assertEquals(Listed.NO, NotificationAccess.listed("com.a.b/com.a.b.L:com.c.d/.Other"))
        assertEquals(Listed.NO, NotificationAccess.listed("tech.hybriq.adbuddy/tech.hybriq.adbuddy.Other"))
        assertEquals(Listed.NO, NotificationAccess.listed("tech.hybriq.adbuddyx/${NotificationAccess.CLASS}"))
    }

    @Test fun aMissingOrEmptySettingMeansNoListenerIsEnabled() {
        assertEquals(Listed.NO, NotificationAccess.listed(null))
        assertEquals(Listed.NO, NotificationAccess.listed(""))
        assertEquals(Listed.NO, NotificationAccess.listed("  "))
        assertEquals(Listed.NO, NotificationAccess.listed(":"))
    }

    @Test fun anEntryThatCannotBeReadIsUnknownUnlessOursIsThere() {
        assertEquals(Listed.UNKNOWN, NotificationAccess.listed("garbage"))
        assertEquals(Listed.UNKNOWN, NotificationAccess.listed("com.a.b/com.a.b.L:/x:y/"))
        assertEquals(Listed.UNKNOWN, NotificationAccess.listed("com.a.b/"))
        assertEquals(Listed.YES, NotificationAccess.listed("garbage:$ours"))
    }

    @Test fun theStateNeverPaintsUnknownAsGrantedOrDenied() {
        assertEquals(State.GRANTED, NotificationAccess.state(Listed.YES, connected = true))
        assertEquals(State.NOT_GRANTED, NotificationAccess.state(Listed.NO, connected = false))
        assertEquals(State.NOT_GRANTED, NotificationAccess.state(Listed.NO, connected = true)) // revoked, the binding not yet gone
        // Allowed in the setting but the listener is not bound: not granted-and-working, not denied.
        assertEquals(State.UNKNOWN, NotificationAccess.state(Listed.YES, connected = false))
        assertEquals(State.UNKNOWN, NotificationAccess.state(Listed.UNKNOWN, connected = true))
        assertEquals(State.UNKNOWN, NotificationAccess.state(Listed.UNKNOWN, connected = false))
        assertEquals(State.UNKNOWN, NotificationAccess.state(null, connected = true)) // the setting could not be read
        assertEquals(State.UNKNOWN, NotificationAccess.state(null, connected = false))
    }

    @Test fun theSystemsAnswerAndTheSettingAreCombinedAndNeverGuessed() {
        // The system's own answer first; the setting is the second source.
        assertEquals(Listed.YES, NotificationAccess.combine(true, Listed.YES))
        assertEquals(Listed.NO, NotificationAccess.combine(false, Listed.NO))
        assertEquals(Listed.YES, NotificationAccess.combine(true, null)) // the setting could not be read
        assertEquals(Listed.NO, NotificationAccess.combine(false, Listed.UNKNOWN))
        assertEquals(Listed.YES, NotificationAccess.combine(null, Listed.YES)) // the system's answer failed
        assertEquals(Listed.NO, NotificationAccess.combine(null, Listed.NO))
        // They disagree: not an answer.
        assertEquals(Listed.UNKNOWN, NotificationAccess.combine(true, Listed.NO))
        assertEquals(Listed.UNKNOWN, NotificationAccess.combine(false, Listed.YES))
        // Both failed: unknown, never granted nor denied.
        assertEquals(null, NotificationAccess.combine(null, null))
        assertEquals(State.UNKNOWN, NotificationAccess.state(NotificationAccess.combine(null, null), connected = true))
        assertEquals(State.UNKNOWN, NotificationAccess.state(NotificationAccess.combine(true, Listed.NO), connected = true))
    }

    @Test fun allowedInTheSystemIsNotEnoughUntilTheUserConfirms() {
        val e = NotificationAccess.Effective.entries
        assertEquals(NotificationAccess.Effective.ON, NotificationAccess.effective(State.GRANTED, confirmed = true))
        assertEquals(NotificationAccess.Effective.NEEDS_CONFIRMATION, NotificationAccess.effective(State.GRANTED, confirmed = false))
        for (confirmed in listOf(true, false)) {
            assertEquals(NotificationAccess.Effective.OFF, NotificationAccess.effective(State.NOT_GRANTED, confirmed))
            assertEquals(NotificationAccess.Effective.UNKNOWN, NotificationAccess.effective(State.UNKNOWN, confirmed))
        }
        assertEquals(4, e.size)
    }
}
