package tech.hybriq.adbuddy.ui

import androidx.activity.ComponentActivity
import androidx.compose.material3.SnackbarHostState
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithText
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
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The chat shows the notice for closed red questions only when there are some. */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ChatPendingQuestionsTest {
    private val compose = createAndroidComposeRule<ComponentActivity>()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private fun chat(pending: Int) = compose.setContent {
        AdbuddyTheme(dynamic = false) {
            ChatScreen(
                BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc"),
                outgoing = emptyList(), drafts = emptyList(), downloads = emptyMap(), cut = false, terminal = null,
                snackbar = SnackbarHostState(), actions = ChatActions.None, configured = true,
                pendingQuestions = pending, remoteCard = {},
            )
        }
    }

    @Test fun withoutClosedQuestionsThereIsNoNotice() {
        chat(0)
        compose.onNodeWithText(app.getString(R.string.confirm_waiting_show)).assertDoesNotExist()
    }

    @Test fun withAClosedQuestionTheNoticeShows() {
        chat(1)
        compose.onNodeWithText(app.getString(R.string.confirm_waiting_show)).assertIsDisplayed()
        compose.onNodeWithText("1 delicate action is waiting for your answer").assertIsDisplayed()
    }
}
