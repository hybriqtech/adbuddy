package tech.hybriq.adbuddy.a11y

/**
 * Finds what a tap means (M5 spec §3.2), as host/internal/uidump.Find does
 * for adb: the visible nodes with that exact id; else with that exact text or
 * description; else the same ignoring case. Each one stands for its nearest
 * clickable ancestor (itself if it is clickable, itself if none is). A
 * password field's text never matches.
 */
object Matcher {
    sealed interface Match {
        /** target is the node to click; bounds what it covers on screen. */
        data class Found(val target: UiNode, val bounds: Box) : Match

        data object NotFound : Match

        /** A node of ADBuddy or of a hidden (protected) app, which is never touched. */
        data object Protected : Match

        /** Several targets: how many and the bounds of the first 5. */
        data class Ambiguous(val n: Int, val bounds: List<Box>) : Match
    }

    private class Candidate(val target: UiNode, val bounds: Box, val adbuddy: Boolean)

    fun find(windows: List<UiWindow>, screen: Box, id: String?, text: String?, hidden: (String) -> Boolean = { false }): Match {
        val exact = mutableListOf<Candidate>()
        val byText = mutableListOf<Candidate>()
        val folded = mutableListOf<Candidate>()
        fun walk(n: UiNode, clip: Box, clickable: Pair<UiNode, Box>?, depth: Int) {
            if (depth > Dumper.MAX_DEPTH || depth > 0 && !n.visible) return
            val b = n.bounds.clip(clip)
            val here = if (n.clickable && !b.empty) n to b else clickable
            if (!b.empty) {
                val (t, tb) = here ?: (n to b)
                val c = Candidate(t, tb, n.pkg == Packages.ADBUDDY || t.pkg == Packages.ADBUDDY || hidden(n.pkg) || hidden(t.pkg))
                when {
                    !id.isNullOrEmpty() && n.id == id -> exact += c
                    !text.isNullOrEmpty() && !n.password && (n.text == text || n.desc == text) -> byText += c
                    !text.isNullOrEmpty() && !n.password && (n.text.equals(text, ignoreCase = true) || n.desc.equals(text, ignoreCase = true)) -> folded += c
                }
            }
            val inner = if (n.scrollable && !b.empty) b else clip
            n.children.forEach { walk(it, inner, here, depth + 1) }
        }
        for (w in windows.sortedByDescending { it.layer }) {
            val root = w.root ?: continue
            walk(root, screen.clip(w.bounds).takeUnless { it.empty } ?: screen, null, 0)
        }
        for (set in listOf(exact, byText, folded)) {
            if (set.isEmpty()) continue
            if (set.any { it.adbuddy }) return Match.Protected
            val targets = set.distinctBy { it.bounds }
            if (targets.size == 1) return Match.Found(targets[0].target, targets[0].bounds)
            return Match.Ambiguous(targets.size, targets.take(5).map { it.bounds })
        }
        return Match.NotFound
    }

    /** The top window at a point, or null. */
    fun windowAt(windows: List<UiWindow>, x: Int, y: Int): UiWindow? =
        windows.sortedByDescending { it.layer }.firstOrNull { it.bounds.contains(x, y) }
}
