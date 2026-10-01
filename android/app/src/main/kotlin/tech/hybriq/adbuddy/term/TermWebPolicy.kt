package tech.hybriq.adbuddy.term

/** What the terminal's WebView may load: only the bundled files under /assets/term/ (spec M3 §7.3). */
object TermWebPolicy {
    const val ORIGIN = "https://appassets.androidplatform.net"
    const val PREFIX = "$ORIGIN/assets/term/"
    const val PAGE = PREFIX + "index.html"

    fun allowed(url: String): Boolean {
        if (!url.startsWith(PREFIX)) return false
        val rest = url.removePrefix(PREFIX)
        // No way out of the folder, however it is written.
        return rest.isNotEmpty() && !rest.contains("..") && !rest.contains('%') && !rest.contains('\\')
    }
}
