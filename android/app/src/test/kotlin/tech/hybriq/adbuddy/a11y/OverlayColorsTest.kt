package tech.hybriq.adbuddy.a11y

import java.io.File
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.ui.theme.Palette

class OverlayColorsTest {
    @Test fun thePillReadsInLightAndDark() {
        for (dark in listOf(false, true)) {
            val c = OverlayColors.of(dark)
            assertTrue("dark=$dark", Palette.contrast(c.text, c.background) >= 4.5)
        }
    }

    @Test fun noColorOfApi31OnAMinSdk30App() {
        val src = File(System.getProperty("adbuddy.kotlinDir"), "tech/hybriq/adbuddy/a11y/Overlay.kt").readText()
        assertFalse("system_accent1_* is API 31+ (minSdk 30)", src.contains("system_accent"))
    }
}
