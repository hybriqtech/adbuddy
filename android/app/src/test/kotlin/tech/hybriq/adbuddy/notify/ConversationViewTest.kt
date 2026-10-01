package tech.hybriq.adbuddy.notify

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.Attachment
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ChatMessage
import tech.hybriq.adbuddy.bridge.ChatState
import tech.hybriq.adbuddy.bridge.PermissionRequest
import tech.hybriq.adbuddy.bridge.SessionState

class ConversationViewTest {
    private fun msg(id: String, role: String, text: String, conv: String = "c_1", atts: List<Attachment> = emptyList()) =
        ChatMessage(id, role, text, atts, conv)

    private fun state(st: String = "working", activity: String = "Open Settings", msgs: List<ChatMessage> = emptyList(),
                      conn: BridgeState.Connection = BridgeState.Connection.CONNECTED) =
        BridgeState(connection = conn, hostName = "PC", sessionState = SessionState(agent = "claude", state = st, activity = activity, conversation = "c_1"),
            chat = ChatState(messages = msgs))

    @Test fun workingShowsWhatTheAgentIsDoing() {
        val v = ConversationView.of(state(), cut = false)
        assertEquals(Headline(R.string.notif_working_on, "Open Settings"), v.headline())
        assertEquals(Headline(R.string.notif_working), ConversationView.of(state(activity = ""), cut = false).headline())
        assertEquals("activity is only news while working", "", ConversationView.of(state("waiting"), cut = false).activity)
        assertEquals(Headline(R.string.notif_waiting), ConversationView.of(state("waiting"), cut = false).headline())
        assertEquals(Headline(R.string.notif_stopped), ConversationView.of(state("stopped"), cut = false).headline())
    }

    @Test fun theLockScreenNeverShowsTheActivityNorTheHostName() {
        assertEquals(Headline(R.string.notif_working), ConversationView.of(state(), cut = false).publicHeadline())
        assertEquals(Headline(R.string.notif_public_connected), ConversationView.of(state("idle"), cut = false).publicHeadline())
        assertEquals(Headline(R.string.notif_waiting), ConversationView.of(state("waiting"), cut = false).publicHeadline())
        assertEquals(Headline(R.string.status_waiting), ConversationView.of(state(conn = BridgeState.Connection.WAITING), cut = false).publicHeadline())
    }

    @Test fun withoutAConnectionTheLinkStatusSpeaks() {
        val v = ConversationView.of(state(conn = BridgeState.Connection.WAITING), cut = false)
        assertEquals(Headline(R.string.status_waiting), v.headline())
        assertEquals(Headline(R.string.status_cut), ConversationView.of(state(), cut = true).headline())
    }

    @Test fun theLastMessageOfTheCurrentConversation() {
        val msgs = listOf(msg("a", "user", "hi"), msg("b", "assistant", "hello", conv = "c_0"), msg("c", "assistant", "done", conv = ""))
        val last = ConversationView.of(state(msgs = msgs), cut = false).last!!
        assertEquals(LastMessage("c", fromUser = false, text = "done"), last)
        assertNull(ConversationView.of(state(msgs = listOf(msg("x", "user", "old", conv = "c_0"))), cut = false).last)
    }

    @Test fun anAttachmentOnlyMessageShowsItsFileNames_andLongTextIsCut() {
        val a = Attachment("0".repeat(32), "photo.png", "image/png", 1, "x")
        assertEquals("photo.png", ConversationView.of(state(msgs = listOf(msg("a", "assistant", "", atts = listOf(a)))), cut = false).last!!.text)
        val long = ConversationView.of(state(msgs = listOf(msg("a", "assistant", "x".repeat(1000)))), cut = false).last!!.text
        assertEquals(ConversationView.MAX_TEXT, long.length)
        assertTrue(long.endsWith("…"))
    }

    @Test fun equalStatesGiveEqualViews_soTheNotificationIsNotPostedAgain() {
        assertEquals(ConversationView.of(state(), cut = false), ConversationView.of(state(), cut = false))
    }

    @Test fun doneLineIsTheStartOfTheLastAnswer() {
        val chat = ChatState(messages = listOf(msg("a", "assistant", "\n\n  Fixed the Wi-Fi toggle.\nMore detail"), msg("b", "user", "thanks")))
        assertEquals("Fixed the Wi-Fi toggle. More detail", Notify.doneLine(chat, "c_1"))
        assertNull(Notify.doneLine(ChatState(messages = listOf(msg("b", "user", "thanks"))), "c_1"))
        assertNull(Notify.doneLine(chat, "c_2"))
    }

    private fun done(text: String) = Notify.doneLine(ChatState(messages = listOf(msg("a", "assistant", text))), "c_1")

    @Test fun doneLineIsCutAtAWordBoundaryWithinAHundredLetters() {
        val d = done("word ".repeat(40))!!
        assertTrue(d.length <= 101)
        assertTrue(d.endsWith("…"))
        assertEquals("no half word", "word word", d.removeSuffix("…").takeLast(9))
        assertEquals(100, done("y".repeat(500))!!.removeSuffix("…").length)
        assertEquals("short stays whole", "short", done("short"))
        assertNull(done("   \n  "))
        assertNull(done(""))
    }

    @Test fun doneLineHasNoLineBreaksNorRawMarkdown() {
        assertEquals("Fixed it now and tested", done("**Fixed** it\n\n- now and\r\n`tested`"))
        assertEquals("See docs for more", done("# See [docs](https://x.y/z) for more"))
        assertEquals("code", done("```kotlin\ncode\n```"))
    }

    @Test fun doneLineNeverSplitsASurrogatePair() {
        val emoji = "😀"
        // The hundredth char would fall between the two halves of the emoji.
        val cut = done("a".repeat(99) + emoji + "b".repeat(20))!!.removeSuffix("…")
        assertTrue(cut.isEmpty() || !cut.last().isHighSurrogate())
        assertTrue(cut.all { !it.isSurrogate() })
        val whole = done("a".repeat(98) + emoji + "b".repeat(20))!!
        assertTrue(whole.contains(emoji))
    }

    private fun perm(id: String = "p1") = PermissionRequest(id, "Bash", "SECRET-SUMMARY rm -rf /", "SECRET-DETAIL", "2026-10-01T10:00:00Z")

    @Test fun aPendingPermissionIsAskingAndTheNoticeIsGenericWithoutItsData() {
        val s = state(msgs = listOf(msg("a", "assistant", "hello")))
        val v = ConversationView.of(s.copy(chat = s.chat.copy(permissions = listOf(perm()))), cut = false)
        assertTrue(v.asking)
        assertEquals(Headline(R.string.notif_asking), v.headline())
        assertEquals(Headline(R.string.notif_asking), v.publicHeadline())
        assertTrue(!v.toString().contains("SECRET"))
        assertTrue(!ConversationView.of(state(), cut = false).asking)
    }

    @Test fun askingBeatsWorkingWaitingAndStoppedButNotALostConnection() {
        for (st in listOf("working", "waiting", "idle", "stopped")) {
            val s = state(st)
            val v = ConversationView.of(s.copy(chat = s.chat.copy(permissions = listOf(perm()))), cut = false)
            assertEquals(st, Headline(R.string.notif_asking), v.headline())
        }
        val lost = state(conn = BridgeState.Connection.WAITING)
        assertEquals(Headline(R.string.status_waiting), ConversationView.of(lost.copy(chat = lost.chat.copy(permissions = listOf(perm()))), cut = false).headline())
    }
}
