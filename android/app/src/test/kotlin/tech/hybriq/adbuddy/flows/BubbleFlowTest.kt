package tech.hybriq.adbuddy.flows

import android.app.NotificationManager
import android.content.ComponentName
import android.provider.Settings
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isHeading
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.ChatBubbleActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.listener.AgentNotificationListener
import tech.hybriq.adbuddy.listener.NotificationConsent
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.Onboarding

/**
 * Validation 34 (ruling 16 of M5c): the chat bubble (ChatBubbleActivity, firstRun = false) never shows the first-run
 * steps, and its menu reaches Settings, the protected apps and the notification access screen, whose confirm button
 * works. What needs a phone stays manual: `adbuddy tap` and `adb shell input tap` against ADBuddy in front.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class BubbleFlowTest {
    @get:Rule val compose = createAndroidComposeRule<ChatBubbleActivity>()

    private val app get() = RuntimeEnvironment.getApplication()

    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    private var listener: AgentNotificationListener? = null

    @After fun unbind() {
        // The listener's state is a process-wide flow: leave it unbound for the next test (this also clears the mark).
        listener?.onListenerDisconnected()
    }

    private fun openSettings() {
        compose.onNodeWithContentDescription(text(R.string.action_menu)).performClick()
        compose.onNodeWithText(text(R.string.menu_settings)).performClick()
        compose.onNode(hasText(text(R.string.settings_title)) and isHeading()).assertIsDisplayed()
    }

    private fun back() {
        compose.onNodeWithContentDescription(text(R.string.action_back)).performClick()
    }

    /** The system's back (gesture or button), not the bar's arrow. */
    private fun systemBack() {
        compose.activityRule.scenario.onActivity { it.onBackPressedDispatcher.onBackPressed() }
        compose.waitForIdle()
    }

    @Test fun theBubbleNeverShowsTheFirstRunSteps() {
        compose.onNodeWithContentDescription(text(R.string.action_menu)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.step_progress, 1, 2)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.onboarding_skip)).assertDoesNotExist()
        // And it does not mark them done either: the app still shows them on its first run.
        assertFalse(Onboarding.of(app).done())
    }

    @Test fun settingsLeadsToTheProtectedAppsAndTheNotificationAccess() {
        openSettings()
        compose.onNodeWithText(text(R.string.protected_title)).performClick()
        compose.onNodeWithText(text(R.string.protected_intro)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.step_progress, 1, 2)).assertDoesNotExist()
        back()
        compose.onNode(hasText(text(R.string.settings_title)) and isHeading()).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_title)).performClick()
        compose.onNodeWithText(text(R.string.access_intro)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.step_progress, 2, 2)).assertDoesNotExist()
        back()
        back()
        compose.onNodeWithContentDescription(text(R.string.action_menu)).assertIsDisplayed()
        // Nothing along the way started another activity (MainActivity and its first-run steps).
        assertNull(shadowOf(app).nextStartedActivity)
    }

    @Test fun theSystemBackAlsoLeavesTheProtectedAppsForSettings() {
        openSettings()
        compose.onNodeWithText(text(R.string.protected_title)).performClick()
        compose.onNodeWithText(text(R.string.protected_intro)).assertIsDisplayed()
        systemBack()
        compose.onNode(hasText(text(R.string.settings_title)) and isHeading()).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.protected_intro)).assertDoesNotExist()
        assertNull(shadowOf(app).nextStartedActivity)
    }

    private val component get() = ComponentName(app, AgentNotificationListener::class.java)

    /** The system allows the listener and has it bound (what the person does in the system's settings); the confirm button shows. */
    private fun allowInTheSystemAndOpenTheAccess() {
        Settings.Secure.putString(app.contentResolver, "enabled_notification_listeners", component.flattenToString())
        shadowOf(app.getSystemService(NotificationManager::class.java)).setNotificationListenerAccessGranted(component, true)
        listener = Robolectric.buildService(AgentNotificationListener::class.java).create().get().also { it.onListenerConnected() }
        assertFalse(NotificationConsent.of(app).confirmed())
        openSettings()
        compose.onNodeWithText(text(R.string.access_title)).performClick()
        compose.waitUntil(15_000) { compose.onAllNodes(hasText(text(R.string.access_confirm))).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithText(text(R.string.access_state_confirm)).assertIsDisplayed()
    }

    @Test fun theConfirmButtonLetsTheAgentRead() {
        allowInTheSystemAndOpenTheAccess()
        val consent = NotificationConsent.of(app)
        compose.onNodeWithText(text(R.string.access_confirm)).performScrollTo().performClick()

        compose.waitUntil(15_000) { consent.confirmed() }
        compose.waitUntil(15_000) { compose.onAllNodes(hasText(text(R.string.access_withdraw))).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithText(text(R.string.access_state_granted)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_confirm)).assertDoesNotExist()
        assertTrue(consent.confirmed())
    }

    @Test fun aNewBindingOfTheListenerAsksForANewTap() {
        allowInTheSystemAndOpenTheAccess()
        val consent = NotificationConsent.of(app)
        compose.onNodeWithText(text(R.string.access_confirm)).performScrollTo().performClick()
        compose.waitUntil(15_000) { consent.confirmed() }
        compose.waitUntil(15_000) { compose.onAllNodes(hasText(text(R.string.access_withdraw))).fetchSemanticsNodes().isNotEmpty() }

        // The system binds the listener again (a reboot, an update, a process it killed; or, M5c-32 b, a restart it had
        // scheduled before the access was revoked with the app dead, which never calls onListenerDisconnected): the
        // confirmation belonged to a binding that is gone.
        compose.runOnIdle { listener!!.onListenerConnected() }
        assertFalse("the mark survived a new binding of the listener", consent.confirmed())
        compose.waitUntil(15_000) { compose.onAllNodes(hasText(text(R.string.access_confirm))).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithText(text(R.string.access_state_confirm)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.access_withdraw)).assertDoesNotExist()
    }

    @Test fun theConfirmButtonAsksTheSystemAgainAtTheClick() {
        allowInTheSystemAndOpenTheAccess()
        // Revoked in the system after the screen drew the button, before the tap (only the system's own answer changes,
        // so nothing repaints the screen): the click reads the system again and must not set the mark.
        // Simulating only the API (setNotificationListenerAccessGranted) is on purpose: the setting
        // enabled_notification_listeners still says yes, so the two sources disagree and the state is UNKNOWN.
        shadowOf(app.getSystemService(NotificationManager::class.java)).setNotificationListenerAccessGranted(component, false)
        compose.onNodeWithText(text(R.string.access_confirm)).performScrollTo().performClick()
        // The click found the system no longer saying "allowed" and read the state again: the two sources now disagree.
        compose.waitUntil(15_000) { compose.onAllNodes(hasText(text(R.string.access_state_unknown))).fetchSemanticsNodes().isNotEmpty() }
        assertFalse(NotificationConsent.of(app).confirmed())
        compose.onNodeWithText(text(R.string.access_confirm)).assertDoesNotExist()
    }
}
