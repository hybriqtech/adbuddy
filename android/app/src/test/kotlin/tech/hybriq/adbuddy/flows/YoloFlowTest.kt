package tech.hybriq.adbuddy.flows

import android.Manifest
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import okhttp3.Request
import okhttp3.WebSocket
import okio.ByteString
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
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
import tech.hybriq.adbuddy.bridge.Bridge
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.Protocol
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

    // The band sits above the new header (Cut pill, menu): stacked, not overlapped. Mutation: draw the band over the chat screen in a Box.
    @Test fun theBandDoesNotCoverTheHeader() {
        host(YoloState(active = true, session = true))
        val band = compose.onNodeWithTag(YOLO_BAND_TAG).fetchSemanticsNode().boundsInRoot
        val menu = compose.onNodeWithContentDescription(text(R.string.action_menu)).fetchSemanticsNode().boundsInRoot
        assertTrue("band $band, menu $menu", band.bottom <= menu.top + 0.5f)
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

    // Mutation: show the band only when `route.parent == null` (so not on the sub-screens of Settings).
    @Test fun theBandIsOnTheListOfProtectedApps() {
        host(YoloState(active = true, session = true))
        compose.onNodeWithContentDescription(text(R.string.action_menu)).performClick()
        compose.onNodeWithText(text(R.string.menu_settings)).performClick()
        compose.onNodeWithText(text(R.string.protected_title)).performClick()
        compose.onNodeWithText(text(R.string.protected_intro)).assertIsDisplayed()
        compose.onNodeWithTag(YOLO_BAND_TAG).assertIsDisplayed()
    }

    // What leaves the app is the request to turn off, with no question in between. Mutation: onOff = {} in ChatRoot.
    @Test fun turnOffInTheBandSendsYoloSetOff() {
        host(YoloState(active = true, session = true))
        val sent = mutableListOf<String>()
        val socket = object : WebSocket {
            override fun request(): Request = Request.Builder().url("http://localhost/").build()
            override fun queueSize(): Long = 0
            override fun send(text: String): Boolean { sent += text; return true }
            override fun send(bytes: ByteString): Boolean = true
            override fun close(code: Int, reason: String?): Boolean = true
            override fun cancel() {}
        }
        val field = Bridge::class.java.getDeclaredField("socket").apply { isAccessible = true }
        field.set(app.bridge, socket)
        compose.onNodeWithText(text(R.string.yolo_band_off)).performClick()
        compose.waitForIdle()
        assertEquals(1, sent.size)
        assertTrue(sent[0], sent[0].contains("\"type\":\"${Protocol.YOLO_SET}\""))
        assertTrue(sent[0], sent[0].contains("\"off\":true"))
        compose.onNodeWithText(text(R.string.yolo_dialog_title)).assertDoesNotExist()
    }
}
