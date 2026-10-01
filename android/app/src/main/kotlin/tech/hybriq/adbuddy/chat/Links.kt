package tech.hybriq.adbuddy.chat

import java.net.URI

/** Links in what the agent says (spec M3 §5.1 and §7.3): only http(s), and the domain is always shown before opening one. */
object Links {
    const val MAX_URL = 2048

    /** The host of a web link, lower-case, or null when it is not a plain http(s) link with a readable host. */
    fun domain(url: String): String? {
        if (url.length > MAX_URL) return null
        val uri = try { URI(url) } catch (e: Exception) { return null }
        val scheme = uri.scheme?.lowercase()
        if (scheme != "http" && scheme != "https") return null
        return uri.host?.lowercase()
    }
}
