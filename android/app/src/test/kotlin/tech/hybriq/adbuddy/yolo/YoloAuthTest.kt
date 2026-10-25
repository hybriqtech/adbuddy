package tech.hybriq.adbuddy.yolo

import android.content.Intent
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import tech.hybriq.adbuddy.yolo.YoloUi.Choice

@RunWith(RobolectricTestRunner::class)
class YoloAuthTest {
    private var strong = false
    private var intent: Intent? = Intent("credential")
    private val sent = mutableListOf<Choice>()
    private val launched = mutableListOf<Intent>()
    private var prompted = 0
    private var noLocks = 0
    private val auth = YoloAuth({ strong }, { intent }, { sent += it }, { noLocks++ })

    private fun start(c: Choice) = auth.start(c, { prompted++ }, { launched += it })

    // Mutation: send in start() instead of onResult().
    @Test fun nothingIsSentUntilTheLockIsConfirmed() {
        start(Choice.HOUR)
        assertEquals(1, launched.size)
        assertTrue(sent.isEmpty())
        auth.onResult(true)
        assertEquals(listOf(Choice.HOUR), sent)
    }

    // The two branches. Mutation: swap them (credential first).
    @Test fun withStrongBiometricsOnlyTheBiometricPromptIsShown() {
        strong = true
        start(Choice.QUARTER)
        assertEquals(1, prompted)
        assertTrue("no credential screen as well", launched.isEmpty())
        assertTrue(sent.isEmpty())
        auth.onResult(true)
        assertEquals(listOf(Choice.QUARTER), sent)
    }

    @Test fun withoutStrongBiometricsTheCredentialScreenIsUsed() {
        strong = false
        start(Choice.QUARTER)
        assertEquals(0, prompted)
        assertEquals(1, launched.size)
    }

    // Mutation: ignore `ok` in onResult.
    @Test fun aCancelledOrFailedAuthenticationSendsNothing() {
        start(Choice.QUARTER)
        auth.onResult(false)
        assertTrue(sent.isEmpty())
        strong = true
        start(Choice.QUARTER)
        auth.onResult(false)
        assertTrue(sent.isEmpty())
    }

    // The third branch (R20): no secure lock. The warning is shown (noLock) and nothing is sent until its armed button
    // says OK; Cancel sends nothing. Mutation: send in start() when there is no intent.
    @Test fun withNoSecureLockTheWarningIsShownAndOnlyItsOkSends() {
        strong = false
        intent = null
        start(Choice.SESSION)
        assertEquals(1, noLocks)
        assertTrue(launched.isEmpty())
        assertEquals(0, prompted)
        assertTrue("nothing before the warning is accepted", sent.isEmpty())
        auth.onResult(true)
        assertEquals(listOf(Choice.SESSION), sent)
    }

    @Test fun cancellingTheNoLockWarningSendsNothing() {
        strong = false
        intent = null
        start(Choice.SESSION)
        auth.onResult(false)
        assertTrue(sent.isEmpty())
    }

    // The order: strong biometrics, then the credential, then the warning. Mutation: check the credential first.
    @Test fun theOrderIsBiometricsThenCredentialThenTheWarning() {
        strong = true
        intent = null // even with no lock credential, strong biometrics win
        start(Choice.QUARTER)
        assertEquals(1, prompted)
        assertEquals(0, noLocks)
        strong = false
        intent = Intent("credential")
        start(Choice.QUARTER)
        assertEquals(1, launched.size)
        assertEquals(0, noLocks)
    }


    @Test fun aResultCountsOnceAndOnlyAfterAStart() {
        auth.onResult(true)
        assertTrue(sent.isEmpty())
        start(Choice.QUARTER)
        auth.onResult(true)
        auth.onResult(true)
        assertEquals(listOf(Choice.QUARTER), sent)
    }

    @Test fun aSecondStartReplacesTheFirst() {
        start(Choice.QUARTER)
        start(Choice.HOUR)
        auth.onResult(true)
        assertEquals(listOf(Choice.HOUR), sent)
    }

    // A start on the warning branch replaces the choice of the one before it: an old choice is never sent by a new OK.
    @Test fun aStartOnTheWarningBranchReplacesThePreviousChoice() {
        start(Choice.QUARTER)
        intent = null
        start(Choice.HOUR)
        auth.onResult(true)
        assertEquals(listOf(Choice.HOUR), sent)
    }
}
