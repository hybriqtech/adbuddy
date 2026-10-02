package tech.hybriq.adbuddy.i18n

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** A string literal of a Kotlin source: its text with the `${…}` and `$name` templates taken out, and its line. */
class Literal(val text: String, val line: Int)

/** The source with comments gone and every string and character literal replaced by "S" (line breaks kept). */
class Lexed(val skeleton: String, val literals: List<Literal>)

object Lexer {
    fun lex(src: String): Lexed {
        val sk = StringBuilder()
        val lits = mutableListOf<Literal>()
        var line = 1
        var i = 0
        fun newline() { line++ }

        // Reads a string body from i (after the opening quote) to its closing quote; returns the text without templates.
        fun readString(raw: Boolean): String {
            val text = StringBuilder()
            while (i < src.length) {
                val c = src[i]
                when {
                    !raw && c == '\\' -> { text.append(c); if (i + 1 < src.length) text.append(src[i + 1]); i += 2 }
                    !raw && c == '"' -> { i++; return text.toString() }
                    raw && src.startsWith("\"\"\"", i) -> {
                        i += 3
                        while (i < src.length && src[i] == '"') i++ // """" ends at the last quote
                        return text.toString()
                    }
                    c == '$' && i + 1 < src.length && src[i + 1] == '{' -> {
                        i += 2
                        var depth = 1
                        while (i < src.length && depth > 0) {
                            when (src[i]) {
                                '{' -> depth++
                                '}' -> depth--
                                '"' -> { i++; readString(false); continue } // a string inside the template
                                '\n' -> newline()
                            }
                            i++
                        }
                    }
                    c == '$' && i + 1 < src.length && (src[i + 1].isLetter() || src[i + 1] == '_') -> {
                        i++
                        while (i < src.length && (src[i].isLetterOrDigit() || src[i] == '_')) i++
                    }
                    else -> { if (c == '\n') newline(); text.append(c); i++ }
                }
            }
            return text.toString()
        }

        while (i < src.length) {
            val c = src[i]
            when {
                src.startsWith("//", i) -> while (i < src.length && src[i] != '\n') i++
                src.startsWith("/*", i) -> {
                    var depth = 0
                    while (i < src.length) {
                        if (src.startsWith("/*", i)) { depth++; i += 2 }
                        else if (src.startsWith("*/", i)) { depth--; i += 2; if (depth == 0) break }
                        else { if (src[i] == '\n') { newline(); sk.append('\n') }; i++ }
                    }
                }
                src.startsWith("\"\"\"", i) -> {
                    val start = line
                    i += 3
                    val text = readString(true)
                    lits += Literal(text, start)
                    sk.append("\"S\"")
                    repeat(line - start) { sk.append('\n') }
                }
                c == '"' -> {
                    val start = line
                    i++
                    val text = readString(false)
                    lits += Literal(text, start)
                    sk.append("\"S\"")
                    repeat(line - start) { sk.append('\n') }
                }
                c == '\'' -> { // a character literal: '{', '\'', 'A'
                    i++
                    while (i < src.length && src[i] != '\'') { if (src[i] == '\\') i++; i++ }
                    i++
                    sk.append("'c'")
                }
                else -> { if (c == '\n') newline(); sk.append(c); i++ }
            }
        }
        return Lexed(sk.toString(), lits)
    }
}

/** Where a literal sits in the skeleton: the index of its "S" and its place in `literals`. */
private class Spot(val skIndex: Int, val lit: Literal)

class Finding(val file: String, val line: Int, val text: String, val why: String) {
    override fun toString() = "$file:$line: \"$text\" ($why)"
}

object HardcodedText {
    /** Calls whose literal arguments are text a person sees. */
    private val sinkCalls = listOf(
        "Text", "setContentTitle", "setContentText", "setSubText", "setTicker", "Toast.makeText", "NotificationChannel",
        "setName", "setLabel", "addAction", "Action.Builder", "AlertDialog", "Snackbar.make",
    )

    /** Named arguments (or assignments) whose literal value is text a person sees. */
    private val sinkNames = listOf("text", "contentDescription", "label", "title", "placeholder", "subtitle", "hint")

    /** Where phrases (two words or more) are looked for too: the files that build what a person sees. */
    val phrasePaths = listOf("ui/", "notify/", "confirm/", "control/", "files/", "AdbuddyApp.kt", "MainActivity.kt", "ChatBubbleActivity.kt", "bridge/BridgeService.kt", "a11y/Overlay.kt")

    private val phrase = Regex("\\p{L}{2,}\\s+\\p{L}{2,}")
    private val hasLetter = Regex("\\p{L}")
    private val previewAnnotation = Regex("@\\w*Preview\\w*")
    private val skipTokens = listOf("Log.", "throw ", "require(", "check(", "error(", "assert(")

    /** Matches the paren that closes the one at [open] in the skeleton, or -1. */
    private fun close(sk: String, open: Int): Int {
        var depth = 0
        for (j in open until sk.length) {
            when (sk[j]) {
                '(', '{', '[' -> depth++
                ')', '}', ']' -> { depth--; if (depth == 0) return j }
            }
        }
        return -1
    }

