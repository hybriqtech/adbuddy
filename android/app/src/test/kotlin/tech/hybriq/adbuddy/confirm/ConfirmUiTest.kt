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
        for (d in listOf(Decision.DENY, Decision.ALLOW)) {
            assertTrue(d, ConfirmUi.canSend(s, "j_1", d, now, locked = false, armed = true, notification = true))
        }
    }

    // M2: allow during the session only from the dialog, never from the notification (collapsed, it shows only
    // the title and a line).
    @Test fun allowDuringTheSessionOnlyFromTheDialog() {
        val r = q("j_1", allowSession = true)
        assertEquals(listOf(Decision.DENY, Decision.ALLOW), ConfirmUi.notificationDecisions(r, ConfirmUi.Status.OPEN))
        val s = state(r)
        assertFalse(ConfirmUi.canSend(s, "j_1", Decision.ALLOW_SESSION, now, locked = false, armed = true, notification = true))
        assertTrue(ConfirmUi.canSend(s, "j_1", Decision.ALLOW_SESSION, now, locked = false, armed = true, notification = false))
    }

    @Test fun aLockedPhoneCanDenyButNotAllow() {
        val s = state(q("j_1", allowSession = true))
        assertTrue(ConfirmUi.canSend(s, "j_1", Decision.DENY, now, locked = true, armed = true, notification = true))
        assertFalse(ConfirmUi.canSend(s, "j_1", Decision.ALLOW, now, locked = true, armed = true, notification = true))
        assertFalse(ConfirmUi.canSend(s, "j_1", Decision.ALLOW_SESSION, now, locked = true, armed = true, notification = true))
    }

    @Test fun theReceiverSendsNothingThatIsNotOpen() {
        assertFalse("gone", ConfirmUi.canSend(state(), "j_1", Decision.DENY, now, locked = false, armed = true, notification = true))
        assertFalse("sending", ConfirmUi.canSend(state(q("j_1"), answering = setOf("j_1")), "j_1", Decision.DENY, now, locked = false, armed = true, notification = true))
        assertFalse("expired", ConfirmUi.canSend(state(q("j_1")), "j_1", Decision.DENY, now + 61_000, locked = false, armed = true, notification = true))
        val offline = state(q("j_1"), connection = BridgeState.Connection.CONNECTING)
        assertFalse("offline", ConfirmUi.canSend(offline, "j_1", Decision.DENY, now, locked = false, armed = true, notification = true))
    }

    @Test fun theReceiverSendsOnlyKnownDecisions() {
        assertFalse(ConfirmUi.canSend(state(q("j_1")), "j_1", "allow_forever", now, locked = false, armed = true, notification = true))
        assertFalse("not offered", ConfirmUi.canSend(state(q("j_1")), "j_1", Decision.ALLOW_SESSION, now, locked = false, armed = true, notification = true))
    }

    // --- the arming: Allow waits ConfirmUi.ARMING_MS from when it is shown, Deny never ---

    @Test fun allowWaitsSixHundredMillisecondsFromWhenItIsShown() {
        assertEquals(600L, ConfirmUi.ARMING_MS)
        val shown = 1_000_000L
        assertFalse("at once", ConfirmUi.armed(shown, shown))
        assertFalse("a quick burst", ConfirmUi.armed(shown, shown + 300))
        assertFalse("a millisecond short", ConfirmUi.armed(shown, shown + 599))
        assertTrue(ConfirmUi.armed(shown, shown + 600))
        assertTrue(ConfirmUi.armed(shown, shown + 60_000))
    }

    // Fail closed: a button nobody saw shown (a stale notification after the process restarted) is never armed.
    @Test fun aButtonNotKnownToBeShownIsNotArmed() {
        assertFalse(ConfirmUi.armed(null, 1_000_000L))
        assertFalse(ConfirmUi.armed(null, Long.MAX_VALUE))
    }

    @Test fun anUnarmedButtonAllowsNothingButDenyAlwaysWorks() {
        val s = state(q("j_1", allowSession = true))
        for (notification in listOf(true, false)) {
            assertFalse("allow $notification", ConfirmUi.canSend(s, "j_1", Decision.ALLOW, now, locked = false, armed = false, notification = notification))
            assertTrue("deny $notification", ConfirmUi.canSend(s, "j_1", Decision.DENY, now, locked = false, armed = false, notification = notification))
            assertTrue("deny locked $notification", ConfirmUi.canSend(s, "j_1", Decision.DENY, now, locked = true, armed = false, notification = notification))
        }
        assertFalse(ConfirmUi.canSend(s, "j_1", Decision.ALLOW_SESSION, now, locked = false, armed = false, notification = false))
        assertTrue("armed, the dialog allows for the session", ConfirmUi.canSend(s, "j_1", Decision.ALLOW_SESSION, now, locked = false, armed = true, notification = false))
        assertTrue("armed, the notification allows", ConfirmUi.canSend(s, "j_1", Decision.ALLOW, now, locked = false, armed = true, notification = true))
        assertFalse("armed but locked", ConfirmUi.canSend(s, "j_1", Decision.ALLOW, now, locked = true, armed = true, notification = true))
    }

    // Armed or not, a question that is not open sends nothing, not even Deny.
    @Test fun theArmingNeverOpensWhatIsNotOpen() {
        val offline = state(q("j_1"), connection = BridgeState.Connection.CONNECTING)
        for (armed in listOf(true, false)) {
            assertFalse("gone $armed", ConfirmUi.canSend(state(), "j_1", Decision.DENY, now, locked = false, armed = armed, notification = false))
            assertFalse("expired $armed", ConfirmUi.canSend(state(q("j_1")), "j_1", Decision.ALLOW, now + 61_000, locked = false, armed = armed, notification = false))
            assertFalse("offline $armed", ConfirmUi.canSend(offline, "j_1", Decision.DENY, now, locked = false, armed = armed, notification = false))
        }
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

    private val nl = Char(10).toString()
    private fun more(n: Int) = "and $n more"
    private fun notif(r: ConfirmRequest, special: String = "May cut the link.", footer: String = "x", open: String? = null, ltr: Boolean = false) =
        ConfirmUi.notificationLines(r, special, footer, ::more, open, ltr)

    // A4-3: the ⚠ warning goes first, before the detail the agent can make long.
    @Test fun theWarningComesFirstThenTheRuleThenTheAgentsCommand() {
        val r = ConfirmRequest(
            id = "j_1", title = "Restart", detail = "Reboots the phone.", rawCommand = "adb shell reboot",
            expiresAt = "2026-09-30T10:01:00Z", special = true,
        )
        assertEquals(
            listOf("⚠ May cut the link.", "Reboots the phone.", "adb shell reboot", "Expires at 10:01"),
            notif(r, footer = "Expires at 10:01"),
        )
    }

    @Test fun withoutDetailOrWarningOnlyTheCommandAndTheFooter() {
        assertEquals(listOf("adb shell reboot", "x"), notif(q("j_1"), special = "never shown"))
    }

    @Test fun theNotificationsCommandAndDetailStartWithALeftToRightMarkButTheLabelsDoNot() {
        val r = ConfirmRequest(
            id = "j_1", title = "Restart", detail = "Reboots the phone.${nl}Deletes א", rawCommand = "א reboot",
            expiresAt = "2026-09-30T10:01:00Z", special = true,
        )
        assertEquals(
            listOf("⚠ May cut the link.", "‎Reboots the phone.", "‎Deletes א", "‎א reboot", "Open it", "Expires at 10:01"),
            notif(r, footer = "Expires at 10:01", open = "Open it", ltr = true),
        )
        assertEquals(listOf("‎א reboot", "x"), notif(r.copy(detail = "", special = false), special = "", ltr = true))
    }

    // A4-3: the detail of the notification is at most three lines, and says how many more.
    @Test fun theNotificationsDetailIsCappedAndSaysHowManyMore() {
        val detail = (1..15).joinToString(nl) { "Finding $it" }
        val r = q("j_1").copy(detail = detail, special = true)
        assertEquals(
            listOf("⚠ May cut the link.", "Finding 1", "Finding 2", "Finding 3", "and 12 more", "adb shell reboot", "Open it", "x"),
            notif(r, open = "Open it"),
        )
        assertEquals(15, ConfirmUi.shown(r).detail.size) // the dialog shows them all
    }

    @Test fun aLongCommandIsCutInTheNotification() {
        val r = q("j_1").copy(rawCommand = "adb shell " + "a".repeat(1_000))
        val cmd = notif(r, special = "")[0]
        assertEquals(ConfirmUi.MAX_COMMAND, cmd.length)
        assertTrue(cmd.endsWith("…"))
        assertTrue(ConfirmUi.cut(r))
        assertFalse(ConfirmUi.cut(q("j_1")))
    }

    // A4-2: the dialog shows much more, and says how much it still hides.
    @Test fun theDialogShowsTheWholeCommandOrSaysHowMuchIsHidden() {
        val fits = q("j_1").copy(rawCommand = "a".repeat(ConfirmUi.DIALOG_MAX_COMMAND))
        assertEquals(fits.rawCommand, ConfirmUi.shown(fits).command)
        assertEquals(0, ConfirmUi.shown(fits).hidden)
        val long = q("j_1").copy(rawCommand = "a".repeat(ConfirmUi.DIALOG_MAX_COMMAND + 500))
        val s = ConfirmUi.shown(long)
        assertEquals(ConfirmUi.DIALOG_MAX_COMMAND, s.command.length)
        assertTrue(s.command.endsWith("…"))
        assertEquals(501, s.hidden)
    }

    // H3: past what the dialog shows, the person cannot have read the whole command: it can only be denied, and
    // the notification and the dialog say why.
    @Test fun aCommandTheDialogCannotShowWholeCanOnlyBeDenied() {
        val long = q("j_1", allowSession = true).copy(rawCommand = "a".repeat(ConfirmUi.DIALOG_MAX_COMMAND + 1))
        assertTrue(ConfirmUi.shown(long).hidden > 0)
        assertEquals(listOf(Decision.DENY), ConfirmUi.decisions(long, ConfirmUi.Status.OPEN))
        assertEquals(listOf(Decision.DENY), ConfirmUi.notificationDecisions(long, ConfirmUi.Status.OPEN))
        val s = state(long)
        assertFalse(ConfirmUi.canSend(s, "j_1", Decision.ALLOW, now, locked = false, armed = true, notification = false))
        assertFalse(ConfirmUi.canSend(s, "j_1", Decision.ALLOW_SESSION, now, locked = false, armed = true, notification = false))
        assertTrue(ConfirmUi.canSend(s, "j_1", Decision.DENY, now, locked = false, armed = true, notification = false))
        assertEquals(ConfirmUi.Why.TOO_LONG, ConfirmUi.why(long, ConfirmUi.Status.OPEN))
        val fits = q("j_2").copy(rawCommand = "a".repeat(ConfirmUi.DIALOG_MAX_COMMAND))
        assertEquals(listOf(Decision.DENY, Decision.ALLOW), ConfirmUi.decisions(fits, ConfirmUi.Status.OPEN))
    }

    @Test fun theNotificationSaysWhyItDoesNotAllow() {
        assertEquals(ConfirmUi.Why.SPECIAL, ConfirmUi.why(q("j_1").copy(special = true), ConfirmUi.Status.OPEN))
        assertEquals(ConfirmUi.Why.CUT, ConfirmUi.why(q("j_1").copy(rawCommand = "x".repeat(ConfirmUi.MAX_COMMAND + 1)), ConfirmUi.Status.OPEN))
        assertNull(ConfirmUi.why(q("j_1"), ConfirmUi.Status.OPEN))
        assertNull(ConfirmUi.why(q("j_1").copy(special = true), ConfirmUi.Status.EXPIRED))
    }

    // B2: the cut never splits a character outside the BMP (two UTF-16 units) in two.
    @Test fun theCutDoesNotSplitASurrogatePair() {
        val emoji = String(Character.toChars(0x1F600))
        for (max in listOf(ConfirmUi.MAX_COMMAND, ConfirmUi.DIALOG_MAX_COMMAND)) {
            val r = q("j_1").copy(rawCommand = "a".repeat(max - 2) + emoji + "b".repeat(10))
            val s = ConfirmUi.shown(r, maxCommand = max)
            val kept = s.command.removeSuffix("…")
            assertFalse("$max", Character.isHighSurrogate(kept.last()))
            assertEquals("$max", r.rawCommand.length, kept.length + s.hidden)
        }
    }

    // --- what the notification itself may allow ---

    // A4-2/A4-3: a special question, or a command the notification cuts, is
    // not allowed from the notification: only Deny, and the dialog to read it all.
    @Test fun aSpecialOrCutQuestionOffersOnlyDenyInTheNotification() {
        val special = q("j_1").copy(special = true)
        val cut = q("j_2", allowSession = true).copy(rawCommand = "adb shell " + " ".repeat(400) + "reboot")
        for (r in listOf(special, cut)) {
            assertEquals(r.id, listOf(Decision.DENY), ConfirmUi.notificationDecisions(r, ConfirmUi.Status.OPEN))
        }
        assertEquals(
            listOf(Decision.DENY, Decision.ALLOW),
            ConfirmUi.notificationDecisions(q("j_3", allowSession = true), ConfirmUi.Status.OPEN),
        )
        assertTrue(ConfirmUi.notificationDecisions(special, ConfirmUi.Status.EXPIRED).isEmpty())
    }

    // A4-2: a command longer than the notification shows is never allowed for the session.
    @Test fun aCutCommandIsNeverAllowedForTheSession() {
        val cut = q("j_1", allowSession = true).copy(rawCommand = "adb shell " + "a".repeat(ConfirmUi.MAX_COMMAND))
        assertEquals(listOf(Decision.DENY, Decision.ALLOW), ConfirmUi.decisions(cut, ConfirmUi.Status.OPEN))
    }

    @Test fun theNotificationButtonsCannotSendWhatTheNotificationDoesNotOffer() {
        val s = state(q("j_1").copy(special = true), q("j_2", allowSession = true).copy(rawCommand = "x".repeat(ConfirmUi.MAX_COMMAND + 1)))
        assertFalse(ConfirmUi.canSend(s, "j_1", Decision.ALLOW, now, locked = false, armed = true, notification = true))
        assertTrue(ConfirmUi.canSend(s, "j_1", Decision.DENY, now, locked = false, armed = true, notification = true))
        assertTrue("the dialog can", ConfirmUi.canSend(s, "j_1", Decision.ALLOW, now, locked = false, armed = true, notification = false))
        assertFalse(ConfirmUi.canSend(s, "j_2", Decision.ALLOW, now, locked = false, armed = true, notification = true))
        assertFalse(ConfirmUi.canSend(s, "j_2", Decision.ALLOW_SESSION, now, locked = false, armed = true, notification = false))
        assertTrue(ConfirmUi.canSend(s, "j_2", Decision.ALLOW, now, locked = false, armed = true, notification = false))
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

    // --- a change of language repaints what is on screen ---

    @Test fun repaintPostsAgainEveryShownQuestionWithTheSameIdsAndStatuses() {
        val s = state(q("j_1"), q("j_2")).answered("j_2")
        val first = ConfirmUi.plan(emptyMap(), s, now)
        val p = ConfirmUi.repaint(first.shown, s, now + 5_000)
        assertEquals(listOf("j_1", "j_2"), p.post.map { it.request.id })
        assertEquals(listOf(ConfirmUi.Status.OPEN, ConfirmUi.Status.SENDING), p.post.map { it.status })
        assertEquals(first.shown, p.shown)
        assertTrue(p.cancel.isEmpty())
        assertEquals(55_000L, p.post[0].left)
    }

    @Test fun repaintDoesNotPostWhatWasNotShownNorWhatExpired() {
        val s = state(q("j_1"), q("j_2"))
        val p = ConfirmUi.repaint(mapOf("j_1" to ConfirmUi.Status.OPEN), s, now)
        assertEquals(listOf("j_1"), p.post.map { it.request.id })
        val late = ConfirmUi.repaint(mapOf("j_1" to ConfirmUi.Status.OPEN), s, now + 61_000)
        assertTrue(late.post.isEmpty())
    }

    @Test fun repaintCancelsWhatIsNoLongerPending() {
        val p = ConfirmUi.repaint(mapOf("j_1" to ConfirmUi.Status.OPEN, "j_9" to ConfirmUi.Status.OPEN), state(q("j_1")), now)
        assertEquals(setOf("j_9"), p.cancel)
        assertEquals(mapOf("j_1" to ConfirmUi.Status.OPEN), p.shown)
    }
}
