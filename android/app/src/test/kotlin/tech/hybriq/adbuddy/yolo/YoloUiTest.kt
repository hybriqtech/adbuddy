package tech.hybriq.adbuddy.yolo

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.YoloState
import tech.hybriq.adbuddy.confirm.ConfirmUi
import tech.hybriq.adbuddy.yolo.YoloUi.Choice
import tech.hybriq.adbuddy.yolo.YoloUi.Kind

class YoloUiTest {
    private fun state(y: YoloState?, connected: Boolean = true) =
        BridgeState(yolo = y, connection = if (connected) BridgeState.Connection.CONNECTED else BridgeState.Connection.WAITING)

    @Test fun unknownOffTimedAndSession() {
        assertEquals(YoloUi.View(Kind.UNKNOWN), YoloUi.view(state(null), 1000))
        assertEquals(YoloUi.View(Kind.OFF), YoloUi.view(state(YoloState()), 1000))
        assertEquals(YoloUi.View(Kind.TIMED, 4000), YoloUi.view(state(YoloState(true, 5000, false)), 1000))
        assertEquals(YoloUi.View(Kind.SESSION), YoloUi.view(state(YoloState(true, 0, true)), 1000))
    }

    // Past its expiry and still "active" for the app means the host has not said it is off yet: the band says 0:00, never off.
    // Mutation: return OFF when msLeft <= 0.
    @Test fun anExpiryThatHasPassedStaysOnUntilTheHostSaysOtherwise() {
        val v = YoloUi.view(state(YoloState(true, 5000, false)), 9000)
        assertEquals(Kind.TIMED, v.kind)
        assertEquals(0L, v.msLeft)
        assertTrue(v.active)
    }

    @Test fun theBandOutlivesTheConnection() {
        assertTrue(YoloUi.view(state(YoloState(true, 0, true), connected = false), 0).active)
    }

    @Test fun countdownRoundsUpToWholeSeconds() {
        assertEquals("0:00", YoloUi.countdown(0))
        assertEquals("0:01", YoloUi.countdown(1))
        assertEquals("0:59", YoloUi.countdown(59_000))
        assertEquals("1:01", YoloUi.countdown(61_000))
        assertEquals("15:00", YoloUi.countdown(900_000))
        assertEquals("59:59", YoloUi.countdown(3_599_000))
        assertEquals("1:00:00", YoloUi.countdown(3_599_001))
        assertEquals("1:00:00", YoloUi.countdown(3_600_000))
    }

    @Test fun theRequestsAreTheOnesTheHostAccepts() {
        assertEquals("""{"minutes":15,"session":false,"off":false}""", YoloUi.request(Choice.QUARTER).toString())
        assertEquals("""{"minutes":60,"session":false,"off":false}""", YoloUi.request(Choice.HOUR).toString())
        assertEquals("""{"minutes":0,"session":true,"off":false}""", YoloUi.request(Choice.SESSION).toString())
        assertEquals("""{"minutes":0,"session":false,"off":true}""", YoloUi.offRequest().toString())
    }

    @Test fun onlyAKnownOffWithThePcConnectedCanBeTurnedOn() {
        assertTrue(YoloUi.canTurnOn(YoloUi.View(Kind.OFF), connected = true))
        assertFalse(YoloUi.canTurnOn(YoloUi.View(Kind.OFF), connected = false))
        assertFalse(YoloUi.canTurnOn(YoloUi.View(Kind.UNKNOWN), connected = true))
        assertFalse(YoloUi.canTurnOn(YoloUi.View(Kind.TIMED, 1), connected = true))
        assertFalse(YoloUi.canTurnOn(YoloUi.View(Kind.SESSION), connected = true))
    }

    // Each condition of the confirmation, taken away alone. Mutation: drop any one of them from mayConfirm.
    @Test fun turningItOnNeedsAChoiceTheConnectionTheUnlockAndTheArmingWait() {
        val shown = 10_000L
        val armed = shown + ConfirmUi.ARMING_MS
        assertTrue(YoloUi.mayConfirm(Choice.QUARTER, shown, armed, locked = false, connected = true))
        assertFalse("no choice", YoloUi.mayConfirm(null, shown, armed, locked = false, connected = true))
        assertFalse("not armed yet", YoloUi.mayConfirm(Choice.QUARTER, shown, armed - 1, locked = false, connected = true))
        assertFalse("never shown", YoloUi.mayConfirm(Choice.QUARTER, null, armed, locked = false, connected = true))
        assertFalse("locked", YoloUi.mayConfirm(Choice.QUARTER, shown, armed, locked = true, connected = true))
        assertFalse("not connected", YoloUi.mayConfirm(Choice.QUARTER, shown, armed, locked = false, connected = false))
    }

    @Test fun everyChoiceIsOneOfTheThreeTheHostAccepts() {
        assertEquals(listOf(15 to false, 60 to false, 0 to true), Choice.entries.map { it.minutes to it.session })
    }
}
