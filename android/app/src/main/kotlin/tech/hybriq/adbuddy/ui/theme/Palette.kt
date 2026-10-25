package tech.hybriq.adbuddy.ui.theme

import kotlin.math.pow

/**
 * ADBuddy's own palette, used wherever the dynamic color is off (the default). Plain ARGB values so the contrast
 * of every pair can be tested on the JVM (PaletteContrastTest). Nowhere else in the app has a fixed color
 * (except a11y/OverlayColors, which has no Compose).
 */
object Palette {
    private fun c(v: Long) = v.toInt()

    /** The *Fixed roles of Material 3 (1.4): the same in light and dark (all pairs checked: >= 5.4:1). */
    private val fixed: Map<String, Int> = mapOf(
        "primaryFixed" to c(0xFFB8F0E4), "primaryFixedDim" to c(0xFF7FD8C6), "onPrimaryFixed" to c(0xFF00201B), "onPrimaryFixedVariant" to c(0xFF004F45),
        "secondaryFixed" to c(0xFFCDE8E1), "secondaryFixedDim" to c(0xFFB1CCC6), "onSecondaryFixed" to c(0xFF06201C), "onSecondaryFixedVariant" to c(0xFF334B47),
        "tertiaryFixed" to c(0xFFD6E3FF), "tertiaryFixedDim" to c(0xFFA8C8FF), "onTertiaryFixed" to c(0xFF001B3E), "onTertiaryFixedVariant" to c(0xFF26477A),
    )

    val light: Map<String, Int> = fixed + mapOf(
        "primary" to c(0xFF00695C), "onPrimary" to c(0xFFFFFFFF), "primaryContainer" to c(0xFFB8F0E4), "onPrimaryContainer" to c(0xFF00201B),
        "inversePrimary" to c(0xFF7FD8C6),
        "secondary" to c(0xFF4A635E), "onSecondary" to c(0xFFFFFFFF), "secondaryContainer" to c(0xFFCDE8E1), "onSecondaryContainer" to c(0xFF06201C),
        "tertiary" to c(0xFF3F5F90), "onTertiary" to c(0xFFFFFFFF), "tertiaryContainer" to c(0xFFD6E3FF), "onTertiaryContainer" to c(0xFF001B3E),
        "background" to c(0xFFF6FAF8), "onBackground" to c(0xFF171D1B), "surface" to c(0xFFF6FAF8), "onSurface" to c(0xFF171D1B),
        "surfaceVariant" to c(0xFFDBE5E1), "onSurfaceVariant" to c(0xFF3F4946), "surfaceTint" to c(0xFF00695C),
        "inverseSurface" to c(0xFF2B3230), "inverseOnSurface" to c(0xFFEDF2EF),
        "error" to c(0xFFBA1A1A), "onError" to c(0xFFFFFFFF), "errorContainer" to c(0xFFFFDAD6), "onErrorContainer" to c(0xFF410002),
        "outline" to c(0xFF6F7976), "outlineVariant" to c(0xFFBFC9C5), "scrim" to c(0xFF000000),
        "surfaceBright" to c(0xFFF6FAF8), "surfaceDim" to c(0xFFD6DBD9),
        "surfaceContainerLowest" to c(0xFFFFFFFF), "surfaceContainerLow" to c(0xFFF0F5F2), "surfaceContainer" to c(0xFFEAEFEC),
        "surfaceContainerHigh" to c(0xFFE4E9E7), "surfaceContainerHighest" to c(0xFFDEE4E1),
    )

    val dark: Map<String, Int> = fixed + mapOf(
        "primary" to c(0xFF7FD8C6), "onPrimary" to c(0xFF00382F), "primaryContainer" to c(0xFF004F45), "onPrimaryContainer" to c(0xFFB8F0E4),
        "inversePrimary" to c(0xFF00695C),
        "secondary" to c(0xFFB1CCC6), "onSecondary" to c(0xFF1C3531), "secondaryContainer" to c(0xFF334B47), "onSecondaryContainer" to c(0xFFCDE8E1),
        "tertiary" to c(0xFFA8C8FF), "onTertiary" to c(0xFF06305F), "tertiaryContainer" to c(0xFF26477A), "onTertiaryContainer" to c(0xFFD6E3FF),
        "background" to c(0xFF0F1513), "onBackground" to c(0xFFDEE4E1), "surface" to c(0xFF0F1513), "onSurface" to c(0xFFDEE4E1),
        "surfaceVariant" to c(0xFF3F4946), "onSurfaceVariant" to c(0xFFBFC9C5), "surfaceTint" to c(0xFF7FD8C6),
        "inverseSurface" to c(0xFFDEE4E1), "inverseOnSurface" to c(0xFF2B3230),
        "error" to c(0xFFFFB4AB), "onError" to c(0xFF690005), "errorContainer" to c(0xFF93000A), "onErrorContainer" to c(0xFFFFDAD6),
        "outline" to c(0xFF899390), "outlineVariant" to c(0xFF3F4946), "scrim" to c(0xFF000000),
        "surfaceBright" to c(0xFF353B39), "surfaceDim" to c(0xFF0F1513),
        "surfaceContainerLowest" to c(0xFF0A0F0E), "surfaceContainerLow" to c(0xFF171D1B), "surfaceContainer" to c(0xFF1B211F),
        "surfaceContainerHigh" to c(0xFF252B2A), "surfaceContainerHighest" to c(0xFF303634),
    )

    /** ADBuddy's own semantic colours (the status pill, the journal's levels): never from the wallpaper (spec R6). */
    val extraLight: Map<String, Int> = mapOf(
        "success" to c(0xFF2E6B30), "onSuccess" to c(0xFFFFFFFF), "successContainer" to c(0xFFC8E6C9), "onSuccessContainer" to c(0xFF0A2E0C),
        "warning" to c(0xFF7A4A00), "onWarning" to c(0xFFFFFFFF), "warningContainer" to c(0xFFFFDDB3), "onWarningContainer" to c(0xFF291800),
    )
    val extraDark: Map<String, Int> = mapOf(
        "success" to c(0xFF8BD88F), "onSuccess" to c(0xFF00390A), "successContainer" to c(0xFF1F5323), "onSuccessContainer" to c(0xFFC8E6C9),
        "warning" to c(0xFFFFB951), "onWarning" to c(0xFF452B00), "warningContainer" to c(0xFF633F00), "onWarningContainer" to c(0xFFFFDDB3),
    )

    /** WCAG contrast ratio of two ARGB colors (alpha ignored). */
    fun contrast(a: Int, b: Int): Double {
        val la = luminance(a)
        val lb = luminance(b)
        return (maxOf(la, lb) + 0.05) / (minOf(la, lb) + 0.05)
    }

    private fun luminance(c: Int): Double {
        fun ch(v: Int): Double {
            val s = v / 255.0
            return if (s <= 0.03928) s / 12.92 else ((s + 0.055) / 1.055).pow(2.4)
        }
        return 0.2126 * ch((c shr 16) and 0xFF) + 0.7152 * ch((c shr 8) and 0xFF) + 0.0722 * ch(c and 0xFF)
    }
}
