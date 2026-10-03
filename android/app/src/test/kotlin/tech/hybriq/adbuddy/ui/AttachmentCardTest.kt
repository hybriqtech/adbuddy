package tech.hybriq.adbuddy.ui

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.files.DownloadState
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** C15: the file card is a button, says what tapping does in every state, and says "tap to cancel" while it can be cancelled. */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class AttachmentCardTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private fun text(id: Int): String = app.getString(id)

    private val attachment = Attachment("0".repeat(32), "report.pdf", "application/pdf", 400, "0".repeat(64))

    private var state by mutableStateOf<DownloadState?>(null)

    private fun clickLabel(): String? =
        compose.onNode(hasClickAction()).fetchSemanticsNode().config[SemanticsActions.OnClick].label

    @Test fun aRunningDownloadSaysTappingCancelsItAndTheCardIsAButton() {
        state = DownloadState.Running(100, 400)
        compose.setContent { AdbuddyTheme(dynamic = false) { AttachmentCard(attachment, state) {} } }
        compose.onNodeWithText(text(R.string.file_tap_cancel)).assertIsDisplayed()
        assertEquals(Role.Button, compose.onNode(hasClickAction()).fetchSemanticsNode().config.getOrNull(SemanticsProperties.Role))
        assertEquals(text(R.string.file_action_cancel), clickLabel())
    }

    @Test fun everyStateNamesWhatTappingDoes() {
        compose.setContent { AdbuddyTheme(dynamic = false) { AttachmentCard(attachment, state) {} } }
        val expected = listOf(
            null to R.string.file_action_download,
            DownloadState.Running(100, 400) to R.string.file_action_cancel,
            DownloadState.Done(File("x")) to R.string.file_action_open,
            DownloadState.Failed("") to R.string.file_action_retry,
        )
        for ((s, label) in expected) {
            state = s
            compose.waitForIdle()
            assertEquals("label for $s", text(label), clickLabel())
            // "Tap to cancel" is only said while it can be cancelled.
            if (s !is DownloadState.Running) compose.onNodeWithText(text(R.string.file_tap_cancel)).assertDoesNotExist()
        }
    }
}
