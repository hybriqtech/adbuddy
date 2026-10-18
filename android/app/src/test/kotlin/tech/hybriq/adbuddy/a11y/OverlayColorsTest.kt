package tech.hybriq.adbuddy.a11y

import java.io.File
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.listener.CodeScan
import tech.hybriq.adbuddy.ui.theme.Palette

class OverlayColorsTest {
    @Test fun thePillReadsInLightAndDark() {
        for (dark in listOf(false, true)) {
            val c = OverlayColors.of(dark)
            assertTrue("dark=$dark", Palette.contrast(c.text, c.background) >= 4.5)
        }
    }

    @Test fun thePillTakesItsColorsFromOverlayColorsAndHasNoLiteralArgb() {
        val src = File(System.getProperty("adbuddy.kotlinDir"), "tech/hybriq/adbuddy/a11y/Overlay.kt").readText()
        val code = CodeScan.code(src)
        assertTrue("pill text must be colors.text", code.contains("setTextColor(colors.text)"))
        assertTrue("pill background must be colors.background", code.contains("setColor(colors.background)"))
        assertFalse("white text on the pill (bug 6)", code.contains("setTextColor(0xFFFFFFFF"))
        assertFalse("no literal ARGB in Overlay.kt", Regex("0x[0-9A-Fa-f]{8}").containsMatchIn(code))
    }

    @Test fun noColorOfApi31OnAMinSdk30App() {
        val src = File(System.getProperty("adbuddy.kotlinDir"), "tech/hybriq/adbuddy/a11y/Overlay.kt").readText()
        assertFalse("system_accent1_* is API 31+ (minSdk 30)", src.contains("system_accent"))
    }
}
