package tech.hybriq.adbuddy.ui.theme

import org.junit.Assert.assertTrue
import org.junit.Test

class PaletteContrastTest {
    // The pairs that carry text or controls (WCAG AA: 4.5 for text).
    private val pairs = listOf(
        "onPrimary" to "primary", "onPrimaryContainer" to "primaryContainer",
        "onSecondary" to "secondary", "onSecondaryContainer" to "secondaryContainer",
        "onSurface" to "surface", "onSurface" to "background", "onSurfaceVariant" to "surfaceVariant",
        "onError" to "error", "onErrorContainer" to "errorContainer",
        "primary" to "surface", "error" to "surface",
    )

    private fun check(name: String, scheme: Map<String, Int>) {
        for ((fg, bg) in pairs) {
            val c = Palette.contrast(scheme.getValue(fg), scheme.getValue(bg))
            assertTrue("$name: $fg on $bg is $c", c >= 4.5)
        }
    }

    @Test fun lightPaletteIsReadable() = check("light", Palette.light)
    @Test fun darkPaletteIsReadable() = check("dark", Palette.dark)

    @Test fun contrastOfBlackOnWhiteIs21() {
        assertTrue(Math.abs(Palette.contrast(0xFF000000.toInt(), 0xFFFFFFFF.toInt()) - 21.0) < 0.01)
    }
}
