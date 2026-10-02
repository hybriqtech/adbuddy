package tech.hybriq.adbuddy.i18n

import java.io.File
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

// V36: plain `adbuddy setup` does not replace an installed app, so the text of an
// app that is too old must not send the person to it; `--apk <file>` is what does.
class UpdateAppTextTest {
    private fun text(dir: String): String =
        StringsCheck.parse(File(System.getProperty("adbuddy.resDir"), "$dir/strings.xml").readText())["status_update_app"]!!.texts[""]!!

    @Test fun statusUpdateAppDoesNotPromiseThatSetupUpdates() {
        for ((dir, bad) in listOf("values" to "to update it", "values-es" to "para actualizarla")) {
            val t = text(dir)
            assertFalse("$dir: status_update_app still says \"$bad\": $t", t.contains(bad))
            assertTrue("$dir: status_update_app must mention --apk: $t", t.contains("--apk"))
        }
    }
}
