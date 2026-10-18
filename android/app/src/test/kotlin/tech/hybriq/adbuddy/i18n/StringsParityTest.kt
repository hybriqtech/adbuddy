package tech.hybriq.adbuddy.i18n

import java.io.File
import java.io.StringReader
import javax.xml.parsers.DocumentBuilderFactory
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.w3c.dom.Element
import org.xml.sax.InputSource

/** One `<string>` or `<plurals>` of a strings.xml: the texts by quantity ("" for a plain string). */
class Res(val name: String, val translatable: Boolean, val plural: Boolean, val texts: Map<String, String>)

object StringsCheck {
    fun parse(xml: String): Map<String, Res> {
        val doc = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(InputSource(StringReader(xml)))
        val out = LinkedHashMap<String, Res>()
        val nodes = doc.documentElement.childNodes
        for (i in 0 until nodes.length) {
            val e = nodes.item(i) as? Element ?: continue
            val name = e.getAttribute("name")
            val translatable = e.getAttribute("translatable") != "false"
            when (e.tagName) {
                "string" -> out[name] = Res(name, translatable, false, mapOf("" to e.textContent))
                "plurals" -> {
                    val items = LinkedHashMap<String, String>()
                    val kids = e.getElementsByTagName("item")
                    for (j in 0 until kids.length) {
                        val item = kids.item(j) as Element
                        items[item.getAttribute("quantity")] = item.textContent
                    }
                    out[name] = Res(name, translatable, true, items)
                }
            }
        }
        return out
    }

    /** %1$s, %2$d, %s, %d, %.1f…; "%%" is a percent sign and is not a marker. */
    private val marker = Regex("%(?:(\\d+)\\$)?[-#+ 0,(]*\\d*(?:\\.\\d+)?([a-zA-Z%])")

    /** The markers of a text as "index:conversion"; an unnumbered one has index 0. */
    fun markers(text: String): List<String> = marker.findAll(text)
        .filter { it.groupValues[2] != "%" && it.groupValues[2] != "n" }
        .map { "${it.groupValues[1].ifEmpty { "0" }}:${it.groupValues[2]}" }
        .toList().sorted()

    /** A text AAPT2 would break or change silently: a bare ' or ", or a first character that makes it a reference. */
    fun escapingProblem(text: String): String? {
        var i = 0
        while (i < text.length) {
            val c = text[i]
            if (c == '\\') { i += 2; continue }
            if (c == '\'') return "an apostrophe without a backslash (write \\')"
            if (c == '"') return "a double quote without a backslash (write \\\")"
            i++
        }
        if (text.startsWith("@") || text.startsWith("?")) return "starts with @ or ?, which Android reads as a reference (write \\@ or \\?)"
        return null
    }

    /** The same text in both languages on purpose (a name, a symbol, a word that is the same). Each one says why. */
    val sameInBoth = mapOf(
        "perm_deny" to "\"No\" is the same word in both languages",
        "activity_entry" to "a status and an adb command line: the command is not translated",
        "activity_inverse" to "an adb command line: not translated",
        "settings_section_app" to "\"App\" is the same word in both languages",
    )

    fun problems(en: Map<String, Res>, es: Map<String, Res>): List<String> {
        val out = mutableListOf<String>()
        for ((name, e) in en) {
            if (!e.translatable) continue
            val s = es[name]
            if (s == null) { out += "$name: missing in values-es"; continue }
            if (s.plural != e.plural) { out += "$name: a plurals in one language and a string in the other"; continue }
            if (e.texts.keys != s.texts.keys) out += "$name: the quantities differ: en ${e.texts.keys}, es ${s.texts.keys}"
            if (e.plural) for ((lang, r) in listOf("en" to e, "es" to s)) {
                if ("one" !in r.texts || "other" !in r.texts) out += "$name: $lang needs one and other"
            }
            for ((q, et) in e.texts) {
                val st = s.texts[q] ?: continue
                val label = if (e.plural) "$name[$q]" else name
                if (et.isBlank()) out += "$label: empty in values"
                if (st.isBlank()) out += "$label: empty in values-es"
                val me = markers(et)
                val ms = markers(st)
                if (me.size > 1 && me.any { it.startsWith("0:") }) out += "$label: more than one marker needs positions (%1\$s, %2\$d) in values"
                if (ms.size > 1 && ms.any { it.startsWith("0:") }) out += "$label: more than one marker needs positions (%1\$s, %2\$d) in values-es"
                if (me != ms) out += "$label: the markers differ: values $me, values-es $ms"
                if (et == st && name !in sameInBoth && Regex("\\p{L}{2,}").containsMatchIn(et)) out += "$label: the same text in both languages (untranslated?); list it in sameInBoth with a reason if it is right"
                escapingProblem(et)?.let { out += "$label (values): $it" }
                escapingProblem(st)?.let { out += "$label (values-es): $it" }
            }
        }
        for ((name, s) in es) {
            if (name !in en) out += "$name: in values-es but not in values"
            else if (!s.translatable && en[name]?.translatable == true) out += "$name: translatable=\"false\" only in values-es"
        }
        return out
    }
}

