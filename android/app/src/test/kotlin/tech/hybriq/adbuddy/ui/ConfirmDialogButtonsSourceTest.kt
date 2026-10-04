package tech.hybriq.adbuddy.ui

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.listener.CodeScan

/**
 * The hierarchy of the buttons of the 🔴 dialog is its class, which nothing on screen tells apart in a test: Deny is the
 * filled Button (the way out and the loudest), Allow an OutlinedButton (a deliberate decision, not the highlighted
 * action) and Allow during the session the quiet TextButton. A source check, as TouchGuardSourceTest.
 */
class ConfirmDialogButtonsSourceTest {
    private val src = CodeScan.code(File("src/main/kotlin/tech/hybriq/adbuddy/ui/Screens.kt").readText())

    private fun dialogBody(): String {
        val at = Regex("""\bfun\s+ConfirmDialog\(""").find(src)!!
        var i = at.range.last
        var depth = 0
        while (i < src.length) {
            if (src[i] == '(') depth++
            if (src[i] == ')' && --depth == 0) break
            i++
        }
        val open = src.indexOf('{', i)
        depth = 0
        var j = open
        while (true) {
            if (src[j] == '{') depth++
            if (src[j] == '}' && --depth == 0) return src.substring(open, j + 1)
            j++
        }
    }

    /** The class of the button whose label is [res]. */
    private fun classOf(res: String): List<String> =
        Regex("""\b(\w*Button)\((?:(?!\b\w*Button\()[\s\S])*?R\.string\.$res\)""").findAll(dialogBody()).map { it.groupValues[1] }.toList()

    @Test fun denyIsTheFilledButton() = assertEquals(listOf("Button"), classOf("action_deny"))

    // The dialog's own button row put Deny beside a wide Allow label and over it (found by the layout test): the three
    // buttons are one Column, Deny first, so it cannot overlap Allow nor the one for the session.
    @Test fun denyComesFirstInTheOneColumnOfButtons() {
        val body = dialogBody()
        val col = body.substring(body.indexOf("confirmButton"))
        assertTrue(col.indexOf("action_deny") in 0 until col.indexOf("action_allow)"))
        assertTrue(col.indexOf("action_allow)") < col.indexOf("action_allow_session"))
    }

    @Test fun allowIsOutlined() = assertEquals(listOf("OutlinedButton"), classOf("action_allow"))

    @Test fun allowForTheSessionIsAQuietTextButton() {
        assertEquals(listOf("TextButton"), classOf("action_allow_session"))
        assertTrue(Regex("""action_allow_session\)[^
]*labelMedium""").containsMatchIn(dialogBody()))
    }

    @Test fun theGateOfAllowIsUnchanged() {
        val body = dialogBody()
        assertEquals(2, Regex("""enabled = !unread && armed""").findAll(body).count())
        assertTrue(Regex("""onAnswer\(Decision\.DENY\)""").containsMatchIn(body))
    }
}
