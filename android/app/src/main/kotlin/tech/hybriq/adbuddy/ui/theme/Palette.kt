package tech.hybriq.adbuddy.ui.theme

import kotlin.math.pow

/**
 * ADBuddy's own palette, used on Android 11 (no Material You) and wherever the
 * dynamic color is off. Plain ARGB values so the contrast of every pair can be
 * tested on the JVM (PaletteContrastTest). Nowhere else in the app has a fixed color.
 */
object Palette {
    val light: Map<String, Int> = mapOf(
        "primary" to 0xFF006B5E.toInt(), "onPrimary" to 0xFFFFFFFF.toInt(),
        "primaryContainer" to 0xFF9FF2E1.toInt(), "onPrimaryContainer" to 0xFF00201B.toInt(),
        "secondary" to 0xFF4A6360.toInt(), "onSecondary" to 0xFFFFFFFF.toInt(),
        "secondaryContainer" to 0xFFCCE8E3.toInt(), "onSecondaryContainer" to 0xFF051F1D.toInt(),
        "background" to 0xFFF4FBF8.toInt(), "onBackground" to 0xFF161D1B.toInt(),
        "surface" to 0xFFF4FBF8.toInt(), "onSurface" to 0xFF161D1B.toInt(),
        "surfaceVariant" to 0xFFDAE5E1.toInt(), "onSurfaceVariant" to 0xFF3F4947.toInt(),
        "outline" to 0xFF6F7977.toInt(),
        "error" to 0xFFBA1A1A.toInt(), "onError" to 0xFFFFFFFF.toInt(),
        "errorContainer" to 0xFFFFDAD6.toInt(), "onErrorContainer" to 0xFF410002.toInt(),
    )

    val dark: Map<String, Int> = mapOf(
        "primary" to 0xFF82D5C5.toInt(), "onPrimary" to 0xFF00382F.toInt(),
        "primaryContainer" to 0xFF005046.toInt(), "onPrimaryContainer" to 0xFF9FF2E1.toInt(),
        "secondary" to 0xFFB1CCC7.toInt(), "onSecondary" to 0xFF1C3532.toInt(),
        "secondaryContainer" to 0xFF334B48.toInt(), "onSecondaryContainer" to 0xFFCCE8E3.toInt(),
        "background" to 0xFF0E1513.toInt(), "onBackground" to 0xFFDEE4E1.toInt(),
        "surface" to 0xFF0E1513.toInt(), "onSurface" to 0xFFDEE4E1.toInt(),
        "surfaceVariant" to 0xFF3F4947.toInt(), "onSurfaceVariant" to 0xFFBEC9C5.toInt(),
        "outline" to 0xFF899390.toInt(),
        "error" to 0xFFFFB4AB.toInt(), "onError" to 0xFF690005.toInt(),
        "errorContainer" to 0xFF93000A.toInt(), "onErrorContainer" to 0xFFFFDAD6.toInt(),
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
