package tech.hybriq.adbuddy.confirm

import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.ConfirmRequest
import tech.hybriq.adbuddy.bridge.Decision

class ConfirmUiTest {
    private val now = Instant.parse("2026-09-30T10:00:00Z").toEpochMilli()
    private fun q(id: String, allowSession: Boolean = false, expiresAt: String = "2026-09-30T10:01:00Z") =
        ConfirmRequest(id = id, title = "Restart", rawCommand = "adb shell reboot", expiresAt = expiresAt, allowSession = allowSession)

    private fun state(vararg pending: ConfirmRequest, answering: Set<String> = emptySet()) =
        BridgeState(connection = BridgeState.Connection.CONNECTED, pending = pending.toList(), answering = answering)

    // --- whether a question can be answered ---

    @Test fun aQuestionBeforeItsExpiryIsOpen() {
        assertEquals(ConfirmUi.Status.OPEN, ConfirmUi.status(q("j_1"), answering = false, now = now))
    }

    @Test fun aQuestionPastItsExpiryIsExpired() {
        val r = q("j_1", expiresAt = "2026-09-30T10:00:00Z")
        assertEquals("at the second it expires", ConfirmUi.Status.EXPIRED, ConfirmUi.status(r, answering = false, now = now))
        assertEquals(ConfirmUi.Status.EXPIRED, ConfirmUi.status(q("j_1"), answering = false, now = now + 61_000))
    }

    @Test fun anUnreadableExpiryCannotBeAnswered() {
        assertEquals(ConfirmUi.Status.EXPIRED, ConfirmUi.status(q("j_1", expiresAt = "soon"), answering = false, now = now))
    }

    @Test fun anAnswerOnItsWayIsSendingEvenPastTheExpiry() {
        assertEquals(ConfirmUi.Status.SENDING, ConfirmUi.status(q("j_1"), answering = true, now = now))
        assertEquals(ConfirmUi.Status.SENDING, ConfirmUi.status(q("j_1"), answering = true, now = now + 61_000))
    }

    // --- buttons ---

    @Test fun aQuestionOffersDenyAndAllow() {
        assertEquals(listOf(Decision.DENY, Decision.ALLOW), ConfirmUi.decisions(q("j_1"), ConfirmUi.Status.OPEN))
    }

    @Test fun allowDuringTheSessionOnlyWhenTheHostOffersIt() {
        assertEquals(
            listOf(Decision.DENY, Decision.ALLOW, Decision.ALLOW_SESSION),
            ConfirmUi.decisions(q("j_1", allowSession = true), ConfirmUi.Status.OPEN),
        )
    }

    @Test fun aQuestionBeingSentOffersNoButtons() {
        assertTrue(ConfirmUi.decisions(q("j_1", allowSession = true), ConfirmUi.Status.SENDING).isEmpty())
    }

    @Test fun anExpiredQuestionOffersNoButtons() {
        assertTrue(ConfirmUi.decisions(q("j_1", allowSession = true), ConfirmUi.Status.EXPIRED).isEmpty())
    }

    // --- the dialog's question ---

    @Test fun theDialogShowsTheQuestionTheNotificationOpened() {
        val s = state(q("j_1"), q("j_2"))
        assertEquals("j_2", ConfirmUi.question(s, focus = "j_2", now = now)?.id)
        assertEquals("j_1", ConfirmUi.question(s, focus = null, now = now)?.id)
        assertEquals("gone: the first one", "j_1", ConfirmUi.question(s, focus = "j_9", now = now)?.id)
    }

    @Test fun theDialogPrefersAQuestionStillToAnswer() {
        val s = state(q("j_1"), q("j_2"), answering = setOf("j_1"))
        assertEquals("j_2", ConfirmUi.question(s, focus = null, now = now)?.id)
        assertEquals("the focused one is being sent", "j_2", ConfirmUi.question(s, focus = "j_1", now = now)?.id)
    }

    @Test fun theDialogSkipsExpiredQuestionsWhileAnotherIsOpen() {
        val s = state(q("j_1", expiresAt = "2026-09-30T09:59:00Z"), q("j_2"))
        assertEquals("j_2", ConfirmUi.question(s, focus = "j_1", now = now)?.id)
    }

