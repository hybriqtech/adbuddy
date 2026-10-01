package tech.hybriq.adbuddy.a11y

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class OverlayTest {
    @Test fun ifTheSecondViewFailsTheFirstIsRemoved() {
        val added = mutableListOf<String>()
        val removed = mutableListOf<String>()
        var thrown: Throwable? = null
        try {
            OverlayWindows.addBoth(
                first = { "frame".also { added += it } },
                second = { throw IllegalStateException("no permission") },
                remove = { removed += it },
            )
        } catch (e: IllegalStateException) {
            thrown = e
        }
        assertTrue(thrown != null)
        assertEquals(listOf("frame"), added)
        assertEquals(listOf("frame"), removed)
    }

    @Test fun bothAddedWhenNothingFails() {
        val removed = mutableListOf<String>()
        assertEquals("a" to "b", OverlayWindows.addBoth({ "a" }, { "b" }, { removed += it }))
        assertTrue(removed.isEmpty())
    }
}
