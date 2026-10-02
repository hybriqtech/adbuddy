package tech.hybriq.adbuddy

import android.content.Context
import android.content.res.Configuration
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/** The strings in the language of a context, as Android resolves them (M6c spec 5.4): the real resources, on a device or emulator. */
@RunWith(AndroidJUnit4::class)
class LocalesTest {
    private val target = InstrumentationRegistry.getInstrumentation().targetContext

    private fun inLanguage(tag: String): Context {
        val config = Configuration(target.resources.configuration)
        config.setLocale(Locale.forLanguageTag(tag))
        return target.createConfigurationContext(config)
    }

    @Test fun english() {
        val c = inLanguage("en")
        assertEquals("The agent wants to: x", c.getString(R.string.confirm_title, "x"))
        assertEquals("Working: x", c.getString(R.string.notif_working_on, "x"))
        assertEquals("1 more waiting", c.resources.getQuantityString(R.plurals.perm_more, 1, 1))
        assertEquals("3 more waiting", c.resources.getQuantityString(R.plurals.perm_more, 3, 3))
    }

    @Test fun spanish() {
        val c = inLanguage("es")
        assertEquals("El agente quiere: x", c.getString(R.string.confirm_title, "x"))
        assertEquals("Trabajando: x", c.getString(R.string.notif_working_on, "x"))
        assertEquals("1 más esperando", c.resources.getQuantityString(R.plurals.perm_more, 1, 1))
        assertEquals("3 más esperando", c.resources.getQuantityString(R.plurals.perm_more, 3, 3))
    }

    @Test fun spanishOfOtherCountriesIsSpanish() {
        assertEquals("Conectado", inLanguage("es-MX").getString(R.string.notif_public_connected))
        assertEquals("Conectado", inLanguage("es-419").getString(R.string.notif_public_connected))
    }

    @Test fun anyOtherLanguageIsEnglish() {
        assertEquals("Connected", inLanguage("fr").getString(R.string.notif_public_connected))
        assertEquals("Connected", inLanguage("de-DE").getString(R.string.notif_public_connected))
    }

    @Test fun everyErrorCodeHasATextInBothLanguages() {
        for (tag in listOf("en", "es")) {
            val c = inLanguage(tag)
            for (id in listOf(R.string.error_unsupported, R.string.error_invalid, R.string.error_not_found, R.string.error_no_agent, R.string.error_busy,
                R.string.error_too_large, R.string.error_checksum, R.string.error_failed, R.string.error_unavailable, R.string.error_protected,
                R.string.error_expired, R.string.error_ambiguous, R.string.error_unknown)) {
                assertTrue("$tag: empty text for ${c.resources.getResourceEntryName(id)}", c.getString(id).isNotBlank())
            }
        }
    }
}
