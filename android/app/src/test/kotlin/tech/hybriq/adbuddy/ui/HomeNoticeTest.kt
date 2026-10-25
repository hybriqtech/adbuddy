package tech.hybriq.adbuddy.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Test
import tech.hybriq.adbuddy.R

class HomeNoticeTest {
    @Test fun questionsFirst_theyExpire() {
        assertEquals(HomeNotice.QUESTIONS, HomeNotice.of(LinkStatus.CUT, pendingQuestions = 2, notificationsBlocked = true))
    }

    @Test fun oneQuestionIsEnough() {
        assertEquals(HomeNotice.QUESTIONS, HomeNotice.of(LinkStatus.CUT, 1, true))
        assertEquals(HomeNotice.QUESTIONS, HomeNotice.of(null, 1, false))
    }

    @Test fun blockedNotificationsShowWhateverTheLink() {
        assertEquals(HomeNotice.NOTIFICATIONS, HomeNotice.of(LinkStatus.CONNECTED, 0, true))
        assertEquals(HomeNotice.NOTIFICATIONS, HomeNotice.of(null, 0, true))
        assertEquals(HomeNotice.NOTIFICATIONS, HomeNotice.of(LinkStatus.CONNECTING, 0, true))
    }

    @Test fun thenTheLinkProblems_inOrder() {
        assertEquals(HomeNotice.CUT, HomeNotice.of(LinkStatus.CUT, 0, true))
        assertEquals(HomeNotice.UPDATE_APP, HomeNotice.of(LinkStatus.UPDATE_APP, 0, true))
        assertEquals(HomeNotice.UPDATE_HOST, HomeNotice.of(LinkStatus.UPDATE_HOST, 0, true))
        assertEquals(HomeNotice.NOT_SET_UP, HomeNotice.of(LinkStatus.NOT_SET_UP, 0, true))
    }

    @Test fun blockedNotificationsBeatPauseAndWaiting() {
        assertEquals(HomeNotice.NOTIFICATIONS, HomeNotice.of(LinkStatus.PAUSED, 0, true))
        assertEquals(HomeNotice.NOTIFICATIONS, HomeNotice.of(LinkStatus.WAITING, 0, true))
        assertEquals(HomeNotice.PAUSED, HomeNotice.of(LinkStatus.PAUSED, 0, false))
        assertEquals(HomeNotice.WAITING, HomeNotice.of(LinkStatus.WAITING, 0, false))
    }

    @Test fun nothingToDoNoCard() {
        assertNull(HomeNotice.of(null, 0, false))
        assertNull(HomeNotice.of(LinkStatus.CONNECTED, 0, false))
        assertNull(HomeNotice.of(LinkStatus.CONNECTING, 0, false))
    }

    @Test fun onlyTheCardsWithSomethingToTapHaveAButton() {
        assertEquals(
            setOf(HomeNotice.QUESTIONS, HomeNotice.CUT, HomeNotice.NOTIFICATIONS, HomeNotice.PAUSED, HomeNotice.WAITING),
            HomeNotice.entries.filter { it.hasAction }.toSet(),
        )
    }

    // Review of T7/T8: the shield is the header's Cut button; only the CUT card may wear it.
    @Test fun onlyTheCutCardUsesAShield() {
        for (n in HomeNotice.entries) {
            val res = cardIconRes(n)
            if (n == HomeNotice.CUT) assertEquals(R.drawable.ic_shield_off, res) else {
                assertNotEquals("$n", R.drawable.ic_shield, res)
                assertNotEquals("$n", R.drawable.ic_shield_off, res)
            }
        }
    }

    @Test fun questionsPausedAndUpdateLookDifferent() {
        val icons = listOf(HomeNotice.QUESTIONS, HomeNotice.PAUSED, HomeNotice.UPDATE_APP).map { cardIconRes(it) }
        assertEquals(3, icons.toSet().size)
        assertEquals(cardIconRes(HomeNotice.UPDATE_APP), cardIconRes(HomeNotice.UPDATE_HOST))
        assertNotEquals(null, icons.firstOrNull())
    }
}
