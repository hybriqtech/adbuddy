package tech.hybriq.adbuddy.listener

/** Strips comments and strings from Kotlin/Java source so the source-checking tests look at code only. */
object CodeScan {
    /**
     * The code of a file without comments and strings (a small scanner: a regex overflows the stack on big
     * files). What is inside a template ("${...}") is code, not string: a call hidden in one is seen.
     */
    fun code(t: String): String {
        val out = StringBuilder()
        var i = 0
        while (i < t.length) {
            val c = t[i]
            when {
                t.startsWith("//", i) -> while (i < t.length && t[i] != '\n') i++
                t.startsWith("/*", i) -> {
                    val end = t.indexOf("*/", i + 2)
                    i = if (end < 0) t.length else end + 2
                }
                t.startsWith("\"\"\"", i) -> {
                    i = skipString(t, i + 3, raw = true, out)
                    out.append("\"\"")
                }
                c == '"' -> {
                    i = skipString(t, i + 1, raw = false, out)
                    out.append("\"\"")
                }
                c == '\'' -> { // a char literal, such as '"'
                    i = skipChar(t, i + 1)
                    out.append("''")
                }
                else -> {
                    out.append(c)
                    i++
                }
            }
        }
        return out.toString()
    }

    /** The index just past a string whose content starts at [from]; the code of its templates goes to [out]. */
    private fun skipString(t: String, from: Int, raw: Boolean, out: StringBuilder): Int {
        var i = from
        while (i < t.length) {
            when {
                raw && t.startsWith("\"\"\"", i) -> return i + 3
                !raw && t[i] == '"' -> return i + 1
                !raw && t[i] == '\\' -> i += 2
                t.startsWith("\${", i) -> {
                    val end = templateEnd(t, i + 2)
                    out.append(' ').append(code(t.substring(i + 2, minOf(end, t.length)))).append(' ')
                    i = end + 1
                }
                else -> i++
            }
        }
        return t.length
    }

    private fun skipChar(t: String, from: Int): Int {
        var i = from
        while (i < t.length && t[i] != '\'') i += if (t[i] == '\\') 2 else 1
        return i + 1
    }

    /** The index of the } that closes a template whose code starts at [from]; unbalanced, the rest of the file is code. */
    private fun templateEnd(t: String, from: Int): Int {
        var depth = 1
        var i = from
        while (i < t.length) {
            when {
                t.startsWith("\"\"\"", i) -> i = skipString(t, i + 3, raw = true, StringBuilder())
                t[i] == '"' -> i = skipString(t, i + 1, raw = false, StringBuilder())
                t[i] == '\'' -> i = skipChar(t, i + 1)
                t[i] == '{' -> { depth++; i++ }
                t[i] == '}' -> {
                    if (--depth == 0) return i
                    i++
                }
                else -> i++
            }
        }
        return t.length
    }
}
