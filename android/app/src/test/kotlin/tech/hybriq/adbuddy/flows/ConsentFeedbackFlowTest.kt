package tech.hybriq.adbuddy.flows

import android.content.Intent
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ActivityScenario
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import org.robolectric.shadows.ShadowToast
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgePrefs
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.Onboarding

/** The notice after answering the consent: what was (not) done and what remains in the app. */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ConsentFeedbackFlowTest {
    @get:Rule val compose = createEmptyComposeRule()

    private var scenario: ActivityScenario<MainActivity>? = null

    private val app get() = RuntimeEnvironment.getApplication()

    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    private fun launchPending() {
        val prefs = BridgePrefs(app)
        prefs.consent = "pending"
        prefs.consentHost = "my-pc"
        scenario = ActivityScenario.launch(
            Intent(app, MainActivity::class.java).setAction(MainActivity.ACTION_SETUP),
        )
    }

    @After fun close() {
        scenario?.close()
    }

    @Test fun decliningSaysNothingWasSetUp() {
        launchPending()
        compose.onNodeWithText(text(R.string.consent_decline)).performClick()
        assertEquals("denied", BridgePrefs(app).consent)
        assertEquals(text(R.string.consent_declined), ShadowToast.getTextOfLatestToast())
        compose.onNodeWithText(text(R.string.consent_accept)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.step_progress, 1, 2)).assertIsDisplayed()
    }

    @Test fun acceptingWithStepsPendingSaysTwoStepsRemain() {
        assertFalse(Onboarding.of(app).done())
        launchPending()
        compose.onNodeWithText(text(R.string.consent_accept)).performClick()
        assertEquals("granted", BridgePrefs(app).consent)
        assertEquals(text(R.string.consent_done_steps), ShadowToast.getTextOfLatestToast())
    }

    @Test fun acceptingWithOnboardingDoneSaysGoBackToThePc() {
        Onboarding.of(app).finish()
        launchPending()
        compose.onNodeWithText(text(R.string.consent_accept)).performClick()
        assertEquals("granted", BridgePrefs(app).consent)
        assertEquals(text(R.string.consent_done), ShadowToast.getTextOfLatestToast())
    }
}
