package tech.hybriq.adbuddy.confirm

import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
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

    private fun state(
        vararg pending: ConfirmRequest,
        answering: Set<String> = emptySet(),
        connection: BridgeState.Connection = BridgeState.Connection.CONNECTED,
    ) = BridgeState(connection = connection, pending = pending.toList(), answering = answering)

    private fun status(r: ConfirmRequest, s: BridgeState, at: Long = now) = ConfirmUi.status(r, s, at)

    // --- whether a question can be answered ---

    @Test fun aQuestionBeforeItsExpiryIsOpen() {
        assertEquals(ConfirmUi.Status.OPEN, status(q("j_1"), state(q("j_1"))))
    }

    @Test fun aQuestionPastItsExpiryIsExpired() {
        val r = q("j_1", expiresAt = "2026-09-30T10:00:00Z")
        assertEquals("at the second it expires", ConfirmUi.Status.EXPIRED, status(r, state(r)))
        assertEquals(ConfirmUi.Status.EXPIRED, status(q("j_1"), state(q("j_1")), now + 61_000))
    }

    @Test fun anUnreadableExpiryCannotBeAnswered() {
        val r = q("j_1", expiresAt = "soon")
        assertEquals(ConfirmUi.Status.EXPIRED, status(r, state(r)))
    }

    @Test fun anAnswerOnItsWayIsSendingEvenPastTheExpiry() {
        val s = state(q("j_1"), answering = setOf("j_1"))
        assertEquals(ConfirmUi.Status.SENDING, status(q("j_1"), s))
        assertEquals(ConfirmUi.Status.SENDING, status(q("j_1"), s, now + 61_000))
    }

    @Test fun withoutConnectionAQuestionCannotBeAnswered() {
        for (c in listOf(BridgeState.Connection.WAITING, BridgeState.Connection.CONNECTING, BridgeState.Connection.INCOMPATIBLE)) {
            assertEquals(c.name, ConfirmUi.Status.OFFLINE, status(q("j_1"), state(q("j_1"), connection = c)))
        }
    }

    @Test fun anExpiredQuestionIsExpiredEvenWithoutConnection() {
        val s = state(q("j_1"), connection = BridgeState.Connection.WAITING)
        assertEquals(ConfirmUi.Status.EXPIRED, status(q("j_1"), s, now + 61_000))
    }

    @Test fun reconnectingMakesItAnswerableAgain() {
        var s = state(q("j_1")).answered("j_1").disconnected()
        assertEquals(ConfirmUi.Status.OFFLINE, status(q("j_1"), s))
        s = s.copy(connection = BridgeState.Connection.CONNECTED)
        assertEquals(ConfirmUi.Status.OPEN, status(q("j_1"), s))
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

    @Test fun onlyAnOpenQuestionOffersButtons() {
        for (st in listOf(ConfirmUi.Status.SENDING, ConfirmUi.Status.OFFLINE, ConfirmUi.Status.EXPIRED)) {
            assertTrue(st.name, ConfirmUi.decisions(q("j_1", allowSession = true), st).isEmpty())
        }
    }

    // --- what the notification button may send ---

    @Test fun theReceiverSendsAnOpenQuestion() {
        val s = state(q("j_1", allowSession = true))
        for (d in listOf(Decision.DENY, Decision.ALLOW, Decision.ALLOW_SESSION)) {
            assertTrue(d, ConfirmUi.canSend(s, "j_1", d, now, locked = false))
        }
    }

    @Test fun aLockedPhoneCanDenyButNotAllow() {
        val s = state(q("j_1", allowSession = true))
        assertTrue(ConfirmUi.canSend(s, "j_1", Decision.DENY, now, locked = true))
        assertFalse(ConfirmUi.canSend(s, "j_1", Decision.ALLOW, now, locked = true))
        assertFalse(ConfirmUi.canSend(s, "j_1", Decision.ALLOW_SESSION, now, locked = true))
    }

    @Test fun theReceiverSendsNothingThatIsNotOpen() {
        assertFalse("gone", ConfirmUi.canSend(state(), "j_1", Decision.DENY, now, locked = false))
        assertFalse("sending", ConfirmUi.canSend(state(q("j_1"), answering = setOf("j_1")), "j_1", Decision.DENY, now, locked = false))
        assertFalse("expired", ConfirmUi.canSend(state(q("j_1")), "j_1", Decision.DENY, now + 61_000, locked = false))
        val offline = state(q("j_1"), connection = BridgeState.Connection.CONNECTING)
        assertFalse("offline", ConfirmUi.canSend(offline, "j_1", Decision.DENY, now, locked = false))
    }

    @Test fun theReceiverSendsOnlyKnownDecisions() {
        assertFalse(ConfirmUi.canSend(state(q("j_1")), "j_1", "allow_forever", now, locked = false))
        assertFalse("not offered", ConfirmUi.canSend(state(q("j_1")), "j_1", Decision.ALLOW_SESSION, now, locked = false))
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

    @Test fun whenNoneIsOpenTheDialogShowsOneSendingExpiredOrOffline() {
        assertEquals("j_1", ConfirmUi.question(state(q("j_1"), answering = setOf("j_1")), focus = null, now = now)?.id)
        val expired = state(q("j_1", expiresAt = "2026-09-30T09:59:00Z"))
        assertEquals("j_1", ConfirmUi.question(expired, focus = null, now = now)?.id)
        val offline = state(q("j_1"), connection = BridgeState.Connection.WAITING)
        assertEquals("j_1", ConfirmUi.question(offline, focus = null, now = now)?.id)
    }

    @Test fun aClosedQuestionIsNotShownAgainWhateverItsState() {
        val sending = state(q("j_1"), answering = setOf("j_1"))
        assertNull("answering, closed", ConfirmUi.question(sending, focus = null, now = now, closed = setOf("j_1")))
        val open = state(q("j_1"), q("j_2"))
        assertEquals("j_2", ConfirmUi.question(open, focus = "j_1", now = now, closed = setOf("j_1"))?.id)
    }

    @Test fun noQuestionNoDialog() {
        assertNull(ConfirmUi.question(state(), focus = "j_1", now = now))
    }

    // --- what the question says, rule first ---

    @Test fun theRuleComesBeforeTheAgentsCommand() {
        val r = ConfirmRequest(
            id = "j_1", title = "Restart", detail = "Reboots the phone.", rawCommand = "adb shell reboot",
            expiresAt = "2026-09-30T10:01:00Z", special = true,
        )
        assertEquals(
            listOf("Reboots the phone.", "⚠ May cut the link.", "adb shell reboot", "Expires at 10:01"),
            ConfirmUi.lines(r, special = "May cut the link.", footer = "Expires at 10:01"),
        )
    }

    @Test fun withoutDetailOrWarningOnlyTheCommandAndTheFooter() {
        assertEquals(listOf("adb shell reboot", "x"), ConfirmUi.lines(q("j_1"), special = "never shown", footer = "x"))
    }

    @Test fun aLongCommandIsCut() {
        val r = q("j_1").copy(rawCommand = "adb shell " + "a".repeat(1_000))
        val cmd = ConfirmUi.lines(r, special = "", footer = "x", maxCommand = 50)[0]
        assertEquals(50, cmd.length)
        assertTrue(cmd.endsWith("…"))
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
        assertEquals(ConfirmUi.Status.OPEN, p.post[0].status)
        assertEquals(60_000L, p.post[0].left)
        assertEquals(mapOf("j_1" to ConfirmUi.Status.OPEN), p.shown)
        assertTrue(p.cancel.isEmpty())
    }

    @Test fun theSameStateTwicePostsNothingAgain() {
        val first = ConfirmUi.plan(emptyMap(), state(q("j_1")), now)
        val again = ConfirmUi.plan(first.shown, state(q("j_1")), now + 1_000)
        assertTrue(again.post.isEmpty())
        assertTrue(again.cancel.isEmpty())
        assertEquals(first.shown, again.shown)
    }

    @Test fun anAnsweredQuestionIsPostedAsSendingThenOfflineThenAnswerableAgain() {
        var s = state(q("j_1"))
        val first = ConfirmUi.plan(emptyMap(), s, now)
        s = s.answered("j_1")
        val sending = ConfirmUi.plan(first.shown, s, now + 1_000)
        assertEquals(ConfirmUi.Status.SENDING, sending.post.single().status)
        assertEquals(59_000L, sending.post[0].left)
        s = s.disconnected()
        val offline = ConfirmUi.plan(sending.shown, s, now + 2_000)
        assertEquals("no buttons without the PC", ConfirmUi.Status.OFFLINE, offline.post.single().status)
        s = s.copy(connection = BridgeState.Connection.CONNECTED)
        val again = ConfirmUi.plan(offline.shown, s, now + 3_000)
        assertEquals("answerable again", ConfirmUi.Status.OPEN, again.post.single().status)
        assertEquals(mapOf("j_1" to ConfirmUi.Status.OPEN), again.shown)
    }

    @Test fun aQuestionAnsweredElsewhereIsCancelled() {
        val first = ConfirmUi.plan(emptyMap(), state(q("j_1"), q("j_2")), now)
        val p = ConfirmUi.plan(first.shown, state(q("j_2")), now)
        assertEquals(setOf("j_1"), p.cancel)
        assertTrue(p.post.isEmpty())
        assertEquals(mapOf("j_2" to ConfirmUi.Status.OPEN), p.shown)
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
