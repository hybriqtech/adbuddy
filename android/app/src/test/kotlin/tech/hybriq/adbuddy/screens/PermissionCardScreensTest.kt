package tech.hybriq.adbuddy.screens

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import java.time.Instant
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.PermissionRequest
import tech.hybriq.adbuddy.ui.PermissionCard
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The agent's question card in every ScreenMode: its name in the title, a long monospaced detail, "Yes" off until it is read. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class PermissionCardScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    @Test fun theCardNamesTheAgentAndKeepsTheGrantsOffUntilTheDetailIsRead() {
        val detail = (1..40).joinToString("\n") { "adb shell settings put global stay_on_while_plugged_in $it" }
        val p = PermissionRequest("p1", "Bash", "Change the screen timeout", detail, "2026-10-01T10:10:00Z", true)
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                PermissionCard(
                    p, others = 0, connected = true, hostNow = Instant.parse("2026-10-01T10:08:30Z").toEpochMilli(), answering = false,
                    guardLeftMs = 0, controllerCanAnswer = true, agent = "claude-code", onAnswer = { _, _ -> },
                )
            }
        }
        compose.waitForIdle()
        compose.onNodeWithText(app.getString(R.string.perm_title, "claude-code")).assertIsDisplayed()
        compose.onNodeWithText(app.getString(R.string.perm_allow)).assertIsNotEnabled()
        compose.assertTextsFit(scrolled = true)
        compose.shot("permission_card", mode)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
