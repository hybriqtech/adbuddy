package tech.hybriq.adbuddy.confirm

import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Decision

/**
 * When the notification's Allow is armed (ConfirmArming, with its clock in the test's hands): ConfirmUi.ARMING_MS
 * from when ConfirmNotifier posts the button, counted once per question (posting it again, as a change of language
 * does, does not start over); a question it never saw posted is not armed (fail closed); and it forgets the questions
 * that are no longer pending. ConfirmReceiver.mayReply passes it to the decision: Deny never waits.
 */
class ConfirmArmingTest {
    private var clock = 1_000_000L
    private val arming = ConfirmArming { clock }

    @Test fun anUnknownQuestionIsNotArmed() {
        assertFalse(arming.isArmed("j_1"))
        clock += 3_600_000
        assertFalse("not even an hour later", arming.isArmed("j_1"))
    }

    @Test fun allowIsArmedSixHundredMillisecondsAfterItIsShown() {
        arming.shown("j_1")
        assertFalse(arming.isArmed("j_1"))
        clock += 599
        assertFalse(arming.isArmed("j_1"))
        clock += 1
        assertTrue(arming.isArmed("j_1"))
    }

    // Posting the same question again (its status changed, a change of language) keeps when it was first shown.
    @Test fun showingItAgainDoesNotStartOver() {
        arming.shown("j_1")
        clock += 500
        arming.shown("j_1")
        clock += 100
        assertTrue(arming.isArmed("j_1"))
    }

    @Test fun eachQuestionHasItsOwnArming() {
        arming.shown("j_1")
        clock += 500
        arming.shown("j_2")
        clock += 200
        assertTrue(arming.isArmed("j_1"))
        assertFalse("another question starts its own wait", arming.isArmed("j_2"))
        clock += 400
        assertTrue(arming.isArmed("j_2"))
    }

    // Its button gone (sent, offline, expired) and back: it waits again, as a new button under the finger.
    @Test fun aForgottenQuestionWaitsAgain() {
        arming.shown("j_1")
        clock += 1_000
        arming.forget("j_1")
        assertFalse(arming.isArmed("j_1"))
        arming.shown("j_1")
        assertFalse(arming.isArmed("j_1"))
        clock += 600
        assertTrue(arming.isArmed("j_1"))
    }

    @Test fun itForgetsWhatIsNoLongerPending() {
        arming.shown("j_1")
        arming.shown("j_2")
        clock += 600
        arming.retain(setOf("j_2"))
        assertEquals(setOf("j_2"), arming.tracked())
        assertFalse(arming.isArmed("j_1"))
        assertTrue(arming.isArmed("j_2"))
        arming.clear()
        assertEquals(emptySet<String>(), arming.tracked())
        assertFalse(arming.isArmed("j_2"))
    }

    // The screen turned on (BridgeService, ACTION_SCREEN_ON): every known Allow waits again from now, so the first
    // tap on the lock screen or the shade is not an answer; an unknown question stays unknown.
    @Test fun turningTheScreenOnRearmsEveryKnownQuestion() {
        arming.shown("j_1")
        arming.shown("j_2")
        clock += 5_000
        assertTrue(arming.isArmed("j_1"))
        arming.rearmAll()
        assertFalse(arming.isArmed("j_1"))
        assertFalse(arming.isArmed("j_2"))
        assertEquals(setOf("j_1", "j_2"), arming.tracked())
        assertFalse("an unknown question is still not armed", arming.isArmed("j_3"))
        clock += 599
        assertFalse(arming.isArmed("j_1"))
        clock += 1
        assertTrue(arming.isArmed("j_1"))
        assertTrue(arming.isArmed("j_2"))
        assertFalse(arming.isArmed("j_3"))
        assertEquals("rearmAll adds nothing", setOf("j_1", "j_2"), arming.tracked())
    }

    // --- what the notification buttons send (ConfirmReceiver) ---

    private val now = Instant.parse("2026-09-30T10:00:00Z").toEpochMilli() // the host's clock
    private val state = BridgeState(
        connection = BridgeState.Connection.CONNECTED,
        pending = listOf(ConfirmRequest(id = "j_1", title = "Restart", rawCommand = "adb shell reboot", expiresAt = "2026-09-30T10:01:00Z")),
    )

    private fun mayReply(decision: String, locked: Boolean = false) =
        ConfirmReceiver.mayReply(state, "j_1", decision, now, locked, arming)

    @Test fun theReceiverDropsAnAllowBeforeTheArmingAndSendsDeny() {
        arming.shown("j_1")
        clock += 300
        assertFalse("a tap 300 ms after the button appeared", mayReply(Decision.ALLOW))
        assertTrue("Deny never waits", mayReply(Decision.DENY))
        clock += 300
        assertTrue(mayReply(Decision.ALLOW))
        assertFalse("armed, but locked", mayReply(Decision.ALLOW, locked = true))
    }

    // After the process restarted, the old notification's Allow arrives with nothing known: dropped. Deny still works.
    @Test fun theReceiverDropsAnAllowOfANotificationItNeverPosted() {
        clock += 3_600_000
        assertFalse(mayReply(Decision.ALLOW))
        assertTrue(mayReply(Decision.DENY))
    }
}
