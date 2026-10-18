package tech.hybriq.adbuddy.bridge

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ChatReducerYoloTest {
    private fun msg(seq: Long, payload: String) =
        Protocol.decode("""{"v":1,"seq":$seq,"type":"${Protocol.PERMISSION_RESOLVED}","ts":"x","payload":$payload}""")!!

    @Test fun whatYoloApprovedIsKeptForTheChat() {
        val s = BridgeState(session = "s_1").apply(msg(1, """{"id":"p_9","decision":"allow","by":"yolo","summary":"Run npm test"}"""))
        assertEquals(listOf("Run npm test"), s.chat.yoloApprovals.map { it.summary })
    }

    // Mutation: drop the `r.by == "yolo"` condition.
    @Test fun anAnswerOfThePersonIsNotAnApprovalOfYolo() {
        val s = BridgeState(session = "s_1").apply(msg(1, """{"id":"p_9","decision":"allow","by":"phone","summary":"x"}"""))
        assertTrue(s.chat.yoloApprovals.isEmpty())
    }

    @Test fun theApprovalsAreBoundedAndAnewSessionClearsThem() {
        var s = BridgeState(session = "s_1")
        for (i in 1..30) s = s.apply(msg(i.toLong(), """{"id":"p_$i","decision":"allow","by":"yolo","summary":"cmd $i"}"""))
        assertEquals(ChatReducer.MAX_YOLO_APPROVALS, s.chat.yoloApprovals.size)
        assertEquals("cmd 30", s.chat.yoloApprovals.last().summary)
        s = s.apply(Protocol.decode("""{"v":1,"type":"welcome","ts":"x","payload":{"session":"s_2","fromSeq":1,"protocol":1}}""")!!)
        assertTrue(s.chat.yoloApprovals.isEmpty())
    }

    @Test fun anApprovalOfOlderAppsHasNoSummary() {
        val s = BridgeState(session = "s_1").apply(msg(1, """{"id":"p_9","decision":"allow","by":"yolo"}"""))
        assertEquals("", s.chat.yoloApprovals.single().summary)
    }
}
