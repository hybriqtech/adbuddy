package tech.hybriq.adbuddy.a11y

/**
 * The agent's frame and pill (classic views, no Compose theme): ADBuddy's primary, fixed. Not system_accent1_*:
 * that is API 31 and the app runs on 30 (spec 2026-10-18 §6, bug 5); and dark text on the light accent of the dark
 * theme (bug 6: white on a light tone did not read). Same values as Palette's primary/onPrimary.
 */
object OverlayColors {
    data class Pair(val background: Int, val text: Int)

    fun of(dark: Boolean): Pair =
        if (dark) Pair(0xFF7FD8C6.toInt(), 0xFF00382F.toInt()) else Pair(0xFF00695C.toInt(), 0xFFFFFFFF.toInt())
}
