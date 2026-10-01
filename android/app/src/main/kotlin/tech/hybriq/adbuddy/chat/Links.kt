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
        // A link with userinfo (https://mibanco.com@evil.example) reads as another site: it is no link.
        if (uri.rawUserInfo != null) return null
        return uri.host?.lowercase()
    }

    private const val FSI = "\u2068"
    private const val PDI = "\u2069"

    /**
     * The text a link is drawn with: its own text without invisible or bidi control characters (category Cf) and, unless
     * that already is the address or its domain, the real domain in brackets, isolated (FSI..PDI) so it cannot be reordered.
     * The visible text can never pass for another site: `[https://mibanco.com](https://evil.example)` reads
     * `https://mibanco.com (evil.example)`. Not a web link: the text alone.
     */
    fun label(text: String, url: String): String {
        val clean = text.filter { Character.getType(it) != Character.FORMAT.toInt() }
        val d = domain(url) ?: return clean
        val t = clean.trim()
        return if (t == url || t.equals(d, ignoreCase = true)) clean else "$clean $FSI($d)$PDI"
    }

    data class Span(val start: Int, val end: Int, val url: String)

    private val URL = Regex("https?://[^\\s<>\"]+")
    private const val TRAILING = ".,;:!?'\""

    /** The web links of a text, in order (spec M3 §5.1: plain text with links). The sentence's own punctuation is not part of one. */
    fun find(text: String): List<Span> = URL.findAll(text).mapNotNull { m ->
        val start = m.range.first
        var end = m.range.last + 1
        while (end > start) {
            val c = text[end - 1]
            val unbalanced = (c == ')' && count(text, start, end, '(') < count(text, start, end, ')')) ||
                (c == ']' && count(text, start, end, '[') < count(text, start, end, ']')) ||
                (c == '}' && count(text, start, end, '{') < count(text, start, end, '}'))
            if (c in TRAILING || unbalanced) end-- else break
        }
        val url = text.substring(start, end)
        if (domain(url) == null) null else Span(start, end, url)
    }.toList()

    private fun count(t: String, from: Int, to: Int, ch: Char) = (from until to).count { t[it] == ch }
}
