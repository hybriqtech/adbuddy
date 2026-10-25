package tech.hybriq.adbuddy.i18n

import android.content.res.Configuration
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import tech.hybriq.adbuddy.R

/** Two "offline" texts that sound alike are not the same string: a cross between them (and between languages) is caught here. */
@RunWith(RobolectricTestRunner::class)
class OfflineStringsTest {
    private fun text(lang: String, id: Int): String {
        val cfg = Configuration(RuntimeEnvironment.getApplication().resources.configuration).apply { setLocale(Locale.forLanguageTag(lang)) }
        return RuntimeEnvironment.getApplication().createConfigurationContext(cfg).getString(id)
    }

    @Test fun eachKeyKeepsItsOwnTextInBothLanguages() {
        assertEquals("Available when the PC is connected", text("en", R.string.remote_disconnected))
        assertEquals("Disponible cuando el PC esté conectado", text("es", R.string.remote_disconnected))
        assertEquals("Sin conexión con el PC", text("es", R.string.confirm_offline))
        assertEquals("No connection with the PC", text("en", R.string.confirm_offline))
    }
}
