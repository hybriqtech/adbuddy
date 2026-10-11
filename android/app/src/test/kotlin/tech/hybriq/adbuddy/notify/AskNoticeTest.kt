package tech.hybriq.adbuddy.notify

import android.app.Notification
import android.app.NotificationManager
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ChatState
import tech.hybriq.adbuddy.bridge.PermissionRequest
import tech.hybriq.adbuddy.bridge.SessionState
import tech.hybriq.adbuddy.confirm.ConfirmNotifier

/** The "the agent needs an answer" notice: when it is posted or withdrawn, and that what it shows is generic and has no buttons. */
@RunWith(RobolectricTestRunner::class)
class AskNoticeTest {
    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp
    private val nm get() = app.getSystemService(NotificationManager::class.java)

    @Before fun clean() = ConversationNotifier.cancelAsk(app)

    @After fun cleanAfter() = ConversationNotifier.cancelAsk(app)

    private fun q(id: String) = PermissionRequest(id, "Bash", "Clean the build folder", "SECRET-DETAIL", "2026-10-01T10:10:00Z", true)

    private fun state(vararg ids: String, connection: BridgeState.Connection = BridgeState.Connection.CONNECTED, paused: Boolean = false) =
        BridgeState(connection = connection, sessionState = SessionState(paused = paused), chat = ChatState(permissions = ids.map(::q)))

    private fun asks() = nm.activeNotifications.filter { it.id == ConversationNotifier.ASK_ID }

    private fun text(id: Int) = app.getString(id)

    @Test fun pendingIsTheQuestionsWhileConnected() {
        assertEquals(setOf("a"), AskNotice.pending(state("a")))
        assertEquals(emptySet<String>(), AskNotice.pending(state()))
        assertEquals(emptySet<String>(), AskNotice.pending(state("a", connection = BridgeState.Connection.WAITING)))
        // Unlike ConversationView.asking, a paused session still asks: the question expires all the same.
        assertEquals(setOf("a"), AskNotice.pending(state("a", paused = true)))
    }

    @Test fun theNoticeIsGenericAndHasNoButtons() {
        val announced = AskNotice.apply(app, emptySet(), state("a"), foreground = false)
        assertEquals(setOf("a"), announced)
        val n = asks().single().notification
        assertEquals(AdbuddyApp.CHANNEL_ASK, n.channelId)
        assertEquals(text(R.string.notif_ask_title), n.extras.getCharSequence(Notification.EXTRA_TITLE).toString())
        assertEquals(text(R.string.notif_ask_text), n.extras.getCharSequence(Notification.EXTRA_TEXT).toString())
        assertTrue("no buttons", n.actions == null || n.actions.isEmpty())
        assertTrue("stays on this phone", n.flags and Notification.FLAG_LOCAL_ONLY != 0)
        val pub = n.publicVersion
        assertNotNull(pub)
        assertEquals(text(R.string.app_name), pub.extras.getCharSequence(Notification.EXTRA_TITLE).toString())
        assertEquals(text(R.string.notif_ask_title), pub.extras.getCharSequence(Notification.EXTRA_TEXT).toString())

        val intent = shadowOf(n.contentIntent).savedIntent
        assertEquals(MainActivity::class.java.name, intent.component?.className)
        assertFalse("opening it does not focus a confirmation", intent.hasExtra(MainActivity.EXTRA_CONFIRM))

        for (x in listOf(n, pub)) {
            val all = listOf(Notification.EXTRA_TITLE, Notification.EXTRA_TEXT, Notification.EXTRA_BIG_TEXT, Notification.EXTRA_SUB_TEXT)
                .map { x.extras.getCharSequence(it)?.toString().orEmpty() } + x.tickerText?.toString().orEmpty()
            for (s in all) for (secret in listOf("Bash", "SECRET-DETAIL", "Clean the build folder")) assertFalse("$s leaks $secret", s.contains(secret))
        }
    }

    @Test fun theAppInFrontWithdrawsItAndTheBackgroundPostsItAgain() {
        var announced = AskNotice.apply(app, emptySet(), state("a"), foreground = false)
        assertEquals(1, asks().size)
        announced = AskNotice.apply(app, announced, state("a"), foreground = true)
        assertEquals(emptySet<String>(), announced)
        assertEquals(0, asks().size)
        announced = AskNotice.apply(app, announced, state("a"), foreground = false)
        assertEquals(setOf("a"), announced)
        assertEquals(1, asks().size)
    }

    @Test fun noQuestionsWithdrawsItAndASecondOneAsksAgain() {
        var announced = AskNotice.apply(app, emptySet(), state("a"), foreground = false)
        // The same question again does not post again; a second one does.
        nm.cancel(ConversationNotifier.ASK_ID)
        announced = AskNotice.apply(app, announced, state("a"), foreground = false)
        assertEquals("same question: not posted again", 0, asks().size)
        announced = AskNotice.apply(app, announced, state("a", "b"), foreground = false)
        assertEquals(setOf("a", "b"), announced)
        assertEquals("second question: posted again, still one notice", 1, asks().size)
        announced = AskNotice.apply(app, announced, state(), foreground = false)
        assertEquals(emptySet<String>(), announced)
        assertEquals(0, asks().size)
    }

    @Test fun theChannelsHaveTheirDescription() {
        val ask = nm.getNotificationChannel(AdbuddyApp.CHANNEL_ASK)
        assertEquals(NotificationManager.IMPORTANCE_HIGH, ask.importance)
        assertEquals(text(R.string.channel_ask_description), ask.description)
        assertEquals(text(R.string.channel_conversation_description), nm.getNotificationChannel(AdbuddyApp.CHANNEL_CONVERSATION).description)
        assertEquals(text(R.string.channel_done_description), nm.getNotificationChannel(AdbuddyApp.CHANNEL_DONE).description)
        assertEquals(text(R.string.channel_confirm_description), nm.getNotificationChannel(AdbuddyApp.CHANNEL_CONFIRM).description)
    }

    @Test fun confirmClearAndCancelAskDoNotTouchEachOther() {
        AskNotice.apply(app, emptySet(), state("a"), foreground = false)
        ConfirmNotifier.clear(app)
        assertEquals(1, asks().size)
        ConversationNotifier.cancelAsk(app)
        assertEquals(0, asks().size)
    }

    // BridgeService has no service test in the JVM suite; its two cancelAsk calls (onCreate, onDestroy) are checked by
    // reading. What it relies on: cancelAsk withdraws a notice an earlier process left, and is harmless when there is none.
    @Test fun cancelAskWithdrawsAnOrphanAndIsIdempotent() {
        ConversationNotifier.ask(app)
        assertEquals(1, asks().size)
        ConversationNotifier.cancelAsk(app)
        ConversationNotifier.cancelAsk(app)
        assertEquals(0, asks().size)
        assertNull(nm.activeNotifications.firstOrNull { it.id == ConversationNotifier.ASK_ID })
    }
}
