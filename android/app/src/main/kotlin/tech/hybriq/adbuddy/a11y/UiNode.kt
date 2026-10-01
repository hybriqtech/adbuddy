package tech.hybriq.adbuddy.a11y

/**
 * The screen as the helpers see it (M5 spec §3), copied from the
 * accessibility tree so the logic (Dumper, Matcher, UiExecutor) runs on the
 * JVM without Android (spec §11).
 */

/** A rectangle in screen pixels: left, top, right, bottom. */
data class Box(val l: Int, val t: Int, val r: Int, val b: Int) {
    val empty: Boolean get() = r <= l || b <= t

    /** The part of this box inside o, or an empty box. */
    fun clip(o: Box): Box {
        val c = Box(maxOf(l, o.l), maxOf(t, o.t), minOf(r, o.r), minOf(b, o.b))
        return if (c.empty) NONE else c
    }

    fun contains(x: Int, y: Int): Boolean = !empty && x >= l && x < r && y >= t && y < b

    val centerX: Int get() = (l + r) / 2
    val centerY: Int get() = (t + b) / 2

    fun list(): List<Int> = listOf(l, t, r, b)

    companion object {
        val NONE = Box(0, 0, 0, 0)
    }
}

/**
 * A node of a window. ref is what the service acts on (an
 * AccessibilityNodeInfo); null in tests.
 */
data class UiNode(
    val pkg: String,
    val cls: String,
    val id: String = "",
    val text: String = "",
    val desc: String = "",
    val hint: String = "",
    val bounds: Box,
    val visible: Boolean = true,
    val clickable: Boolean = false,
    val editable: Boolean = false,
    val scrollable: Boolean = false,
    val checkable: Boolean = false,
    val checked: Boolean = false,
    val enabled: Boolean = true,
    val focused: Boolean = false,
    val password: Boolean = false,
    val children: List<UiNode> = emptyList(),
    val ref: Any? = null,
) {
    /** Something the agent can do with it. */
    val usable: Boolean get() = clickable || editable || scrollable || checkable

    /** Something to read: text, a hint or a description (a password field's text does not count). */
    val informative: Boolean get() = !password && (text.isNotEmpty() || hint.isNotEmpty()) || desc.isNotEmpty()
}

/** A window on screen: the app, a dialog, the keyboard, the system UI or a split-screen divider. */
data class UiWindow(
    /** app, dialog, ime, system or split. */
    val type: String,
    val layer: Int,
    val bounds: Box,
    val root: UiNode?,
) {
    val pkg: String get() = root?.pkg.orEmpty()
}

/** Packages the helpers care about. */
object Packages {
    const val ADBUDDY = "tech.hybriq.adbuddy"
    const val SYSTEM_UI = "com.android.systemui"
}
