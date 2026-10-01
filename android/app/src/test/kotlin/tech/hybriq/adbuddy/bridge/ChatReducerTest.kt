package tech.hybriq.adbuddy.bridge

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ChatReducerTest {
    private fun msg(seq: Long, type: String, payload: String) =
        Protocol.decode("""{"v":1,"seq":$seq,"type":"$type","ts":"x","payload":$payload}""")!!

    private fun chat(id: String, text: String, extra: String = "") =
        """{"id":"$id","role":"user","text":"$text","conversation":"c_1"$extra}"""

    @Test fun messagesAreUpsertedById() {
        var s = BridgeState()
        s = s.apply(msg(1, Protocol.CHAT_MESSAGE, chat("a", "one")))
        s = s.apply(msg(2, Protocol.CHAT_MESSAGE, chat("b", "two")))
        s = s.apply(msg(3, Protocol.CHAT_MESSAGE, chat("a", "one, edited")))
        assertEquals(listOf("a", "b"), s.chat.messages.map { it.id })
        assertEquals("one, edited", s.chat.messages[0].text)
        assertEquals(3L, s.lastSeq)
    }

    @Test fun onlyTheLast100AreKept() {
        var s = BridgeState()
        for (i in 1..130) s = s.apply(msg(i.toLong(), Protocol.CHAT_MESSAGE, chat("m$i", "x")))
        assertEquals(ChatReducer.MAX_MESSAGES, s.chat.messages.size)
        assertEquals("m31", s.chat.messages.first().id)
        assertEquals("m130", s.chat.messages.last().id)
    }

    @Test fun nullAttachmentsAndBadPayloadsNeverThrow() {
        var s = BridgeState()
        s = s.apply(msg(1, Protocol.CHAT_MESSAGE, chat("a", "x", ""","attachments":null""")))
        assertTrue(s.chat.messages[0].attachments.isEmpty())
        s = s.apply(msg(2, Protocol.CHAT_MESSAGE, """{"id":5}"""))
        s = s.apply(msg(3, Protocol.CHAT_MESSAGE, """{"role":"user"}"""))
        s = s.apply(msg(4, Protocol.CHAT_MESSAGE, "[]"))
        assertEquals(listOf("a"), s.chat.messages.map { it.id })
        assertEquals(4L, s.lastSeq)
    }

    @Test fun snapshotReplacesTheMessagesAndSkipsTheBadOnes() {
        var s = BridgeState(session = "s_1", lastSeq = 5)
        s = s.apply(msg(6, Protocol.CHAT_MESSAGE, chat("old", "old")))
        val snap = """{"state":{},"chat":[${chat("a", "one")},{"id":7},${chat("b", "two")},${chat("a", "one again")}]}"""
        s = s.apply(msg(7, Protocol.SNAPSHOT, snap))
        assertEquals(listOf("b", "a"), s.chat.messages.map { it.id })
        assertEquals("one again", s.chat.messages.last().text)
    }

    @Test fun aNewSessionForgetsTheChatButAReconnectKeepsIt() {
        var s = BridgeState(session = "s_1", lastSeq = 5).apply(msg(6, Protocol.CHAT_MESSAGE, chat("a", "x")))
        assertEquals(1, s.disconnected().chat.messages.size)
        s = s.apply(msg(0, Protocol.WELCOME, """{"session":"s_2","fromSeq":1,"protocol":1}"""))
        assertTrue(s.chat.messages.isEmpty())
    }
}

class ChatReducerM4Test {
    private fun msg(seq: Long, type: String, payload: String) =
        Protocol.decode("""{"v":1,"seq":$seq,"type":"$type","ts":"x","payload":$payload}""")!!

    private fun tool(id: String, status: String, summary: String = "Run ls") =
        """{"id":"$id","messageId":"m1","summary":"$summary","status":"$status"}"""

    private fun perm(id: String, extra: String = "") =
        """{"id":"$id","tool":"Bash","summary":"touch x","detail":"touch x","expiresAt":"2026-10-01T10:10:00Z","allowAlways":true$extra}"""

    @Test fun toolActivityIsUpsertedById() {
        var s = BridgeState()
        s = s.apply(msg(1, Protocol.TOOL_ACTIVITY, tool("t1", "running")))
        s = s.apply(msg(2, Protocol.TOOL_ACTIVITY, tool("t2", "running", "Read a.kt")))
        s = s.apply(msg(3, Protocol.TOOL_ACTIVITY, tool("t1", "done")))
        assertEquals(listOf("t1", "t2"), s.chat.tools.map { it.id })
        assertEquals("done", s.chat.tools[0].status)
        assertEquals(3L, s.lastSeq)
    }

    @Test fun onlyTheLast200ToolsAreKept() {
        var s = BridgeState()
        for (i in 1..230) s = s.apply(msg(i.toLong(), Protocol.TOOL_ACTIVITY, tool("t$i", "done")))
        assertEquals(ChatReducer.MAX_TOOLS, s.chat.tools.size)
        assertEquals("t31", s.chat.tools.first().id)
    }

