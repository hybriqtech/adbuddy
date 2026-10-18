package tech.hybriq.adbuddy.flows

import android.Manifest
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isDialog
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
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.a11y.Packages
import tech.hybriq.adbuddy.a11y.ProtectedApps
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.BridgePrefs
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.ChatState
import tech.hybriq.adbuddy.bridge.PermissionRequest
import tech.hybriq.adbuddy.AdbuddyApp
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
        // R15: the full-screen chat starts with the terminal folded too (it was unfolded before): guards ChatRoot's default.
        compose.onNodeWithContentDescription(text(R.string.chat_terminal_show)).assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.chat_terminal_hide)).assertDoesNotExist()
        step(1).assertDoesNotExist()
        step(2).assertDoesNotExist()
        assertTrue(Onboarding.of(app).done())
        // "Not now" on step 1 saved nothing: the list is still a proposal.
        assertFalse(ProtectedApps.of(app).confirmed)
    }

    // Before the rule: MainActivity starts on the first step and reads the launcher right away (a @Before comes after).
    init {
        // ADBuddy's own icon makes the launcher read usable. Proposed: the code receivers (WhatsApp, Play services) and what looks protected ("Mi Banco"), not "Notes".
        shadowOf(RuntimeEnvironment.getApplication()).grantPermissions(Manifest.permission.POST_NOTIFICATIONS)
        installLauncherApps(Packages.ADBUDDY to "ADBuddy", "com.example.mybank" to "Mi Banco", "com.whatsapp" to "WhatsApp", "org.example.notes" to "Notes")
    }

    @Test fun confirmingTheListThenNotNowFinishesTheSteps() {
        // The apps are read off the main thread: the button turns on once they are there.
        compose.waitUntil(15_000) { compose.onAllNodes(hasText(text(R.string.protected_confirm)) and isEnabled()).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithText(text(R.string.protected_confirm)).performClick()
        step(2).assertIsDisplayed()
        assertTrue(ProtectedApps.of(app).confirmed)
        assertEquals(setOf("com.example.mybank", "com.whatsapp", "com.google.android.gms"), ProtectedApps.of(app).list())
        compose.onNodeWithText(text(R.string.onboarding_skip)).performClick()
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

/** The reason for the notification permission: after the first-run steps, once, never over a question. The permission is denied. */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class NotificationsRationaleFlowTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    private val app get() = RuntimeEnvironment.getApplication()

    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    private val title get() = compose.onNodeWithText(text(R.string.notifications_rationale_title))

    init {
        shadowOf(RuntimeEnvironment.getApplication()).denyPermissions(Manifest.permission.POST_NOTIFICATIONS)
        installLauncherApps(Packages.ADBUDDY to "ADBuddy")
    }

    private fun skip() = compose.onNodeWithText(text(R.string.onboarding_skip)).performClick()

    /** The channel's state, as the host would have left it. */
    private fun setBridgeState(s: BridgeState) {
        compose.runOnUiThread { (app as AdbuddyApp).bridge.setStateForTest(s) }
        compose.waitForIdle()
    }

    private fun connected(chat: ChatState = ChatState(), pending: List<ConfirmRequest> = emptyList()) =
        BridgeState(connection = BridgeState.Connection.CONNECTED, chat = chat, pending = pending)

    @Test fun notOverTheStepsButAfterThemOnceOnly() {
        compose.onNodeWithText(text(R.string.step_progress, 1, 2)).assertIsDisplayed()
        title.assertDoesNotExist()
        skip()
        compose.onNodeWithText(text(R.string.step_progress, 2, 2)).assertIsDisplayed()
        title.assertDoesNotExist()
        skip()
        // Both steps done: the chat, with the reason on top.
        title.assertIsDisplayed()
        assertFalse(BridgePrefs(app).notificationsAsked)
        // "Not now" inside the dialog closes it for good.
        compose.onNode(hasText(text(R.string.onboarding_skip)) and hasAnyAncestor(isDialog())).performClick()
        title.assertDoesNotExist()
        assertTrue(BridgePrefs(app).notificationsAsked)
        compose.waitForIdle()
        title.assertDoesNotExist()
        compose.activityRule.scenario.recreate()
        compose.waitForIdle()
        title.assertDoesNotExist()
    }

    private fun reachTheChatWithTheReason() {
        skip()
        skip()
        title.assertIsDisplayed()
    }

    @Test fun aRedQuestionThatArrivesWithTheReasonOpenReplacesIt() {
        reachTheChatWithTheReason()
        // No recreation: the question comes in while the dialog is on screen.
        setBridgeState(connected(pending = listOf(ConfirmRequest("r1", title = "Delete it", rawCommand = "rm x", expiresAt = "2099-01-01T00:00:00Z"))))
        title.assertDoesNotExist()
        compose.onNodeWithText(text(R.string.confirm_label)).assertIsDisplayed()
        assertFalse(BridgePrefs(app).notificationsAsked)
    }

    @Test fun aQuestionOfTheAgentThatArrivesWithTheReasonOpenReplacesIt() {
        reachTheChatWithTheReason()
        val q = PermissionRequest("a", "Bash", "Clean the build folder", "detail", "2099-01-01T00:00:00Z", true)
        setBridgeState(connected(chat = ChatState(permissions = listOf(q))))
        title.assertDoesNotExist()
        compose.onNodeWithText(text(R.string.perm_title)).assertIsDisplayed()
        assertFalse(BridgePrefs(app).notificationsAsked)
    }

    @Test fun neverOverAQuestionOfTheAgent() {
        reachTheChatWithTheReason()
        // Start over in a fresh state: the permission was never asked and a question is waiting.
        compose.onNode(hasText(text(R.string.onboarding_skip)) and hasAnyAncestor(isDialog())).performClick()
        BridgePrefs(app).notificationsAsked = false
        val q = PermissionRequest("a", "Bash", "Clean the build folder", "detail", "2099-01-01T00:00:00Z", true)
        setBridgeState(connected(chat = ChatState(permissions = listOf(q))))
        compose.activityRule.scenario.recreate()
        compose.waitForIdle()
        title.assertDoesNotExist()
        assertFalse(BridgePrefs(app).notificationsAsked)
    }
}
