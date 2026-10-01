package tech.hybriq.adbuddy.notify

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.SessionState

class DoneDetectorTest {
    private val d = DoneDetector()
    private fun st(s: String) = SessionState(state = s)

    @Test fun workEndingAfterTenSecondsIsNews() {
        assertFalse(d.onState(st("working"), 0))
        assertFalse(d.onState(st("working"), 5_000))
        assertTrue(d.onState(st("waiting"), 12_000))
    }

    @Test fun shortWorkIsNot() {
        d.onState(st("working"), 0)
        assertFalse(d.onState(st("waiting"), 9_999))
    }

    @Test fun idleEndsItToo_andTenSecondsExactlyCounts() {
        d.onState(st("working"), 0)
        assertTrue(d.onState(st("idle"), 10_000))
    }

    @Test fun workCountsFromItsFirstWorkingState() {
        d.onState(st("working"), 0)
        d.onState(st("working"), 8_000)
        assertTrue(d.onState(st("waiting"), 11_000))
    }

    @Test fun onlyOnceAndNotWithoutWork() {
        d.onState(st("working"), 0)
        assertTrue(d.onState(st("waiting"), 20_000))
        assertFalse(d.onState(st("waiting"), 30_000))
        assertFalse(d.onState(st("idle"), 40_000))
    }

    @Test fun aStoppedAgentOrAResetForgetsTheWork() {
        d.onState(st("working"), 0)
        assertFalse(d.onState(st("stopped"), 5_000))
        assertFalse(d.onState(st("waiting"), 20_000))
        d.onState(st("working"), 30_000)
        d.reset()
        assertFalse(d.onState(st("waiting"), 50_000))
    }
}