    @Test fun whenNoneIsOpenTheDialogShowsOneSendingOrExpired() {
        assertEquals("j_1", ConfirmUi.question(state(q("j_1"), answering = setOf("j_1")), focus = null, now = now)?.id)
        val expired = state(q("j_1", expiresAt = "2026-09-30T09:59:00Z"))
        assertEquals("j_1", ConfirmUi.question(expired, focus = null, now = now)?.id)
    }

    @Test fun noQuestionNoDialog() {
        assertNull(ConfirmUi.question(state(), focus = "j_1", now = now))
    }

    // --- when the screen must look again ---

    @Test fun theNextExpiryIsTheSoonestStillToCome() {
        val pending = listOf(
            q("j_1", expiresAt = "2026-09-30T10:02:00Z"),
            q("j_2", expiresAt = "2026-09-30T10:00:30Z"),
            q("j_3", expiresAt = "2026-09-30T09:59:00Z"),
            q("j_4", expiresAt = "soon"),
        )
        assertEquals(30_000L, ConfirmUi.nextExpiry(pending, now))
    }

    @Test fun nothingToComeNothingToWaitFor() {
        assertNull(ConfirmUi.nextExpiry(listOf(q("j_3", expiresAt = "2026-09-30T09:59:00Z")), now))
        assertNull(ConfirmUi.nextExpiry(emptyList(), now))
    }

    // --- notifications ---

    @Test fun aNewQuestionIsPostedWithItsButtons() {
        val p = ConfirmUi.plan(emptyMap(), state(q("j_1")), now)
        assertEquals(listOf("j_1"), p.post.map { it.request.id })
        assertEquals(false, p.post[0].answering)
        assertEquals(60_000L, p.post[0].left)
        assertEquals(mapOf("j_1" to false), p.shown)
        assertTrue(p.cancel.isEmpty())
    }

    @Test fun theSameStateTwicePostsNothingAgain() {
        val first = ConfirmUi.plan(emptyMap(), state(q("j_1")), now)
        val again = ConfirmUi.plan(first.shown, state(q("j_1")), now + 1_000)
        assertTrue(again.post.isEmpty())
        assertTrue(again.cancel.isEmpty())
        assertEquals(first.shown, again.shown)
    }

    @Test fun anAnsweredQuestionIsPostedAgainAsSendingAndThenAnswerableWhenTheLinkDrops() {
        var s = state(q("j_1"))
        val first = ConfirmUi.plan(emptyMap(), s, now)
        s = s.answered("j_1")
        val sending = ConfirmUi.plan(first.shown, s, now + 1_000)
        assertEquals(listOf("j_1"), sending.post.map { it.request.id })
        assertEquals(true, sending.post[0].answering)
        assertEquals(59_000L, sending.post[0].left)
        s = s.disconnected()
        val again = ConfirmUi.plan(sending.shown, s, now + 2_000)
        assertEquals("answerable again", false, again.post.single().answering)
        assertEquals(mapOf("j_1" to false), again.shown)
    }

    @Test fun aQuestionAnsweredElsewhereIsCancelled() {
        val first = ConfirmUi.plan(emptyMap(), state(q("j_1"), q("j_2")), now)
        val p = ConfirmUi.plan(first.shown, state(q("j_2")), now)
        assertEquals(setOf("j_1"), p.cancel)
        assertTrue(p.post.isEmpty())
        assertEquals(mapOf("j_2" to false), p.shown)
    }

    @Test fun expiredOrUnreadableQuestionsAreNotPosted() {
        val p = ConfirmUi.plan(emptyMap(), state(q("j_1", expiresAt = "2026-09-30T09:59:59Z"), q("j_2", expiresAt = "soon")), now)
        assertTrue(p.post.isEmpty())
        assertTrue(p.shown.isEmpty())
    }

    @Test fun aShownQuestionThatExpiredIsNotPostedAgain() {
        val first = ConfirmUi.plan(emptyMap(), state(q("j_1")), now)
        val p = ConfirmUi.plan(first.shown, state(q("j_1"), answering = setOf("j_1")), now + 61_000)
        assertTrue(p.post.isEmpty())
        assertTrue("still tracked, so it is cancelled when it leaves", "j_1" in p.shown)
    }
}
