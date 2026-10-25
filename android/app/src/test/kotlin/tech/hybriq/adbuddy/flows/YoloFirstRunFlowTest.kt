package tech.hybriq.adbuddy.flows

import android.Manifest
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.a11y.Packages
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.YoloState
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.YOLO_BAND_TAG

/**
 * The band is also over the first-run steps: the mode is never out of sight.
 * Mutation: draw the band only when `first == null && route.parent == null` in ChatRoot.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class YoloFirstRunFlowTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    init {
        shadowOf(RuntimeEnvironment.getApplication()).grantPermissions(Manifest.permission.POST_NOTIFICATIONS)
        installLauncherApps(Packages.ADBUDDY to "ADBuddy", "com.example.mybank" to "Mi Banco")
    }

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    @Test fun theBandIsOverBothFirstRunSteps() {
        app.bridge.setStateForTest(BridgeState(yolo = YoloState(active = true, session = true)))
        compose.waitForIdle()
        compose.onNodeWithText(text(R.string.step_progress, 1, 2)).assertIsDisplayed()
        compose.onNodeWithTag(YOLO_BAND_TAG).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.onboarding_skip)).performClick()
        compose.onNodeWithText(text(R.string.step_progress, 2, 2)).assertIsDisplayed()
        compose.onNodeWithTag(YOLO_BAND_TAG).assertIsDisplayed()
    }
}