class StringsParityTest {
    private fun res(dir: String): Map<String, Res> =
        StringsCheck.parse(File(System.getProperty("adbuddy.resDir"), "$dir/strings.xml").readText())

    @Test fun everyStringIsInBothLanguagesWithTheSameMarkersAndPlurals() {
        val problems = StringsCheck.problems(res("values"), res("values-es"))
        assertTrue("strings.xml and values-es/strings.xml disagree:\n" + problems.joinToString("\n"), problems.isEmpty())
    }

    // The lock-screen texts are public (spec §4): neither language gives them an argument, so nothing private can be put in them.
    @Test fun theLockScreenTextsTakeNoArgumentsInAnyLanguage() {
        for (dir in listOf("values", "values-es")) {
            val strings = res(dir)
            for (name in listOf("notif_public_connected", "notif_done_title", "notif_working", "notif_waiting", "notif_stopped", "notif_asking")) {
                val s = strings[name]
                assertTrue("$dir: $name is missing", s != null)
                for ((quantity, text) in s!!.texts) {
                    assertEquals("$dir: $name[$quantity] is shown on the lock screen and takes no marker", emptyList<String>(), StringsCheck.markers(text))
                }
            }
        }
    }

    @Test fun everyErrorCodeHasItsTextAndThereIsAnUnknownOne() {
        val en = res("values")
        val codes = tech.hybriq.adbuddy.bridge.ErrorCode::class.java.declaredFields
            .filter { java.lang.reflect.Modifier.isStatic(it.modifiers) && it.type == String::class.java }
            .map { it.get(null) as String }
        assertTrue("ErrorCode changed shape: $codes", codes.size >= 12)
        val missing = (codes + "unknown").map { "error_$it" }.filter { it !in en }
        assertTrue("strings.xml needs: $missing (ErrorText.resFor maps each code of the protocol to its own string)", missing.isEmpty())
    }

    @Test fun theCheckSeesWhatItShould() {
        fun p(en: String, es: String) = StringsCheck.problems(StringsCheck.parse("<resources>$en</resources>"), StringsCheck.parse("<resources>$es</resources>"))
        assertEquals(emptyList<String>(), p("<string name=\"a\">Hello %1\$s</string>", "<string name=\"a\">Hola %1\$s</string>"))
        assertEquals(emptyList<String>(), p("<string name=\"a\" translatable=\"false\">ADBuddy</string>", ""))
        assertEquals(emptyList<String>(), p("<string name=\"a\">100%% of %1\$s</string>", "<string name=\"a\">100%% de %1\$s</string>"))
        assertEquals(emptyList<String>(), p("<string name=\"a\">%1\$s of %2\$s</string>", "<string name=\"a\">%2\$s de %1\$s</string>")) // reordering is fine
        assertTrue(p("<string name=\"a\">x</string>", "").single().contains("missing in values-es"))
        assertTrue(p("", "<string name=\"a\">x</string>").single().contains("not in values"))
        assertTrue(p("<string name=\"a\">Hello %1\$s</string>", "<string name=\"a\">Hola %1\$d</string>").single().contains("markers differ"))
        assertTrue(p("<string name=\"a\">Hello %1\$s</string>", "<string name=\"a\">Hola</string>").single().contains("markers differ"))
        assertTrue(p("<string name=\"a\">%s of %s</string>", "<string name=\"a\">%s de %s</string>").any { it.contains("needs positions") })
        assertTrue(p("<string name=\"a\">Hello</string>", "<string name=\"a\"> </string>").any { it.contains("empty in values-es") })
        assertTrue(p("<string name=\"a\">Same words</string>", "<string name=\"a\">Same words</string>").single().contains("untranslated"))
        assertTrue(p("<string name=\"a\">Don't</string>", "<string name=\"a\">No</string>").any { it.contains("apostrophe") })
        assertEquals(emptyList<String>(), p("<string name=\"a\">Don\\'t</string>", "<string name=\"a\">No lo hagas</string>"))
        assertTrue(p("<string name=\"a\">@home</string>", "<string name=\"a\">hogar</string>").any { it.contains("reference") })
        val plural = "<plurals name=\"n\"><item quantity=\"one\">%1\$d file</item><item quantity=\"other\">%1\$d files</item></plurals>"
        val pluralEs = "<plurals name=\"n\"><item quantity=\"one\">%1\$d fichero</item><item quantity=\"other\">%1\$d ficheros</item></plurals>"
        assertEquals(emptyList<String>(), p(plural, pluralEs))
        assertTrue(p(plural, "<plurals name=\"n\"><item quantity=\"other\">%1\$d ficheros</item></plurals>").any { it.contains("quantities differ") })
        assertTrue(p("<plurals name=\"n\"><item quantity=\"other\">%1\$d files</item></plurals>", "<plurals name=\"n\"><item quantity=\"other\">%1\$d ficheros</item></plurals>").any { it.contains("needs one and other") })
        assertTrue(p(plural, "<string name=\"n\">x</string>").single().contains("plurals in one language"))
    }
}
