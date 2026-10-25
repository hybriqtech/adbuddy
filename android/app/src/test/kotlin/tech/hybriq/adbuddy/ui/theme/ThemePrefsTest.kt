package tech.hybriq.adbuddy.ui.theme

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment

@RunWith(RobolectricTestRunner::class)
class ThemePrefsTest {
    private val app get() = RuntimeEnvironment.getApplication()

    @Test fun wallpaperColorsAreOffUnlessThePersonTurnsThemOn() {
        assertFalse(ThemePrefs(app).dynamic)
    }

    @Test fun theChoiceIsKept() {
        ThemePrefs(app).dynamic = true
        assertTrue(ThemePrefs(app).dynamic)
        ThemePrefs(app).dynamic = false
        assertFalse(ThemePrefs(app).dynamic)
    }
}
