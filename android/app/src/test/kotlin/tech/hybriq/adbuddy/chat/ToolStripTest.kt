package tech.hybriq.adbuddy.chat

import org.junit.Assert.assertEquals
import org.junit.Test
import tech.hybriq.adbuddy.bridge.ChatState
import tech.hybriq.adbuddy.bridge.ToolActivity

class ToolStripTest {
    private fun t(i: Int, status: String) = ToolActivity("t$i", "m1", "Run $i", status)

    @Test fun showsTheLastFiveWhileItWorks() {
        val chat = ChatState(tools = (1..8).map { t(it, "done") })
        assertEquals((4..8).map { "t$it" }, ToolStrip.of(chat, "working").map { it.id })
    }

    @Test fun showsNothingWhenTheAgentIsNotWorking() {
        val chat = ChatState(tools = listOf(t(1, "done"), t(2, "running")))
        for (s in listOf("idle", "stopped", "", "whatever")) assertEquals(s, emptyList<ToolActivity>(), ToolStrip.of(chat, s))
    }

    @Test fun waitingWithAToolStillRunningIsAnAgentHeldOnAQuestion() {
        assertEquals(2, ToolStrip.of(ChatState(tools = listOf(t(1, "done"), t(2, "running"))), "waiting").size)
        assertEquals(emptyList<ToolActivity>(), ToolStrip.of(ChatState(tools = listOf(t(1, "done"))), "waiting"))
    }

    @Test fun emptyWhenThereIsNoActivity() = assertEquals(emptyList<ToolActivity>(), ToolStrip.of(ChatState(), "working"))
}
