package tech.hybriq.adbuddy.ui.theme

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.a11y.OverlayColors

class PaletteContrastTest {
    /** Every colour role of Material 3's ColorScheme: a missing one falls back to the lilac baseline (bug 7). */
    private val roles = setOf(
        "primary", "onPrimary", "primaryContainer", "onPrimaryContainer", "inversePrimary",
        "secondary", "onSecondary", "secondaryContainer", "onSecondaryContainer",
        "tertiary", "onTertiary", "tertiaryContainer", "onTertiaryContainer",
        "background", "onBackground", "surface", "onSurface", "surfaceVariant", "onSurfaceVariant", "surfaceTint",
        "inverseSurface", "inverseOnSurface", "error", "onError", "errorContainer", "onErrorContainer",
        "outline", "outlineVariant", "scrim", "surfaceBright", "surfaceDim",
        "surfaceContainer", "surfaceContainerHigh", "surfaceContainerHighest", "surfaceContainerLow", "surfaceContainerLowest",
        "primaryFixed", "primaryFixedDim", "onPrimaryFixed", "onPrimaryFixedVariant",
        "secondaryFixed", "secondaryFixedDim", "onSecondaryFixed", "onSecondaryFixedVariant",
        "tertiaryFixed", "tertiaryFixedDim", "onTertiaryFixed", "onTertiaryFixedVariant",
    )

    private val surfaces = listOf("surface", "background", "surfaceBright", "surfaceDim", "surfaceContainerLowest", "surfaceContainerLow",
        "surfaceContainer", "surfaceContainerHigh", "surfaceContainerHighest")

    // The pairs that carry text or controls (WCAG AA: 4.5 for text).
    private val pairs = listOf(
        "onPrimary" to "primary", "onPrimaryContainer" to "primaryContainer",
        "onSecondary" to "secondary", "onSecondaryContainer" to "secondaryContainer",
        "onTertiary" to "tertiary", "onTertiaryContainer" to "tertiaryContainer", "tertiary" to "tertiaryContainer",
        "onSurfaceVariant" to "surfaceVariant", "inverseOnSurface" to "inverseSurface", "inversePrimary" to "inverseSurface",
        "onError" to "error", "onErrorContainer" to "errorContainer",
        "primary" to "surface", "error" to "surface", "tertiary" to "surface", "primary" to "surfaceContainerHigh",
    ) + surfaces.flatMap { listOf("onSurface" to it, "onSurfaceVariant" to it) } +
        listOf("Primary", "Secondary", "Tertiary").flatMap { r ->
            listOf("${r.lowercase()}Fixed", "${r.lowercase()}FixedDim").flatMap { bg -> listOf("on${r}Fixed" to bg, "on${r}FixedVariant" to bg) }
        }

    private val extraPairs = listOf(
        "onSuccess" to "success", "onSuccessContainer" to "successContainer",
        "onWarning" to "warning", "onWarningContainer" to "warningContainer",
    )

    private val containers = listOf("surfaceContainerLowest", "surfaceContainerLow", "surfaceContainer", "surfaceContainerHigh", "surfaceContainerHighest")

    private fun check(name: String, scheme: Map<String, Int>, extra: Map<String, Int>) {
        for ((fg, bg) in pairs) {
            val c = Palette.contrast(scheme.getValue(fg), scheme.getValue(bg))
            assertTrue("$name: $fg on $bg is $c", c >= 4.5)
        }
        for ((fg, bg) in extraPairs) {
            val c = Palette.contrast(extra.getValue(fg), extra.getValue(bg))
            assertTrue("$name: $fg on $bg is $c", c >= 4.5)
        }
        for (s in listOf("success", "warning")) {
            val c = Palette.contrast(extra.getValue(s), scheme.getValue("surface"))
            assertTrue("$name: $s on surface is $c", c >= 4.5)
        }
        // The cards: the outline and the pill dot are graphic elements (3.0, WCAG 1.4.11) on every surface they sit on.
        for (bg in listOf("surface") + containers) {
            val c = Palette.contrast(scheme.getValue("outline"), scheme.getValue(bg))
            assertTrue("$name: outline on $bg is $c", c >= 3.0)
        }
        // The button inside the red card is primary text on errorContainer.
        val button = Palette.contrast(scheme.getValue("primary"), scheme.getValue("errorContainer"))
        assertTrue("$name: primary on errorContainer is $button", button >= 4.5)
        // success and warning are used as text and icons on the neutral cards.
        for (s in listOf("success", "warning")) for (bg in containers) {
            val c = Palette.contrast(extra.getValue(s), scheme.getValue(bg))
            assertTrue("$name: $s on $bg is $c", c >= 4.5)
        }
    }

    @Test fun lightPaletteIsReadable() = check("light", Palette.light, Palette.extraLight)
    @Test fun darkPaletteIsReadable() = check("dark", Palette.dark, Palette.extraDark)

    @Test fun noRoleIsLeftToTheBaseline() {
        assertEquals(roles, Palette.light.keys)
        assertEquals(roles, Palette.dark.keys)
    }

    /** The overlay (classic views, no Compose) repeats primary/onPrimary: the two must not drift apart. */
    @Test fun overlayColorsAreThePalettePrimary() {
        assertEquals(0xFF00695C.toInt(), Palette.light.getValue("primary"))
        assertEquals(0xFFFFFFFF.toInt(), Palette.light.getValue("onPrimary"))
        assertEquals(0xFF7FD8C6.toInt(), Palette.dark.getValue("primary"))
        assertEquals(0xFF00382F.toInt(), Palette.dark.getValue("onPrimary"))
        for ((dark, p) in listOf(false to Palette.light, true to Palette.dark)) {
            val o = OverlayColors.of(dark)
            assertEquals("dark=$dark background", p.getValue("primary"), o.background)
            assertEquals("dark=$dark text", p.getValue("onPrimary"), o.text)
        }
    }

    @Test fun contrastOfBlackOnWhiteIs21() {
        assertTrue(Math.abs(Palette.contrast(0xFF000000.toInt(), 0xFFFFFFFF.toInt()) - 21.0) < 0.01)
    }
}
