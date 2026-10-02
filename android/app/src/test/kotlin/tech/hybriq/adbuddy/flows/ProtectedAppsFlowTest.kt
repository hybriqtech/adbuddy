package tech.hybriq.adbuddy.flows

import android.content.res.Configuration
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.hasScrollToKeyAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isHeading
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollToKey
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import org.robolectric.shadows.ShadowToast
import tech.hybriq.adbuddy.ChatBubbleActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.a11y.ProtectedApps
import tech.hybriq.adbuddy.screens.PHONE

/**
 * Validation 27, the list of apps from Settings (reached from the bubble, which has no first-run steps): the search,
 * a rotation that keeps the selection, and leaving with changes (discard or keep them), and saving.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ProtectedAppsFlowTest {
    @get:Rule val compose = createAndroidComposeRule<ChatBubbleActivity>()

    private val app get() = RuntimeEnvironment.getApplication()

    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    @Before fun apps() {
        installLauncherApps("com.example.mybank" to "Mi Banco", "com.whatsapp" to "WhatsApp", "org.example.notes" to "Notes")
        // A list the person already confirmed: the screen opens in "edit" mode, with Save off until something changes.
        ProtectedApps.of(app).confirm(setOf("com.example.mybank"))
    }

    private fun openProtectedApps() {
        compose.onNodeWithContentDescription(text(R.string.action_menu)).performClick()
        compose.onNodeWithText(text(R.string.menu_settings)).performClick()
        compose.onNodeWithText(text(R.string.protected_title)).performClick()
        // The apps are read off the main thread: wait for the rows.
        compose.waitUntil(15_000) { compose.onAllNodes(hasText("Mi Banco")).fetchSemanticsNodes().isNotEmpty() }
    }

    private fun row(pkg: String, label: String) = compose.run {
        onNode(hasScrollToKeyAction()).performScrollToKey(pkg)
        onNodeWithText(label)
    }

    private fun settingsShown() = compose.onNode(hasText(text(R.string.settings_title)) and isHeading()).assertIsDisplayed()

    @Test fun theSearchFiltersByNameAndSaysWhenNothingMatches() {
        openProtectedApps()
        // "banco" is only in the name (the package is com.example.mybank), as a person searches.
        compose.onNodeWithText(text(R.string.protected_search)).performTextInput("banco")
        compose.onNodeWithText("Mi Banco").assertIsDisplayed()
        compose.onNodeWithText("WhatsApp").assertDoesNotExist()
        compose.onNodeWithText("Notes").assertDoesNotExist()
        compose.onNodeWithText("banco").performTextClearance()
        // Lower case finds the name in any case.
        compose.onNodeWithText(text(R.string.protected_search)).performTextInput("whatsapp")
        compose.onNodeWithText("WhatsApp").assertIsDisplayed()
        compose.onNodeWithText("Mi Banco").assertDoesNotExist()
        compose.onNodeWithText("whatsapp").performTextClearance()
        // Upper and mixed case too: the query is lowered as well as the name.
        compose.onNodeWithText(text(R.string.protected_search)).performTextInput("Banco")
        compose.onNodeWithText("Mi Banco").assertIsDisplayed()
        compose.onNodeWithText("WhatsApp").assertDoesNotExist()
        compose.onNodeWithText("Banco").performTextClearance()
        compose.onNodeWithText(text(R.string.protected_search)).performTextInput("WHATS")
        compose.onNodeWithText("WhatsApp").assertIsDisplayed()
        compose.onNodeWithText("Mi Banco").assertDoesNotExist()
        compose.onNodeWithText("WHATS").performTextClearance()
        compose.onNodeWithText(text(R.string.protected_search)).performTextInput("zzz")
        compose.onNodeWithText(text(R.string.protected_empty)).assertIsDisplayed()
    }

    @Test fun aRotationKeepsTheSelection() {
        openProtectedApps()
        compose.onNodeWithText(text(R.string.protected_save)).assertIsNotEnabled()
        row("com.whatsapp", "WhatsApp").assertIsOff().performClick()
        row("com.whatsapp", "WhatsApp").assertIsOn()

        RuntimeEnvironment.setQualifiers("+land")
        compose.activityRule.scenario.recreate()
        assertEquals(Configuration.ORIENTATION_LANDSCAPE, compose.activity.resources.configuration.orientation)

        compose.waitUntil(15_000) { compose.onAllNodes(hasText(text(R.string.protected_save))).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithText(text(R.string.protected_intro)).assertExists()
        row("com.whatsapp", "WhatsApp").assertIsOn()
        row("com.example.mybank", "Mi Banco").assertIsOn()
        compose.onNodeWithText(text(R.string.protected_save)).assertIsEnabled()
    }

    @Test fun leavingWithChangesAsksAndDiscardDropsThem() {
        openProtectedApps()
        row("com.whatsapp", "WhatsApp").performClick()
        compose.onNodeWithContentDescription(text(R.string.action_back)).performClick()
        compose.onNodeWithText(text(R.string.protected_discard_title)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.protected_discard_confirm)).performClick()
        settingsShown()
        assertEquals(setOf("com.example.mybank"), ProtectedApps.of(app).list())
        compose.onNodeWithText(text(R.string.protected_title)).performClick()
        compose.waitUntil(15_000) { compose.onAllNodes(hasText("Mi Banco")).fetchSemanticsNodes().isNotEmpty() }
        row("com.whatsapp", "WhatsApp").assertIsOff()
    }

    @Test fun theSystemBackWithChangesAsksToo() {
        openProtectedApps()
        row("com.whatsapp", "WhatsApp").performClick()
        // Recomposed with the change (a person's Back comes long after the tap; the test's would not).
        row("com.whatsapp", "WhatsApp").assertIsOn()
        // The usual gesture on the phone: the system's Back, not the arrow.
        compose.runOnUiThread { compose.activity.onBackPressedDispatcher.onBackPressed() }
        compose.onNodeWithText(text(R.string.protected_discard_title)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.protected_discard_confirm)).performClick()
        settingsShown()
        assertEquals(setOf("com.example.mybank"), ProtectedApps.of(app).list())
    }

    @Test fun cancelKeepsTheChangesOnScreen() {
        openProtectedApps()
        row("com.whatsapp", "WhatsApp").performClick()
        compose.onNodeWithContentDescription(text(R.string.action_back)).performClick()
        compose.onNodeWithText(text(R.string.action_cancel)).performClick()
        compose.onNodeWithText(text(R.string.protected_discard_title)).assertDoesNotExist()
        row("com.whatsapp", "WhatsApp").assertIsOn()
        compose.onNodeWithText(text(R.string.protected_save)).assertIsEnabled()
    }

    @Test fun savingConfirmsTheListAndSaysThePcGetsItLater() {
        openProtectedApps()
        row("com.whatsapp", "WhatsApp").performClick()
        compose.onNodeWithText(text(R.string.protected_save)).performClick()
        settingsShown()
        val saved = ProtectedApps.of(app)
        assertTrue(saved.confirmed)
        assertEquals(setOf("com.example.mybank", "com.whatsapp"), saved.list())
        // No PC connected in the test: the list waits for the next connection, and the person is told so.
        assertEquals(text(R.string.protected_saved_later), ShadowToast.getTextOfLatestToast())
        compose.onNodeWithText(text(R.string.settings_protected_confirmed, saved.list().size)).assertIsDisplayed()
    }
}
