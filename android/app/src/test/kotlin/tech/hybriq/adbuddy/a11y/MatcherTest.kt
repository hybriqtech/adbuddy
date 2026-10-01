package tech.hybriq.adbuddy.a11y

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.a11y.Matcher.Match
import tech.hybriq.adbuddy.a11y.ScreenFixtures.SCREEN
import tech.hybriq.adbuddy.a11y.ScreenFixtures.windows

class MatcherTest {
    private fun found(m: Match): Box = (m as? Match.Found)?.bounds ?: error("not found: $m")

    @Test fun findsTheClickableAncestor() {
        assertEquals(Box(0, 600, 1344, 690), found(Matcher.find(windows, SCREEN, null, "Network & internet", { false })))
        assertEquals(Box(980, 1000, 1302, 1080), found(Matcher.find(windows, SCREEN, null, "Wi-Fi", { false })))
        assertEquals(Box(42, 180, 1302, 300), found(Matcher.find(windows, SCREEN, "com.android.settings:id/search_action_bar", null, { false })))
    }

    @Test fun exactTextFirst() {
        assertEquals(Box(0, 700, 1344, 790), found(Matcher.find(windows, SCREEN, null, "OK", { false })))
        val m = Matcher.find(windows, SCREEN, null, "Ok", { false })
        assertTrue(m is Match.Ambiguous && m.n == 2)
    }

    @Test fun ambiguousIds() {
        val m = Matcher.find(windows, SCREEN, "android:id/title", null, { false })
        assertTrue("$m", m is Match.Ambiguous && m.n == 4 && m.bounds.size == 4)
    }

    @Test fun neverADBuddyNorPasswordsNorHidden() {
        assertEquals(Match.Protected, Matcher.find(windows, SCREEN, null, "Allow", { false }))
        assertEquals(Match.NotFound, Matcher.find(windows, SCREEN, null, "hunter2", { false }))
        assertEquals(Match.NotFound, Matcher.find(windows, SCREEN, null, "Hidden", { false }))
        assertEquals(Match.NotFound, Matcher.find(windows, SCREEN, null, "Nothing like it", { false }))
    }

    @Test fun theTopWindowAtAPoint() {
        assertEquals(Packages.ADBUDDY, Matcher.windowAt(windows, 1000, 50)?.pkg)
        assertEquals("com.android.settings", Matcher.windowAt(windows, 1000, 500)?.pkg)
        assertFalse(Matcher.windowAt(windows, 5000, 5000) != null)
    }
}