    /** The skeleton indexes of "S" at depth 0 of sk[from, to). */
    private fun topLevelLiterals(sk: String, from: Int, to: Int): List<Int> {
        val out = mutableListOf<Int>()
        var depth = 0
        var j = from
        while (j < to) {
            when (sk[j]) {
                '(', '{', '[' -> depth++
                ')', '}', ']' -> depth--
                '"' -> if (depth == 0 && sk.startsWith("\"S\"", j)) { out += j; j += 2 }
            }
            j++
        }
        return out
    }

    /** The end of the expression a named argument or assignment starts at [from]: a `,`, a `)` that closes the call, or the line's end, at depth 0. */
    private fun expressionEnd(sk: String, from: Int): Int {
        var depth = 0
        var j = from
        while (j < sk.length) {
            when (sk[j]) {
                '(', '{', '[' -> depth++
                ')', '}', ']' -> { if (depth == 0) return j; depth-- }
                ',' -> if (depth == 0) return j
                '\n' -> if (depth == 0) {
                    // a line that ends with an operator goes on in the next one
                    val before = sk.substring(from, j).trimEnd()
                    if (!(before.endsWith("+") || before.endsWith("?:") || before.endsWith("else") || before.endsWith("->"))) return j
                }
            }
            j++
        }
        return sk.length
    }

    /** The regions of the skeleton that are the body of a function with @Preview: no one reads them as text. */
    private fun previewRegions(sk: String): List<IntRange> {
        val out = mutableListOf<IntRange>()
        var from = 0
        while (true) {
            val at = previewAnnotation.find(sk, from)?.range?.first ?: -1
            if (at < 0) break
            val fn = sk.indexOf("fun ", at)
            if (fn < 0) break
            val open = sk.indexOf('{', fn)
            val eq = sk.indexOf('=', fn)
            val end = if (open >= 0 && (eq < 0 || open < eq)) close(sk, open) else sk.indexOf('\n', if (eq >= 0) eq else fn)
            if (end < 0) break
            out += at..end
            from = end
        }
        return out
    }

    /** The text of a literal as a person would see it: the letters it has. */
    private fun visible(l: Literal) = hasLetter.containsMatchIn(l.text)

    fun scan(file: String, src: String, exceptions: Set<String>): List<Finding> {
        val lex = Lexer.lex(src)
        val sk = lex.skeleton
        val spots = mutableListOf<Spot>()
        var n = 0
        var k = sk.indexOf("\"S\"")
        while (k >= 0 && n < lex.literals.size) {
            spots += Spot(k, lex.literals[n++])
            k = sk.indexOf("\"S\"", k + 3)
        }
        val byIndex = spots.associateBy { it.skIndex }
        val previews = previewRegions(sk)
        val lines = src.lines()
        fun skipped(l: Literal, at: Int): Boolean {
            if (previews.any { at in it }) return true
            if ("$file|${l.text.trim()}" in exceptions || "$file|*" in exceptions) return true
            // The literal's line and the two before it (a statement that goes on over several lines).
            val window = (maxOf(0, l.line - 3)..minOf(lines.size - 1, l.line - 1)).joinToString(" ") { lines[it] }
            return skipTokens.any { window.contains(it) }
        }

        val found = LinkedHashMap<Int, Finding>()
        fun flag(index: Int, why: String) {
            val spot = byIndex[index] ?: return
            if (!visible(spot.lit) || skipped(spot.lit, index)) return
            found.putIfAbsent(index, Finding(file, spot.lit.line, spot.lit.text, why))
        }

        // 1. A literal as an argument of a call that shows text, in any file.
        for (name in sinkCalls) {
            Regex("(?<!\\w)" + Regex.escape(name) + "\\s*\\(").findAll(sk).forEach { m ->
                val open = m.range.last
                val end = close(sk, open)
                if (end > 0) topLevelLiterals(sk, open + 1, end).forEach { flag(it, "a literal in $name(…)") }
            }
        }
        // 2. A literal given to a named argument that is text: text = "…", contentDescription = "…".
        for (name in sinkNames) {
            Regex("(?<!\\w)" + name + "\\s*=(?!=)\\s*").findAll(sk).forEach { m ->
                val from = m.range.last + 1
                topLevelLiterals(sk, from, expressionEnd(sk, from)).forEach { flag(it, "a literal given to $name =") }
            }
        }
        // 3. A phrase (two words or more) in the files that build what a person sees.
        if (phrasePaths.any { file == it || file.startsWith(it) }) {
            for (spot in spots) if (phrase.containsMatchIn(spot.lit.text)) flag(spot.skIndex, "a phrase in a file that builds what a person sees")
        }
        return found.values.toList()
    }
}

class NoHardcodedTextTest {
    private val root = File(System.getProperty("adbuddy.kotlinDir"))

    /** `file|text` per line: a literal that is not for a person, with why. `file|*` exempts the whole file. */
    private fun exceptions(): Set<String> {
        val text = javaClass.getResourceAsStream("/i18n-exceptions.txt")!!.bufferedReader().readText()
        return text.lines().map { it.trim() }.filter { it.isNotEmpty() && !it.startsWith("#") }.map { line ->
            val parts = line.split(" | ")
            assertTrue("i18n-exceptions.txt: \"$line\" must be `file | text | why`", parts.size >= 3 && parts[2].isNotBlank())
            parts[0].trim() + "|" + parts[1].trim()
        }.toSet()
    }

