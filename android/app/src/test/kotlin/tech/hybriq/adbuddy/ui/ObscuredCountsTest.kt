package tech.hybriq.adbuddy.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** The reference count behind ObscuredTouchGuard, without Compose or a View. */
class ObscuredCountsTest {
    private class Key
    private val flags = HashMap<Key, Boolean>()
    private val counts = ObscuredCounts<Key>(read = { flags[it] ?: false }, write = { k, v -> flags[k] = v })

    @Test fun enterSetsTheFlagAndExitRestoresTheOriginal() {
        val k = Key()
        counts.enter(k)
        assertTrue(flags[k]!!)
        counts.exit(k)
        assertFalse(flags[k]!!)
    }

    @Test fun anOriginalTrueStaysTrue() {
        val k = Key().also { flags[it] = true }
        counts.enter(k)
        counts.exit(k)
        assertTrue(flags[k]!!)
    }

    @Test fun nestedUsesKeepTheFlagUntilTheLastLeaves() {
        val k = Key()
        counts.enter(k)
        counts.enter(k)
        counts.exit(k)
        assertTrue("one user is still there", flags[k]!!)
        counts.exit(k)
        assertFalse(flags[k]!!)
    }

    @Test fun theOriginalIsTheOneBeforeTheFirstUserNotTheForcedValue() {
        val k = Key()
        counts.enter(k)
        counts.enter(k)
        counts.exit(k)
        counts.exit(k)
        assertEquals(false, flags[k])
    }

    @Test fun anExtraExitChangesNothingAndTheNextCycleStillWorks() {
        val k = Key()
        counts.exit(k)
        assertEquals(null, flags[k])
        counts.enter(k)
        counts.exit(k)
        counts.exit(k)
        assertFalse(flags[k]!!)
        counts.enter(k)
        assertTrue(flags[k]!!)
    }

    @Test fun keysAreIndependent() {
        val a = Key()
        val b = Key()
        counts.enter(a)
        counts.enter(b)
        counts.exit(a)
        assertFalse(flags[a]!!)
        assertTrue(flags[b]!!)
    }
}
