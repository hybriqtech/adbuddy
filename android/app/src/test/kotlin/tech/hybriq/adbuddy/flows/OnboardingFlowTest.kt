package tech.hybriq.adbuddy.flows

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isEnabled
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.a11y.Packages
import tech.hybriq.adbuddy.a11y.ProtectedApps
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.Onboarding

/**
 * Validation 27, the first-run steps in the app (MainActivity, firstRun = true): step 1 and step 2, "Not now",
 * and Back (from step 2 to step 1; from step 1 it leaves the app, never into the chat past the steps).
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class OnboardingFlowTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    private val app get() = RuntimeEnvironment.getApplication()

    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    private fun step(n: Int) = compose.onNodeWithText(text(R.string.step_progress, n, 2))

    private fun pressBack() = compose.runOnUiThread { compose.activity.onBackPressedDispatcher.onBackPressed() }

    @Test fun notNowTwiceReachesTheChatAndBackReturnsAStep() {
        step(1).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_skip)).performClick()
        step(2).assertIsDisplayed()
        pressBack()
        step(1).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_skip)).performClick()
        step(2).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_skip)).performClick()
        compose.onNodeWithContentDescription(text(R.string.action_menu)).assertIsDisplayed()
        step(1).assertDoesNotExist()
        step(2).assertDoesNotExist()
        assertTrue(Onboarding.of(app).done())
        // "Not now" on step 1 saved nothing: the list is still a proposal.
        assertFalse(ProtectedApps.of(app).confirmed)
    }

    // Before the rule: MainActivity starts on the first step and reads the launcher right away (a @Before comes after).
    init {
        // ADBuddy's own icon makes the launcher read usable. Proposed: the code receivers (WhatsApp, Play services) and what looks protected ("Mi Banco"), not "Notes".
        installLauncherApps(Packages.ADBUDDY to "ADBuddy", "com.example.mybank" to "Mi Banco", "com.whatsapp" to "WhatsApp", "org.example.notes" to "Notes")
    }

    @Test fun confirmingTheListThenDoneFinishesTheSteps() {
        // The apps are read off the main thread: the button turns on once they are there.
        compose.waitUntil(15_000) { compose.onAllNodes(hasText(text(R.string.protected_confirm)) and isEnabled()).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithText(text(R.string.protected_confirm)).performClick()
        step(2).assertIsDisplayed()
        assertTrue(ProtectedApps.of(app).confirmed)
        assertEquals(setOf("com.example.mybank", "com.whatsapp", "com.google.android.gms"), ProtectedApps.of(app).list())
        compose.onNodeWithText(text(R.string.onboarding_done)).performClick()
        compose.onNodeWithContentDescription(text(R.string.action_menu)).assertIsDisplayed()
        assertTrue(Onboarding.of(app).done())
    }

    @Test fun backOnTheFirstStepLeavesWithoutSkippingIt() {
        step(1).assertIsDisplayed()
        pressBack()
        assertTrue(compose.activity.isFinishing)
        assertFalse(Onboarding.of(app).done())
    }
}