    @Test fun noTextForAPersonIsWrittenInTheCode() {
        val ex = exceptions()
        val findings = root.walkTopDown().filter { it.isFile && it.extension == "kt" }.sortedBy { it.path }.flatMap { f ->
            HardcodedText.scan(f.relativeTo(File(root, "tech/hybriq/adbuddy")).invariantSeparatorsPath, f.readText(), ex)
        }.toList()
        assertTrue("Text a person sees must come from strings.xml (values and values-es). Found:\n" + findings.joinToString("\n") +
            "\nIf it is not for a person (a message to the agent, a key name), add it to src/test/resources/i18n-exceptions.txt with its reason.", findings.isEmpty())
    }

    @Test fun everyExceptionStillMatchesSomething() {
        val sources = root.walkTopDown().filter { it.isFile && it.extension == "kt" }
            .associate { it.relativeTo(File(root, "tech/hybriq/adbuddy")).invariantSeparatorsPath to it.readText() }
        val stale = exceptions().filter { e ->
            val (file, text) = e.split("|", limit = 2)
            val src = sources[file] ?: return@filter true
            text != "*" && !src.contains(text)
        }
        assertTrue("i18n-exceptions.txt lists literals that are no longer in the code: $stale", stale.isEmpty())
    }

    @Test fun theScannerSeesWhatItShould() {
        fun scan(src: String, file: String = "ui/X.kt") = HardcodedText.scan(file, src, emptySet()).map { it.text }
        assertEquals(listOf("Hello"), scan("""fun a() { Text("Hello") }"""))
        assertEquals(listOf("Hello"), scan("""fun a() { Text(text = "Hello", modifier = m) }"""))
        assertEquals(listOf("Hello"), scan("""fun a() { Icon(x, contentDescription = "Hello") }"""))
        assertEquals(listOf("Sending"), scan("""fun a() { Text(if (b) "Sending" else stringResource(R.string.x)) }"""))
        assertEquals(listOf("Hi"), scan("""fun a() = NotificationCompat.Builder(c, id).setContentTitle("Hi")"""))
        assertEquals(listOf("Hi"), scan("""fun a() { Toast.makeText(c, "Hi", Toast.LENGTH_LONG).show() }""", "bridge/X.kt"))
        assertEquals(listOf("Conversation with the agent"), scan("""fun a() { NotificationChannel("c", "Conversation with the agent", 1) }""", "AdbuddyApp.kt").drop(1))
        assertEquals(listOf("Read the battery level"), scan("""val a = JournalEntry(description = "Read the battery level")"""))
        // What is not text for a person.
        assertEquals(emptyList<String>(), scan("""fun a() { Text(stringResource(R.string.hello)) }"""))
        assertEquals(emptyList<String>(), scan("""fun a() { Text("") ; Text("·"); Text("→ ${'$'}x") }"""))
        assertEquals(emptyList<String>(), scan("""fun a() { Text(foo("Hello")) }"""))
        assertEquals(emptyList<String>(), scan("""fun a() { // Text("Hello") in a comment
            /* Text("Hello") */ }"""))
        assertEquals(emptyList<String>(), scan("""val a = "no phrase here_but_one_word""", "bridge/Y.kt"))
        assertEquals(emptyList<String>(), scan("""fun a() { Log.w(TAG, "could not go to the foreground") }"""))
        assertEquals(emptyList<String>(), scan("""fun a() { throw IllegalStateException("not ready yet") }"""))
        assertEquals(emptyList<String>(), scan("""fun a() { require(x) { "must not be empty" } }"""))
        assertEquals(emptyList<String>(), scan("""@Preview @Composable private fun P() { Text("Hello there") ; Foo(text = "Hi") }
            @Composable fun Q() { Text(stringResource(R.string.x)) }"""))
        // A phrase in a file that builds what a person sees; the same elsewhere is not looked at.
        assertEquals(listOf("two words"), scan("""val s = "two words" """, "notify/Z.kt"))
        assertEquals(emptyList<String>(), scan("""val s = "two words" """, "bridge/Z.kt"))
        // Templates are not words, strings inside templates do not break the reading, and raw strings work.
        assertEquals(listOf("Hello  there"), scan("""val s = "Hello ${'$'}{name.ifEmpty { "x y" }} there" """, "notify/Z.kt"))
        assertEquals(listOf("one two"), scan("val s = \"\"\"one two\"\"\"", "ui/Z.kt"))
        // Exceptions by file and text, or by file.
        val e = HardcodedText.scan("a11y/Overlay.kt", """val s = "to the agent" """, setOf("a11y/Overlay.kt|to the agent"))
        assertTrue(e.isEmpty())
        assertTrue(HardcodedText.scan("a11y/Overlay.kt", """val s = "to the agent" """, setOf("a11y/Overlay.kt|*")).isEmpty())
    }
}
