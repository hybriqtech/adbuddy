package tech.hybriq.adbuddy.screens

import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.text.TextLayoutResult
import org.junit.Assert.assertEquals
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
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ChatMessage
import tech.hybriq.adbuddy.bridge.ChatState
import tech.hybriq.adbuddy.bridge.SessionState
import tech.hybriq.adbuddy.bridge.ToolActivity
import tech.hybriq.adbuddy.chat.Outgoing
import tech.hybriq.adbuddy.chat.SendStatus
import tech.hybriq.adbuddy.ui.ChatActions
import tech.hybriq.adbuddy.ui.ChatScreen
import tech.hybriq.adbuddy.ui.theme.AdbuddyTheme

/** The chat screen (header, link banner, conversation) in every ScreenMode: what is wrong with the link reads whole and in a header of one line. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = PHONE)
class ChatScreensTest(private val mode: ScreenMode) {
    private val compose = createComposeRule()

    @get:Rule val rules: RuleChain = RuleChain.outerRule(ScreenModeRule(mode)).around(ComponentActivityRule()).around(compose)

    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    private fun text(id: Int, vararg args: Any): String = app.getString(id, *args)

    private fun show(state: BridgeState, cut: Boolean = false, configured: Boolean = true, outgoing: List<Outgoing> = emptyList(), terminalOpenByDefault: Boolean = true) {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ChatScreen(
                    state, outgoing = outgoing, drafts = emptyList(), downloads = emptyMap(), cut = cut, terminal = null,
                    snackbar = SnackbarHostState(), actions = ChatActions.None, configured = configured, terminalOpenByDefault = terminalOpenByDefault, remoteCard = {},
                )
            }
        }
    }

    private fun lineCount(t: String): Int {
        val node = compose.onNodeWithText(t).fetchSemanticsNode()
        val results = mutableListOf<TextLayoutResult>()
        node.config[SemanticsActions.GetTextLayoutResult].action!!(results)
        return results.first().lineCount
    }

    /** The header must fit its bar, the subtitle be one line and the banner (if any) show whole. */
    private fun check(subtitle: String, banner: String?, name: String) {
        compose.onNodeWithText(subtitle).assertIsDisplayed()
        if (banner != null) {
            compose.onNodeWithText(banner).assertIsDisplayed()
        }
        assertEquals(1, lineCount(subtitle))
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(subtitle)
        compose.shot(name, mode)
    }

    @Test fun connectedWithMessagesHasNoBanner() {
        show(
            BridgeState(
                connection = BridgeState.Connection.CONNECTED, hostName = "my-pc",
                chat = ChatState(messages = listOf(ChatMessage("1", "user", "Turn the volume down"), ChatMessage("2", "assistant", "Done. The media volume is now at 20%."))),
            ),
        )
        compose.onNodeWithText(text(R.string.app_name)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.session_idle)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.chat_empty_ready)).assertDoesNotExist()
        compose.onAllNodes(SemanticsMatcher.keyIsDefined(SemanticsProperties.LiveRegion)).assertCountEquals(0)
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(text(R.string.session_idle))
        compose.shot("chat_connected_messages", mode)
    }

    @Test fun workingShowsStopInTheInputBar() {
        show(
            BridgeState(
                connection = BridgeState.Connection.CONNECTED, hostName = "my-pc", sessionState = SessionState(state = "working"),
                chat = ChatState(messages = listOf(ChatMessage("1", "user", "Turn the volume down"))),
            ),
        )
        compose.onNodeWithText(text(R.string.session_working)).assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.chat_stop)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("chat_working", mode)
    }

    private val connected = BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc")

    @Test fun theTerminalStartsOpenByDefault() {
        show(connected)
        compose.onNodeWithContentDescription(text(R.string.chat_terminal_hide)).assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.chat_terminal_show)).assertDoesNotExist()
    }

    @Test fun theTerminalStartsFoldedWhenAskedTo() {
        show(connected, terminalOpenByDefault = false)
        compose.onNodeWithContentDescription(text(R.string.chat_terminal_show)).assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.chat_terminal_hide)).assertDoesNotExist()
    }

    @Test fun offlineWaitingSaysItShortInTheHeaderAndInFullInTheBanner() {
        show(BridgeState())
        val short = text(R.string.status_short_waiting)
        compose.onNodeWithText(text(R.string.chat_empty_ready)).assertDoesNotExist()
        check(short, text(R.string.status_waiting), "chat_offline_waiting")
    }

    @Test fun cutSaysSoInTheBanner() {
        show(BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc"), cut = true)
        compose.onNodeWithText(text(R.string.session_idle)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.chat_empty_ready)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.status_cut)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(text(R.string.session_idle))
        compose.shot("chat_cut", mode)
    }

    @Test fun updateAppShowsTheWholeCommandInTheBanner() {
        show(BridgeState().appOutdated())
        val short = text(R.string.status_short_update)
        compose.onNodeWithText(text(R.string.chat_empty_ready)).assertDoesNotExist()
        check(short, text(R.string.status_update_app), "chat_update_app")
        // The banner is announced to TalkBack.
        compose.onNode(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Polite)).assertIsDisplayed()
    }

    @Test fun connectedAndEmptySaysWhereToWrite() {
        show(BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc"))
        compose.onNodeWithText(text(R.string.chat_empty_ready)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("chat_connected_empty", mode)
    }

    @Test fun connectedWithOnlyTheToolStripIsNotEmpty() {
        show(
            BridgeState(
                connection = BridgeState.Connection.CONNECTED, hostName = "my-pc",
                chat = ChatState(tools = listOf(ToolActivity("t1", "", "Run 1", "running"))),
                sessionState = SessionState(state = "working"),
            ),
        )
        compose.onNodeWithText(text(R.string.chat_empty_ready)).assertDoesNotExist()
    }

    @Test fun connectedWithAPendingOutgoingMessageIsNotEmpty() {
        show(
            BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc"),
            outgoing = listOf(Outgoing("o1", "Turn the volume down", emptyList(), SendStatus.SENDING)),
        )
        compose.onNodeWithText(text(R.string.chat_empty_ready)).assertDoesNotExist()
    }

    @Test fun theBannerKeepsItsStateWhileTheLinkRetries() {
        var bridge by mutableStateOf(BridgeState())
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ChatScreen(
                    bridge, outgoing = emptyList(), drafts = emptyList(), downloads = emptyMap(), cut = false, terminal = null,
                    snackbar = SnackbarHostState(), actions = ChatActions.None, configured = true, remoteCard = {},
                )
            }
        }
        val waiting = text(R.string.status_waiting)
        compose.onNodeWithText(waiting).assertIsDisplayed()
        bridge = BridgeState(connection = BridgeState.Connection.CONNECTING)
        compose.waitForIdle()
        compose.onNodeWithText(waiting).assertIsDisplayed()
        bridge = BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc")
        compose.waitForIdle()
        compose.onNodeWithText(waiting).assertDoesNotExist()
        // Connected forgot it: connecting again (a cold start in effect) shows none.
        bridge = BridgeState(connection = BridgeState.Connection.CONNECTING)
        compose.waitForIdle()
        compose.onNodeWithText(waiting).assertDoesNotExist()
    }

    private fun bannerLeavesWhenConnecting(from: BridgeState, fromCut: Boolean, toCut: Boolean, banner: String) {
        var bridge by mutableStateOf(from)
        var cut by mutableStateOf(fromCut)
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ChatScreen(
                    bridge, outgoing = emptyList(), drafts = emptyList(), downloads = emptyMap(), cut = cut, terminal = null,
                    snackbar = SnackbarHostState(), actions = ChatActions.None, configured = true, remoteCard = {},
                )
            }
        }
        compose.onNodeWithText(banner).assertIsDisplayed()
        bridge = BridgeState(connection = BridgeState.Connection.CONNECTING)
        cut = toCut
        compose.waitForIdle()
        compose.onNodeWithText(banner).assertDoesNotExist()
    }

    @Test fun anOutdatedAppBannerDoesNotSurviveConnecting() =
        bannerLeavesWhenConnecting(BridgeState().appOutdated(), false, false, text(R.string.status_update_app))

    @Test fun aPausedBannerDoesNotSurviveConnecting() = bannerLeavesWhenConnecting(
        BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc", sessionState = SessionState(paused = true)),
        false, false, text(R.string.status_paused),
    )

    @Test fun aCutBannerDoesNotSurviveTheCutEndingWhileConnecting() =
        bannerLeavesWhenConnecting(BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc"), true, false, text(R.string.status_cut))

    @Test fun aColdStartConnectingHasNoBanner() {
        show(BridgeState(connection = BridgeState.Connection.CONNECTING))
        compose.onNodeWithText(text(R.string.status_waiting)).assertDoesNotExist()
    }

    @Test fun notSetUpSaysToRunSetup() {
        show(BridgeState(), configured = false)
        check(text(R.string.status_short_not_set_up), text(R.string.status_not_set_up), "chat_not_set_up")
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