    @Test fun aPermissionStaysUntilItIsResolvedWhoeverAnswers() {
        var s = BridgeState()
        s = s.apply(msg(1, Protocol.PERMISSION_REQUEST, perm("p1")))
        s = s.apply(msg(2, Protocol.PERMISSION_REQUEST, perm("p2")))
        s = s.apply(msg(3, Protocol.PERMISSION_REQUEST, perm("p1", ""","detail":"again"""")))
        assertEquals(listOf("p1", "p2"), s.chat.permissions.map { it.id })
        assertEquals("again", s.chat.permissions[0].detail)
        for (by in listOf("phone", "terminal", "timeout", "agent")) {
            val r = s.apply(msg(4, Protocol.PERMISSION_RESOLVED, """{"id":"p1","decision":"deny","by":"$by"}"""))
            assertEquals(listOf("p2"), r.chat.permissions.map { it.id })
        }
        // an id nobody knows changes nothing
        assertEquals(2, s.apply(msg(4, Protocol.PERMISSION_RESOLVED, """{"id":"zz","decision":"allow","by":"phone"}""")).chat.permissions.size)
    }

    @Test fun badPayloadsNeverThrow() {
        var s = BridgeState()
        s = s.apply(msg(1, Protocol.TOOL_ACTIVITY, """{"id":5}"""))
        s = s.apply(msg(2, Protocol.TOOL_ACTIVITY, "[]"))
        s = s.apply(msg(3, Protocol.TOOL_ACTIVITY, """{"id":"","summary":"x","status":"done"}"""))
        s = s.apply(msg(4, Protocol.PERMISSION_REQUEST, """{"tool":"Bash"}"""))
        s = s.apply(msg(5, Protocol.PERMISSION_REQUEST, "null"))
        s = s.apply(msg(6, Protocol.PERMISSION_RESOLVED, """{"id":1}"""))
        assertTrue(s.chat.tools.isEmpty() && s.chat.permissions.isEmpty())
        assertEquals(6L, s.lastSeq)
    }

    @Test fun theSnapshotReplacesThePermissionsAndSkipsTheBadOnes() {
        var s = BridgeState(session = "s_1", lastSeq = 5).apply(msg(6, Protocol.PERMISSION_REQUEST, perm("old")))
        val snap = """{"state":{},"permissions":[${perm("a")},{"id":7},${perm("b")},${perm("a", ""","summary":"newer"""")}]}"""
        s = s.apply(msg(7, Protocol.SNAPSHOT, snap))
        assertEquals(setOf("a", "b"), s.chat.permissions.map { it.id }.toSet())
        assertEquals(2, s.chat.permissions.size)
        assertTrue(s.chat.permissions.none { it.id == "old" })
        // a repeated id: the last wins, at the place of its last appearance (like the messages)
        assertEquals(listOf("b", "a"), s.chat.permissions.map { it.id })
        assertEquals("newer", s.chat.permissions.last().summary)
        // a snapshot with none empties them (all were answered)
        s = s.apply(msg(8, Protocol.SNAPSHOT, """{"state":{},"permissions":[]}"""))
        assertTrue(s.chat.permissions.isEmpty())
    }

    @Test fun aNewSessionForgetsThemButAReconnectKeepsThem() {
        val s = BridgeState(session = "s_1", lastSeq = 5)
            .apply(msg(6, Protocol.PERMISSION_REQUEST, perm("p1")))
            .apply(msg(7, Protocol.TOOL_ACTIVITY, tool("t1", "running")))
        assertEquals(1, s.disconnected().chat.permissions.size)
        val n = s.apply(msg(0, Protocol.WELCOME, """{"session":"s_2","fromSeq":1,"protocol":1}"""))
        assertTrue(n.chat.permissions.isEmpty() && n.chat.tools.isEmpty())
    }

    @Test fun aFinishedToolNeverGoesBackToRunning() {
        var s = BridgeState().apply(msg(1, Protocol.TOOL_ACTIVITY, tool("t1", "running")))
        s = s.apply(msg(2, Protocol.TOOL_ACTIVITY, tool("t1", "done")))
        s = s.apply(msg(3, Protocol.TOOL_ACTIVITY, tool("t1", "running", "late")))
        assertEquals("done", s.chat.tools.single().status)
        assertEquals("Run ls", s.chat.tools.single().summary)
        s = s.apply(msg(4, Protocol.TOOL_ACTIVITY, tool("t2", "running")))
            .apply(msg(5, Protocol.TOOL_ACTIVITY, tool("t2", "error")))
            .apply(msg(6, Protocol.TOOL_ACTIVITY, tool("t2", "running")))
        assertEquals("error", s.chat.tools.single { it.id == "t2" }.status)
        assertEquals(6L, s.lastSeq)
        // running to running, and running to a final one, still update
        s = s.apply(msg(7, Protocol.TOOL_ACTIVITY, tool("t3", "running")))
            .apply(msg(8, Protocol.TOOL_ACTIVITY, tool("t3", "running", "again")))
        assertEquals("again", s.chat.tools.single { it.id == "t3" }.summary)
    }
}
