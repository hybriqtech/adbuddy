package tech.hybriq.adbuddy.flows

import android.Manifest
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import org.robolectric.shadows.ShadowToast
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.ChatBubbleActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.YoloState
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.YOLO_BAND_TAG

/**
 * The band is on every screen while YOLO is on, with the link up or down, and goes when the host says it is off.
 * Mutation: wrap only the chat route of ChatRoot in the band.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class YoloFlowTest {
    @get:Rule val compose = createAndroidComposeRule<ChatBubbleActivity>()

    init {
        shadowOf(RuntimeEnvironment.getApplication()).grantPermissions(Manifest.permission.POST_NOTIFICATIONS)
    }

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    private fun host(y: YoloState?, connected: Boolean = true) {
        app.bridge.setStateForTest(BridgeState(connection = if (connected) BridgeState.Connection.CONNECTED else BridgeState.Connection.WAITING, yolo = y))
        compose.waitForIdle()
    }

    @Test fun theBandIsOnEveryScreenWhileYoloIsOn() {
        host(YoloState(active = true, session = true))
        compose.onNodeWithTag(YOLO_BAND_TAG).assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.action_menu)).performClick()
        compose.onNodeWithText(text(R.string.menu_activity)).performClick()
        compose.onNodeWithTag(YOLO_BAND_TAG).assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.action_back)).performClick()
        compose.onNodeWithContentDescription(text(R.string.action_menu)).performClick()
        compose.onNodeWithText(text(R.string.menu_settings)).performClick()
        compose.onNodeWithTag(YOLO_BAND_TAG).assertIsDisplayed()
    }

    @Test fun theBandOutlivesTheLinkAndGoesWhenTheHostSaysOff() {
        host(YoloState(active = true, session = true), connected = false)
        compose.onNodeWithTag(YOLO_BAND_TAG).assertIsDisplayed()
        host(YoloState(), connected = true)
        compose.onNodeWithTag(YOLO_BAND_TAG).assertDoesNotExist()
        // Unknown never paints it.
        host(null)
        compose.onNodeWithTag(YOLO_BAND_TAG).assertDoesNotExist()
    }

    // Mutation: drop the `!yoloKnown` branch of openYolo in ChatRoot.
    @Test fun anUnknownStateOpensNoDialogAndSaysWhy() {
        host(null)
        compose.onNodeWithContentDescription(text(R.string.action_menu)).performClick()
        compose.onNodeWithText(text(R.string.yolo_menu_on)).performClick()
        compose.waitForIdle()
        assertEquals(text(R.string.yolo_unknown), ShadowToast.getTextOfLatestToast())
        compose.onNodeWithText(text(R.string.yolo_dialog_title)).assertDoesNotExist()
    }

    @Test fun aKnownOffStateStillOpensTheDialog() {
        host(YoloState())
        compose.onNodeWithContentDescription(text(R.string.action_menu)).performClick()
        compose.onNodeWithText(text(R.string.yolo_menu_on)).performClick()
        compose.onNodeWithText(text(R.string.yolo_dialog_title)).assertIsDisplayed()
    }
}
