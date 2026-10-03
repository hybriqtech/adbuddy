package tech.hybriq.adbuddy.ui

import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasScrollToIndexAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollToIndex
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
import tech.hybriq.adbuddy.bridge.ChatMessage
import tech.hybriq.adbuddy.bridge.ChatState
import tech.hybriq.adbuddy.bridge.SessionState
import tech.hybriq.adbuddy.bridge.ToolActivity
import tech.hybriq.adbuddy.screens.ComponentActivityRule
import tech.hybriq.adbuddy.screens.PHONE
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The chat follows what arrives only while the reader is at the end; otherwise it says "New messages" and stays put. */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ChatScrollTest {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private fun text(id: Int): String = app.getString(id)

    private fun messages(n: Int) = (0 until n).map { ChatMessage("$it", if (it % 2 == 0) "user" else "assistant", "message $it") }

    private var state by mutableStateOf(BridgeState())

    private fun start(chat: ChatState = ChatState(messages = messages(30)), session: SessionState = SessionState()) {
        state = BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc", chat = chat, sessionState = session)
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ChatScreen(
                    state, outgoing = emptyList(), drafts = emptyList(), downloads = emptyMap(), cut = false, terminal = null,
                    snackbar = SnackbarHostState(), actions = ChatActions.None, remoteCard = {},
                )
            }
        }
        compose.waitForIdle()
    }

    private fun add(vararg m: ChatMessage) {
        state = state.copy(chat = state.chat.copy(messages = state.chat.messages.map { old -> m.firstOrNull { it.id == old.id } ?: old } + m.filter { n -> state.chat.messages.none { it.id == n.id } }))
        compose.waitForIdle()
    }

    private val notice get() = compose.onNodeWithText(text(R.string.chat_new_messages))
    private fun shown(t: String) = compose.onNodeWithText(t).assertIsDisplayed()
    private fun scrollUp() = compose.onNode(hasScrollToIndexAction()).performScrollToIndex(0).also { compose.waitForIdle() }

    @Test fun startsAtTheEndWithoutNotice() {
        start()
        shown("message 29")
        notice.assertDoesNotExist()
    }

    @Test fun readingAboveKeepsPlaceAndNotices() {
        start()
        scrollUp()
        add(ChatMessage("30", "user", "message 30"))
        compose.onNodeWithText("message 30").assertDoesNotExist()
        notice.assertIsDisplayed()
        shown("message 0")
    }

    @Test fun tappingTheNoticeGoesToTheEnd() {
        start()
        scrollUp()
        add(ChatMessage("30", "user", "message 30"))
        notice.performClick()
        compose.waitForIdle()
        shown("message 30")
        notice.assertDoesNotExist()
    }

    @Test fun atTheEndFollowsWithoutNotice() {
        start()
        add(ChatMessage("30", "user", "message 30"))
        add(ChatMessage("31", "assistant", "message 31"))
        shown("message 31")
        notice.assertDoesNotExist()
    }

    @Test fun lastBubbleGrowingInPlaceStillFollows() {
        start()
        add(ChatMessage("29", "assistant", "long ".repeat(60)))
        add(ChatMessage("30", "user", "message 30"))
        shown("message 30")
        notice.assertDoesNotExist()
    }

    @Test fun theToolStripGoingIsNotNews() {
        val working = SessionState(state = "working")
        start(ChatState(messages = messages(30), tools = listOf(ToolActivity("t1", "", "Run 1", "running"))), working)
        scrollUp()
        state = state.copy(chat = state.chat.copy(tools = emptyList()), sessionState = SessionState(state = "idle"))
        compose.waitForIdle()
        notice.assertDoesNotExist()
    }
}
