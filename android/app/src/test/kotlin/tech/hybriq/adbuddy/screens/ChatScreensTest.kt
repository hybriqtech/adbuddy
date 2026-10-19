package tech.hybriq.adbuddy.screens

import android.content.Intent
import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.text.TextLayoutResult
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Rule
import org.junit.Test
import org.junit.rules.RuleChain
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ChatMessage
import tech.hybriq.adbuddy.bridge.ChatState
import tech.hybriq.adbuddy.bridge.RemoteState
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

    private fun show(state: BridgeState, cut: Boolean = false, configured: Boolean = true, outgoing: List<Outgoing> = emptyList(), terminalOpenByDefault: Boolean = false, notificationsBlocked: Boolean = false) {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ChatScreen(
                    state, outgoing = outgoing, drafts = emptyList(), downloads = emptyMap(), cut = cut, terminal = null,
                    snackbar = SnackbarHostState(), actions = ChatActions.None, configured = configured, terminalOpenByDefault = terminalOpenByDefault, notificationsBlocked = notificationsBlocked,
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

    /** The header must fit its bar, the pill be one line and the card (if any) show whole: its title and, if given, its text. */
    private fun check(subtitle: String, banner: String?, name: String, body: String? = null) {
        compose.onNodeWithText(subtitle).assertIsDisplayed()
        if (banner != null) {
            compose.onNodeWithText(banner).assertIsDisplayed()
        }
        if (body != null) compose.onNodeWithText(body).assertIsDisplayed()
        // One line at normal text; at 200 % the pill may wrap (assertTextsFit below: whole, never cut short or off the bar).
        if (mode.fontScale == 1f) assertEquals(1, lineCount(subtitle))
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
        // The agent and its state are one title now ("ADBuddy · Ready"): each part is in it.
        compose.onNodeWithText(text(R.string.app_name), substring = true).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.session_idle), substring = true).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.title_agent_state, text(R.string.app_name), text(R.string.session_idle))).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.chat_empty_ready)).assertDoesNotExist()
        compose.onAllNodes(SemanticsMatcher.keyIsDefined(SemanticsProperties.LiveRegion)).assertCountEquals(0)
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(text(R.string.title_agent_state, text(R.string.app_name), text(R.string.session_idle)))
        compose.assertTitleFitsTheTopBar(pill(text(R.string.status_connected, "my-pc")))
        compose.shot("chat_connected_messages", mode)
    }

    @Test fun theNotificationsBannerShowsOnlyWhenTheyAreBlocked() {
        val connected = BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc")
        show(connected, notificationsBlocked = true)
        compose.onNodeWithText(text(R.string.notifications_blocked)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.notifications_open_settings)).assertIsDisplayed()
    }

    @Test fun withNotificationsOnThereIsNoNotificationsBanner() {
        show(BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc"), notificationsBlocked = false)
        compose.onNodeWithText(text(R.string.notifications_blocked)).assertDoesNotExist()
    }

    @Test fun workingShowsStopInTheInputBar() {
        show(
            BridgeState(
                connection = BridgeState.Connection.CONNECTED, hostName = "my-pc", sessionState = SessionState(state = "working"),
                chat = ChatState(messages = listOf(ChatMessage("1", "user", "Turn the volume down"))),
            ),
        )
        compose.onNodeWithText(text(R.string.session_working), substring = true).assertIsDisplayed()
        compose.onNodeWithContentDescription(text(R.string.chat_stop)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("chat_working", mode)
    }

    private val connected = BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc")

    @Test fun theTerminalStartsOpenByDefault() {
        show(connected, terminalOpenByDefault = true)
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
        check(short, text(R.string.card_waiting_title), "chat_offline_waiting", body = text(R.string.status_waiting))
    }

    @Test fun cutSaysSoInTheBanner() {
        show(BridgeState(connection = BridgeState.Connection.WAITING, hostName = "my-pc"), cut = true)
        compose.onNodeWithText(text(R.string.status_short_cut)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.chat_empty_ready)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.card_cut_title)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.card_cut_body)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(text(R.string.status_short_cut))
        compose.shot("chat_cut", mode)
    }

    @Test fun updateAppShowsTheWholeCommandInTheBanner() {
        show(BridgeState().appOutdated())
        val short = text(R.string.status_short_update)
        compose.onNodeWithText(text(R.string.chat_empty_ready)).assertDoesNotExist()
        check(short, text(R.string.card_update_app_title), "chat_update_app", body = text(R.string.status_update_app))
        // The banner is announced to TalkBack.
        compose.onNode(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Polite)).assertIsDisplayed()
    }

    @Test fun connectedAndEmptySaysWhereToWrite() {
        show(BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc"))
        compose.onNodeWithText(text(R.string.chat_empty_ready)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.shot("chat_connected_empty", mode)
    }

    @Test fun theEmptyChatShowsWhenPausedToo() {
        show(BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc", sessionState = SessionState(paused = true)))
        compose.onNodeWithText(text(R.string.chat_empty_paused)).assertIsDisplayed()
        // Paused: no suggestions (the agent cannot use the phone).
        compose.onNodeWithText(text(R.string.suggestion_battery)).assertDoesNotExist()
    }

    @Test fun aSuggestionFillsTheField_neverSends() {
        val sent = mutableListOf<String>()
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ChatScreen(
                    BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc"), outgoing = emptyList(), drafts = emptyList(),
                    downloads = emptyMap(), cut = false, terminal = null, snackbar = SnackbarHostState(),
                    actions = ChatActions({ sent += it }, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}),
                )
            }
        }
        compose.onNodeWithText(text(R.string.suggestion_battery)).performClick()
        compose.onNode(hasSetTextAction()).assert(hasText(text(R.string.suggestion_battery)))
        assertEquals("a suggestion must never send", emptyList<String>(), sent)
        compose.shot("chat_connected_empty_suggestions", mode)
    }

    // Task 8 left it noted: the empty state under the notifications card was cut at 200 % in Spanish. It scrolls now: every text is reachable whole.
    @Test fun theEmptyStateUnderACardShowsAllItsTextAtAnySize() {
        show(connected, notificationsBlocked = true)
        compose.onNodeWithText(text(R.string.notifications_blocked)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.chat_empty_ready)).performScrollTo().assertIsDisplayed()
        for (id in listOf(R.string.suggestion_battery, R.string.suggestion_volume, R.string.suggestion_screenshot)) {
            compose.onNodeWithText(text(id)).performScrollTo().assertIsDisplayed()
        }
        compose.assertTextsFit(scrolled = true)
        compose.shot("chat_empty_with_card", mode)
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
                    snackbar = SnackbarHostState(), actions = ChatActions.None, configured = true,
                )
            }
        }
        val waiting = text(R.string.card_waiting_title)
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
                    snackbar = SnackbarHostState(), actions = ChatActions.None, configured = true,
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
        bannerLeavesWhenConnecting(BridgeState().appOutdated(), false, false, text(R.string.card_update_app_title))

    @Test fun aPausedBannerDoesNotSurviveConnecting() = bannerLeavesWhenConnecting(
        BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc", sessionState = SessionState(paused = true)),
        false, false, text(R.string.card_paused_title),
    )

    @Test fun aCutBannerDoesNotSurviveTheCutEndingWhileConnecting() =
        bannerLeavesWhenConnecting(BridgeState(connection = BridgeState.Connection.WAITING, hostName = "my-pc"), true, false, text(R.string.card_cut_title))

    @Test fun aColdStartConnectingHasNoBanner() {
        show(BridgeState(connection = BridgeState.Connection.CONNECTING))
        compose.onNodeWithText(text(R.string.card_waiting_title)).assertDoesNotExist()
    }

    @Test fun notSetUpSaysToRunSetup() {
        show(BridgeState(), configured = false)
        check(text(R.string.status_short_not_set_up), text(R.string.card_not_set_up_title), "chat_not_set_up", body = text(R.string.status_not_set_up))
    }

    @Test fun theShieldAlwaysCutsWhenItSaysCut_evenWithACutRecordAndALiveChannel() {
        var cuts = 0
        var restores = 0
        compose.setContent {
            AdbuddyTheme {
                ChatScreen(
                    BridgeState(connection = BridgeState.Connection.CONNECTED, hostName = "my-pc"), outgoing = emptyList(), drafts = emptyList(),
                    downloads = emptyMap(), cut = true, terminal = null, snackbar = SnackbarHostState(),
                    actions = ChatActions.None.copyWith(cut = { cuts++ }, restore = { restores++ }),
                )
            }
        }
        compose.onNodeWithContentDescription(text(R.string.action_cut)).performClick()
        assertEquals(1, cuts)
        assertEquals(0, restores)
    }

    @Test fun givingAccessBackAsksFirst() {
        var restores = 0
        compose.setContent {
            AdbuddyTheme {
                ChatScreen(
                    BridgeState(connection = BridgeState.Connection.WAITING), outgoing = emptyList(), drafts = emptyList(),
                    downloads = emptyMap(), cut = true, terminal = null, snackbar = SnackbarHostState(),
                    actions = ChatActions.None.copyWith(restore = { restores++ }),
                )
            }
        }
        compose.onNodeWithContentDescription(text(R.string.action_restore)).performClick()
        assertEquals("the dialog asks first", 0, restores)
        compose.onNodeWithText(text(R.string.restore_confirm_title)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.restore_confirm_confirm)).performClick()
        assertEquals(1, restores)
    }

    @Test fun theCutCardButtonAlsoAsksFirst() {
        var restores = 0
        compose.setContent {
            AdbuddyTheme {
                ChatScreen(
                    BridgeState(connection = BridgeState.Connection.WAITING), outgoing = emptyList(), drafts = emptyList(),
                    downloads = emptyMap(), cut = true, terminal = null, snackbar = SnackbarHostState(),
                    actions = ChatActions.None.copyWith(restore = { restores++ }),
                )
            }
        }
        // The card's button is text; the header's shield is a content description: two different nodes.
        compose.onNodeWithText(text(R.string.action_restore)).performClick()
        assertEquals("the dialog asks first", 0, restores)
        compose.onNodeWithText(text(R.string.restore_confirm_title)).assertIsDisplayed()
    }

    @Test fun cancellingTheRestoreDialogRestoresNothing() {
        var restores = 0
        compose.setContent {
            AdbuddyTheme {
                ChatScreen(
                    BridgeState(connection = BridgeState.Connection.WAITING), outgoing = emptyList(), drafts = emptyList(),
                    downloads = emptyMap(), cut = true, terminal = null, snackbar = SnackbarHostState(),
                    actions = ChatActions.None.copyWith(restore = { restores++ }),
                )
            }
        }
        compose.onNodeWithContentDescription(text(R.string.action_restore)).performClick()
        compose.onNodeWithText(text(R.string.action_cancel)).performClick()
        compose.onNodeWithText(text(R.string.restore_confirm_title)).assertDoesNotExist()
        assertEquals(0, restores)
    }

    @Test fun theMenuAlwaysOffersCut_evenWhileCut() {
        var cuts = 0
        compose.setContent {
            AdbuddyTheme {
                ChatScreen(
                    BridgeState(connection = BridgeState.Connection.WAITING), outgoing = emptyList(), drafts = emptyList(),
                    downloads = emptyMap(), cut = true, terminal = null, snackbar = SnackbarHostState(),
                    actions = ChatActions.None.copyWith(cut = { cuts++ }),
                )
            }
        }
        compose.onNodeWithContentDescription(text(R.string.action_menu)).performClick()
        compose.onNodeWithText(text(R.string.action_cut)).performClick()
        assertEquals(1, cuts)
    }

    @Test fun theRestoreDialogClosesWhenTheCutEnds() {
        var cut by mutableStateOf(true)
        compose.setContent {
            AdbuddyTheme {
                ChatScreen(
                    BridgeState(connection = BridgeState.Connection.WAITING), outgoing = emptyList(), drafts = emptyList(),
                    downloads = emptyMap(), cut = cut, terminal = null, snackbar = SnackbarHostState(), actions = ChatActions.None,
                )
            }
        }
        compose.onNodeWithContentDescription(text(R.string.action_restore)).performClick()
        compose.onNodeWithText(text(R.string.restore_confirm_title)).assertIsDisplayed()
        cut = false // access came back some other way (Settings, the notification)
        compose.waitForIdle()
        compose.onNodeWithText(text(R.string.restore_confirm_title)).assertDoesNotExist()
        cut = true // a later cut must not bring the dialog back by itself
        compose.waitForIdle()
        compose.onNodeWithText(text(R.string.restore_confirm_title)).assertDoesNotExist()
    }

    @Test fun oneCardOnly_questionsBeatCutAndBlockedNotifications() {
        compose.setContent {
            AdbuddyTheme {
                ChatScreen(
                    BridgeState(connection = BridgeState.Connection.WAITING), outgoing = emptyList(), drafts = emptyList(),
                    downloads = emptyMap(), cut = true, terminal = null, snackbar = SnackbarHostState(), actions = ChatActions.None,
                    notificationsBlocked = true, pendingQuestions = 2,
                )
            }
        }
        compose.onNodeWithText(app.resources.getQuantityString(R.plurals.confirm_waiting, 2, 2)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.card_cut_title)).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.card_notifications_title)).assertDoesNotExist()
        compose.shot("chat_questions_card", mode)
    }

    // Migrated from NotificationsBannerScreensTest (deleted with its banner): whole text, an actionable button, announced politely.
    @Test fun theNotificationsCardSaysItAndOpensTheSettings() {
        var opened = 0
        // With a message: the empty state's text is Task 9's to make scrollable under a card at 200 %.
        val withMessage = connected.copy(chat = ChatState(messages = listOf(ChatMessage("1", "user", "Hi"))))
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ChatScreen(
                    withMessage, outgoing = emptyList(), drafts = emptyList(), downloads = emptyMap(), cut = false, terminal = null,
                    snackbar = SnackbarHostState(), actions = ChatActions.None, notificationsBlocked = true, onOpenNotificationSettings = { opened++ },
                )
            }
        }
        compose.onNodeWithText(text(R.string.notifications_blocked)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.notifications_open_settings)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.onNodeWithText(text(R.string.notifications_blocked)).assert(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Polite))
        compose.shot("chat_notifications_card", mode)
        compose.onNodeWithText(text(R.string.notifications_open_settings)).performClick()
        assertEquals(1, opened)
    }

    // Migrated from PendingQuestionsBannerScreensTest: whole text and an actionable "Show" that only reopens.
    @Test fun theQuestionsCardFitsAndOffersShow() {
        var shown = 0
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ChatScreen(
                    connected, outgoing = emptyList(), drafts = emptyList(), downloads = emptyMap(), cut = false, terminal = null,
                    snackbar = SnackbarHostState(), actions = ChatActions.None, pendingQuestions = 2, onShowQuestions = { shown++ },
                )
            }
        }
        compose.onNodeWithText(text(R.string.confirm_waiting_show)).assertIsDisplayed()
        compose.assertTextsFit()
        compose.onNodeWithText(text(R.string.confirm_waiting_show)).performClick()
        assertEquals(1, shown)
    }

    // Bug 9: the header never says "Ready" (nor any agent state) beside a pause or a cut; the pill carries the state.
    @Test fun theHeaderNeverSaysReadyWhilePaused() {
        show(connected.copy(sessionState = SessionState(paused = true, state = "idle")))
        compose.onNodeWithText(pill(text(R.string.status_short_paused))).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.session_idle), substring = true).assertDoesNotExist()
        compose.assertTextsFit()
        compose.assertTitleFitsTheTopBar(pill(text(R.string.status_short_paused)))
        compose.shot("chat_paused", mode)
    }

    @Test fun theHeaderNeverSaysReadyWhileCut() {
        show(BridgeState(connection = BridgeState.Connection.WAITING, hostName = "my-pc", sessionState = SessionState(state = "idle")), cut = true)
        compose.onNodeWithText(text(R.string.session_idle), substring = true).assertDoesNotExist()
        compose.onNodeWithText(text(R.string.status_short_cut)).assertIsDisplayed()
    }

    // The notifications card beats the paused one (R8), but the way to resume stays in the header's menu.
    @Test fun pausedWithBlockedNotificationsCanStillBeResumedFromTheMenu() {
        val resumes = mutableListOf<Boolean>()
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ChatScreen(
                    connected.copy(sessionState = SessionState(paused = true)), outgoing = emptyList(), drafts = emptyList(), downloads = emptyMap(),
                    cut = false, terminal = null, snackbar = SnackbarHostState(),
                    actions = ChatActions({}, {}, {}, {}, {}, {}, {}, {}, {}, { resumes += it }, {}, {}, {}, {}, {}, {}),
                    notificationsBlocked = true,
                )
            }
        }
        compose.onNodeWithText(text(R.string.card_notifications_title)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.card_paused_title)).assertDoesNotExist()
        compose.onNodeWithContentDescription(text(R.string.action_menu)).performClick()
        compose.onNodeWithText(text(R.string.action_resume)).performClick()
        assertEquals(listOf(false), resumes)
    }

    @Test fun thePausedCardResumes() {
        val resumes = mutableListOf<Boolean>()
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ChatScreen(
                    connected.copy(sessionState = SessionState(paused = true)), outgoing = emptyList(), drafts = emptyList(), downloads = emptyMap(),
                    cut = false, terminal = null, snackbar = SnackbarHostState(),
                    actions = ChatActions({}, {}, {}, {}, {}, {}, {}, {}, {}, { resumes += it }, {}, {}, {}, {}, {}, {}),
                )
            }
        }
        compose.onNodeWithText(text(R.string.card_paused_title)).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.action_resume)).performClick()
        assertEquals(listOf(false), resumes)
    }

    // R8: "How to connect" opens the fixed https guide, nothing else.
    @Test fun theWaitingCardOpensTheSetupGuide() {
        show(BridgeState())
        compose.onNodeWithText(text(R.string.card_waiting_action)).performClick()
        val started: Intent? = shadowOf(app).nextStartedActivity
        assertNotNull("no activity started", started)
        assertEquals(Intent.ACTION_VIEW, started!!.action)
        assertEquals("https://hybriqtech.github.io/adbuddy/setup/", started.data.toString())
    }

    // R9: no remote-mode card in the chat; only the "Remote" label of the pill, on (or unknown) and never on a known off.
    private fun remoteLabel(remote: RemoteState?, expected: Boolean) {
        compose.setContent {
            AdbuddyTheme(dynamic = false) {
                ChatScreen(
                    connected.copy(remote = remote), outgoing = emptyList(), drafts = emptyList(), downloads = emptyMap(), cut = false,
                    terminal = null, snackbar = SnackbarHostState(), actions = ChatActions.None,
                )
            }
        }
        val base = text(R.string.status_connected, "my-pc")
        if (expected) compose.onNodeWithText(pill(base)).assertIsDisplayed() else compose.onNodeWithText(pill(base)).assertDoesNotExist()
        if (!expected) compose.onNodeWithText(base).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.remote_title)).assertDoesNotExist()
    }

    @Test fun thePillSaysRemoteWhenItIsOn() = remoteLabel(RemoteState(enabled = true), true)
    @Test fun thePillSaysRemoteWhenItIsUnknown() = remoteLabel(null, true)
    @Test fun thePillDoesNotSayRemoteWhenItIsKnownOff() = remoteLabel(RemoteState(enabled = false), false)

    // The pill says "Remote" when the mode is unknown (a state without a remote reading): its text, for the checks of this file.
    private fun pill(base: String) = text(R.string.status_pill_remote, base)

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun modes() = screenModes()
    }
}
