package tech.hybriq.adbuddy.term

/** What the terminal's WebView may load: only the bundled files under /assets/term/ (spec M3 §7.3). */
object TermWebPolicy {
    const val ORIGIN = "https://appassets.androidplatform.net"
    const val PREFIX = "$ORIGIN/assets/term/"
    const val PAGE = PREFIX + "index.html"

    /** A plain file name of the folder (it is flat): ASCII letters, digits, '.', '-' and '_', not starting with '.'. */
    private val NAME = Regex("[A-Za-z0-9_-][A-Za-z0-9._-]*")

    /**
     * One file of the folder by its plain name, and nothing else: no query, fragment, sub-folder, escape (%),
     * backslash or "..". The page and what it links are plain names; an allowlist leaves no way out of the
     * folder however the URL is written.
     */
    fun allowed(url: String): Boolean {
        if (!url.startsWith(PREFIX)) return false
        val rest = url.removePrefix(PREFIX)
        return NAME.matches(rest) && !rest.contains("..")
    }
}
