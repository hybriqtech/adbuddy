package tech.hybriq.adbuddy.a11y

import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.UiDump
import tech.hybriq.adbuddy.bridge.UiElement
import tech.hybriq.adbuddy.bridge.UiLimits

/**
 * Turns the windows into dump's result (M5 spec §3.1): the visible nodes,
 * clipped to the screen, their window and their scrollable ancestors, the
 * windows from the top layer down, never ADBuddy's own. Without all, only the
 * informative or usable nodes. A password field never carries its text, and
 * the packages hidden says (the protected apps) are left out, as in the
 * other half of a split screen.
 */
object Dumper {
    const val MAX_ELEMENTS = 2000
    const val MAX_DEPTH = 75

    /** What is cut first when the result is too big. */
    private const val SHORT_TEXT = 200

    fun dump(
        windows: List<UiWindow>,
        screen: Box,
        rotation: Int,
        pkg: String,
        activity: String,
        locked: Boolean,
        all: Boolean,
        hidden: (String) -> Boolean,
    ): UiDump {
        if (locked) return UiDump(locked = true, `package` = pkg, activity = activity, width = screen.r, height = screen.b, rotation = rotation, truncated = false, elements = emptyList())
        val elements = mutableListOf<UiElement>()
        var truncated = false
        fun walk(n: UiNode, clip: Box, depth: Int, window: String) {
            if (depth > MAX_DEPTH) {
                truncated = true
                return
            }
            if (n.pkg == Packages.ADBUDDY || hidden(n.pkg) || depth > 0 && !n.visible) return
            val b = n.bounds.clip(clip)
            if (!b.empty && (all || n.usable || n.informative)) {
                if (elements.size >= MAX_ELEMENTS) {
                    truncated = true
                    return
                }
                elements += element(n, b, window)
            }
            val inner = if (n.scrollable && !b.empty) b else clip
            n.children.forEach { walk(it, inner, depth + 1, window) }
        }
        for (w in windows.sortedByDescending { it.layer }) {
            val root = w.root ?: continue
            if (w.pkg == Packages.ADBUDDY || hidden(w.pkg)) continue
            walk(root, screen.clip(w.bounds).takeUnless { it.empty } ?: screen, 0, w.type)
        }
        val d = UiDump(locked = false, `package` = pkg, activity = activity, width = screen.r, height = screen.b, rotation = rotation, truncated = truncated, elements = elements)
        return fit(d)
    }

    private fun element(n: UiNode, b: Box, window: String) = UiElement(
        id = n.id,
        text = if (n.password) "" else n.text,
        desc = n.desc,
        hint = if (n.password) "" else n.hint,
        `class` = n.cls.substringAfterLast('.'),
        bounds = b.list(),
        click = n.clickable,
        edit = n.editable,
        scroll = n.scrollable,
        focused = n.focused,
        checked = if (n.checkable) n.checked else null,
        enabled = n.enabled,
        password = n.password,
        window = window,
    )

    /** The size of d as the app sends it: UTF-8 bytes of its JSON. */
    fun size(d: UiDump): Int = Protocol.json.encodeToString(UiDump.serializer(), d).toByteArray(Charsets.UTF_8).size

    /**
     * Keeps d under UiLimits.MAX_UI_RESULT: first the long texts are cut, then
     * the last elements go, and truncated says so.
     */
    fun fit(d: UiDump): UiDump {
        if (size(d) <= UiLimits.MAX_UI_RESULT) return d
        fun cut(s: String) = if (s.codePointCount(0, s.length) <= SHORT_TEXT) s else s.substring(0, s.offsetByCodePoints(0, SHORT_TEXT)) + "…"
        var e = d.elements.map { it.copy(text = cut(it.text), desc = cut(it.desc), hint = cut(it.hint)) }
        var out = d.copy(elements = e, truncated = true)
        while (e.isNotEmpty() && size(out) > UiLimits.MAX_UI_RESULT) {
            e = e.subList(0, e.size * 9 / 10)
            out = out.copy(elements = e)
        }
        return out
    }
}
