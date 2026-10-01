package tech.hybriq.adbuddy.chat

/** A piece of a line, already safe to draw (spec M4 §7.1): no HTML, no images, links only http(s). */
sealed interface Inline {
    data class Text(val text: String) : Inline
    data class Strong(val text: String) : Inline
    data class Em(val text: String) : Inline
    data class Code(val text: String) : Inline

    /** [url] always passes [Links.domain]: the person sees the domain before it opens. */
    data class Link(val text: String, val url: String) : Inline
}

sealed interface Block {
    data class Paragraph(val inlines: List<Inline>) : Block
    data class Heading(val level: Int, val inlines: List<Inline>) : Block
    data class Quote(val inlines: List<Inline>) : Block

    /** One item of a list; [marker] is "•" or "1."; [indent] the nesting level (0..3). */
    data class Item(val marker: String, val indent: Int, val inlines: List<Inline>) : Block
    data class CodeBlock(val lang: String, val text: String) : Block
}

/**
 * The markdown of the agent's messages (protocol spec §5.1: markdown safe).
 * A small parser of our own, with no library: what it does not know it leaves
 * as text, never as HTML, never an image to load, never a link that is not
 * http(s). Pure, never throws, and bounded: it keeps the first [MAX_INPUT]
 * characters and [MAX_BLOCKS] blocks, and has no recursion, so no input can
 * overflow the stack.
 */
object Markdown {
    const val MAX_INPUT = 100_000
    const val MAX_BLOCKS = 2_000

    private val HEADING = Regex("^(#{1,6})\\s+(.*)$")
    private val ITEM = Regex("^(\\s*)([-*+]|\\d{1,9}[.)])\\s+(.*)$")

    fun parse(source: String): List<Block> {
        val src = source.replace("\r\n", "\n").replace('\r', '\n').let { if (it.length > MAX_INPUT) it.take(MAX_INPUT) + "…" else it }
        val out = ArrayList<Block>()
        val para = ArrayList<String>()
        fun flush() {
            if (para.isNotEmpty()) out.add(Block.Paragraph(inlines(para.joinToString("\n"))))
            para.clear()
        }
        val lines = src.split('\n')
        var i = 0
        while (i < lines.size && out.size < MAX_BLOCKS) {
            val line = lines[i]
            val trimmed = line.trimStart()
            when {
                trimmed.startsWith("```") -> {
                    flush()
                    val lang = trimmed.removePrefix("```").trim().takeWhile { !it.isWhitespace() }.take(20)
                    val body = ArrayList<String>()
                    i++
                    while (i < lines.size && !lines[i].trimStart().startsWith("```")) body.add(lines[i++])
                    out.add(Block.CodeBlock(lang, body.joinToString("\n"))) // an unclosed fence runs to the end
                }
                line.isBlank() -> flush()
                else -> {
                    val h = HEADING.find(trimmed)
                    val it = ITEM.find(line)
                    val q = trimmed.startsWith(">")
                    when {
                        h != null -> { flush(); out.add(Block.Heading(h.groupValues[1].length, inlines(h.groupValues[2]))) }
                        q -> { flush(); out.add(Block.Quote(inlines(trimmed.removePrefix(">").trimStart()))) }
                        it != null -> {
                            flush()
                            val m = it.groupValues[2]
                            out.add(Block.Item(if (m[0].isDigit()) m else "•", (it.groupValues[1].length / 2).coerceAtMost(3), inlines(it.groupValues[3])))
                        }
                        else -> para.add(line)
                    }
                }
            }
            i++
        }
        flush()
        return out
    }

    /** The inline spans of a text: `code`, **strong**, *em* and _em_, [text](url) and ![alt](url) (only its alt). No nesting. */
    internal fun inlines(s: String): List<Inline> {
        val out = ArrayList<Inline>()
        val text = StringBuilder()
        fun flush() {
            if (text.isNotEmpty()) out.add(Inline.Text(text.toString()))
            text.setLength(0)
        }
        var i = 0
        while (i < s.length) {
            val c = s[i]
            when {
                c == '`' -> {
                    val j = s.indexOf('`', i + 1)
                    if (j > i + 1) { flush(); out.add(Inline.Code(s.substring(i + 1, j))); i = j + 1 } else { text.append(c); i++ }
                }
                c == '!' && s.startsWith("![", i) -> {
                    val l = link(s, i + 1)
                    if (l != null) { text.append(l.text); i = l.end } else { text.append(c); i++ } // an image is only its alt text: the URL is never loaded
                }
                c == '[' -> {
                    val l = link(s, i)
                    if (l == null) { text.append(c); i++ } else {
                        if (Links.domain(l.url) != null) { flush(); out.add(Inline.Link(l.text.ifEmpty { l.url }, l.url)) } else text.append(l.text)
                        i = l.end
                    }
                }
                c == '*' && s.startsWith("**", i) -> {
                    val j = s.indexOf("**", i + 2)
                    if (j > i + 2 && !s[i + 2].isWhitespace() && !s[j - 1].isWhitespace()) { flush(); out.add(Inline.Strong(s.substring(i + 2, j))); i = j + 2 } else { text.append(c); i++ }
                }
                c == '*' || c == '_' -> {
                    val j = s.indexOf(c, i + 1)
                    val wordBefore = i > 0 && s[i - 1].isLetterOrDigit()
                    val wordAfter = j >= 0 && j + 1 < s.length && s[j + 1].isLetterOrDigit()
                    val ok = j > i + 1 && !s[i + 1].isWhitespace() && !s[j - 1].isWhitespace() && (c == '*' || (!wordBefore && !wordAfter))
                    if (ok && !s.substring(i + 1, j).contains('\n')) { flush(); out.add(Inline.Em(s.substring(i + 1, j))); i = j + 1 } else { text.append(c); i++ }
                }
                else -> { text.append(c); i++ }
            }
        }
        flush()
        return out
    }

    private class Link(val text: String, val url: String, val end: Int)

    /** [text](url) starting at [open] (the '['), or null. The url has no white space, parentheses or angle brackets. */
    private fun link(s: String, open: Int): Link? {
        val close = s.indexOf(']', open + 1)
        if (close < 0 || close + 1 >= s.length || s[close + 1] != '(') return null
        val end = s.indexOf(')', close + 2)
        if (end < 0) return null
        val url = s.substring(close + 2, end)
        if (url.isEmpty() || url.any { it.isWhitespace() || it == '(' || it == '<' || it == '>' }) return null
        val label = s.substring(open + 1, close)
        return Link(label, url, end + 1)
    }
}
