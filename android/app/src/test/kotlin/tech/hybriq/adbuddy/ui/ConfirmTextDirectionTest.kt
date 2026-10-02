package tech.hybriq.adbuddy.ui

import java.io.File
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * What the person confirms must read in the order it runs: the agent's command, detail and summary are laid
 * out left to right whatever their first letter (Compose otherwise takes the base direction from the first
 * strong letter, so a command that starts in Hebrew or Arabic would reorder its parts around ";" and the like).
 * A source check, since the JVM cannot draw a Compose screen: it fails if the fix is taken out of a card.
 */
class ConfirmTextDirectionTest {
    private val dir = File("src/main/kotlin/tech/hybriq/adbuddy/ui")

    /** The text of the `Text(...)` call that holds [marker], from its "Text(" to its closing parenthesis. */
    private fun textCall(src: String, marker: String): String {
        val at = src.indexOf(marker)
        assertTrue("no $marker in the source", at >= 0)
        val start = src.lastIndexOf("Text(", at)
        assertTrue("$marker is not inside a Text(", start >= 0)
        var depth = 0
        for (i in start until src.length) {
            when (src[i]) {
                '(' -> depth++
                ')' -> if (--depth == 0) return src.substring(start, i + 1)
            }
        }
        error("unbalanced parentheses after $marker")
    }

    private fun assertLtr(file: String, marker: String) {
        val call = textCall(File(dir, file).readText(), marker)
        assertTrue("$file: the Text of $marker must set textDirection = TextDirection.Ltr", call.contains("TextDirection.Ltr"))
    }

    @Test fun thePermissionCardShowsTheSummaryLeftToRight() = assertLtr("PermissionCard.kt", "p.summary.ifEmpty")

    @Test fun thePermissionCardShowsTheDetailLeftToRight() = assertLtr("PermissionCard.kt", "p.detail,")

    @Test fun theConfirmDialogShowsTheCommandAndDetailLeftToRight() = assertLtr("Screens.kt", "Text(line,")
}
