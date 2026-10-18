package tech.hybriq.adbuddy.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class HomeNoticeTest {
    @Test fun questionsFirst_theyExpire() {
        assertEquals(HomeNotice.QUESTIONS, HomeNotice.of(LinkStatus.CUT, pendingQuestions = 2, notificationsBlocked = true))
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
}
