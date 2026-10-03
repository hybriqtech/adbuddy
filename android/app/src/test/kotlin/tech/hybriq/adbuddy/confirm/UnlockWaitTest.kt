package tech.hybriq.adbuddy.confirm

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** M2-8: pure JVM, with a fake clock, scheduler and lock. */
class UnlockWaitTest {
    private class Fake(var locked: () -> Boolean = { true }) {
        var t = 0L
        var polls = 0
        var scheduled = 0
        val queue = ArrayDeque<Pair<Long, () -> Unit>>()
        var failSchedule = false
        val results = mutableListOf<Boolean>()
        fun isLocked(): Boolean { polls++; return locked() }
        fun schedule(d: Long, step: () -> Unit) {
            scheduled++
            if (failSchedule) throw IllegalStateException("dead handler")
            queue.addLast(d to step)
        }
        fun wait(sdk: Int = 34) = UnlockWait(sdk, ::isLocked, { t }, ::schedule) { results.add(it) }
        /** Runs the scheduled steps, advancing the clock, until nothing is left or [limit] ms. */
        fun run(limit: Long = 10_000) {
            while (queue.isNotEmpty() && t <= limit) {
                val (d, s) = queue.removeFirst()
                t += d
                s()
            }
        }
    }

    @Test fun unlocksAt300msGivesTrue() {
        val f = Fake()
        f.locked = { f.t < 300 }
        f.wait().begin(); f.run()
        assertEquals(listOf(true), f.results)
    }

    @Test fun neverUnlockedGivesFalseAt2000ByTheClock() {
        val f = Fake()
        f.wait().begin(); f.run()
        assertEquals(listOf(false), f.results)
        assertTrue("by the clock: ${f.t}", f.t in 2000L..2050L)
    }

    @Test fun alreadyUnlockedAtTheFirstPollGivesTrue() {
        val f = Fake { false }
        f.wait().begin()
        assertEquals(listOf(true), f.results)
        assertEquals(0, f.scheduled)
    }

    @Test fun sdk30DropsWithoutPolling() {
        val f = Fake { false }
        f.wait(sdk = 30).begin()
        assertEquals(listOf(false), f.results)
        assertEquals(0, f.polls)
        assertEquals(0, f.scheduled)
    }

    @Test fun anExceptionInIsLockedKeepsWaitingAndEndsFalse() {
        val f = Fake { throw IllegalStateException("boom") }
        f.wait().begin(); f.run()
        assertEquals(listOf(false), f.results)
    }

    @Test fun aLatePollWithAnUnlockedPhoneGivesFalse() {
        val f = Fake()
        val w = f.wait()
        w.begin()
        val (_, step) = f.queue.removeFirst()
        f.t = 2500; f.locked = { false }
        step()
        assertEquals(listOf(false), f.results)
    }

    @Test fun resultIsDeliveredOnceEvenIfStepRunsTwice() {
        val f = Fake()
        f.locked = { f.t < 100 }
        f.wait().begin()
        val (_, step) = f.queue.first()
        f.t = 100
        step(); step()
        assertEquals(listOf(true), f.results)
    }

    @Test fun aFailingScheduleGivesFalse() {
        val f = Fake()
        f.failSchedule = true
        f.wait().begin()
        assertEquals(listOf(false), f.results)
    }

    @Test fun notLockedAtDeliveryIsTrueAtOnceWithoutWaiting() {
        val f = Fake()
        UnlockWait.afterUnlock(false, 34, ::isLockedNever, { f.t }, f::schedule) { f.results.add(it) }
        assertEquals(listOf(true), f.results)
        assertEquals(0, f.scheduled)
    }

    @Test fun lockedAtDeliveryWaitsViaTheSameWait() {
        val f = Fake()
        f.locked = { f.t < 300 }
        UnlockWait.afterUnlock(true, 34, f::isLocked, { f.t }, f::schedule) { f.results.add(it) }
        f.run()
        assertEquals(listOf(true), f.results)
    }

    private fun isLockedNever(): Boolean = throw AssertionError("must not poll")
}
