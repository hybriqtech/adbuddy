package tech.hybriq.adbuddy.ui

import java.io.File
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * A4-2: the dialog's Allow buttons wait until the person has scrolled to the end of the question, as the
 * PermissionCard does, and the dialog says how much of the command it still hides. A source check, since the
 * JVM cannot draw a Compose screen: it fails if the gate is taken out.
 */
class ConfirmDialogSourceTest {
    private val dialog: String = File("src/main/kotlin/tech/hybriq/adbuddy/ui/Screens.kt").readText().let { src ->
        val start = src.indexOf("fun ConfirmDialog(")
        assertTrue("no ConfirmDialog in Screens.kt", start >= 0)
        src.substring(start)
    }

    @Test fun theAllowButtonsWaitForTheEndOfTheText() {
        assertTrue("the dialog must look at canScrollForward", dialog.contains("canScrollForward"))
        for (label in listOf("R.string.action_allow)", "R.string.action_allow_session)")) {
            val at = dialog.indexOf(label)
            assertTrue("no $label", at >= 0)
            val button = dialog.lastIndexOf("TextButton(", at)
            assertTrue("$label: its button must be enabled only once read", dialog.substring(button, at).contains("enabled = !unread"))
        }
    }

    @Test fun theDialogSaysHowMuchOfTheCommandIsHidden() {
        assertTrue(dialog.contains("R.plurals.confirm_hidden"))
        assertTrue(dialog.contains("ConfirmUi.shown("))
    }
}
