package tech.hybriq.adbuddy.a11y

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ProtectedAppsTest {
    @Test fun proposesByName() {
        val apps = listOf(
            "com.bbva.bbvacontigo" to "BBVA",
            "es.lacaixa.mobile.android.newwapicon" to "CaixaBank",
            "com.paypal.android.p2pmobile" to "PayPal",
            "com.google.android.apps.walletnfcrel" to "Google Wallet",
            "com.x8bit.bitwarden" to "Bitwarden",
            "com.google.android.apps.authenticator2" to "Authenticator",
            "com.example.mybank" to "Mi Banco",
            "com.kutxabank.android" to "Kutxabank",
            "es.ibercaja.ibercajaapp" to "Ibercaja",
            "es.ingdirect.android" to "ING",
            "com.transferwise.android" to "Wise",
            "de.traderepublic.app" to "Trade Republic",
            "com.android.settings" to "Settings",
            "com.whatsapp" to "WhatsApp",
            "com.google.android.apps.photos" to "Photos",
            "com.spotify.music" to "Spotify",
            "com.example.display" to "Display",
            Packages.ADBUDDY to "ADBuddy",
            "not a package" to "Bank",
        )
        assertEquals(
            setOf(
                "com.bbva.bbvacontigo", "es.lacaixa.mobile.android.newwapicon", "com.paypal.android.p2pmobile",
                "com.google.android.apps.walletnfcrel", "com.x8bit.bitwarden", "com.google.android.apps.authenticator2", "com.example.mybank",
                "com.kutxabank.android", "es.ibercaja.ibercajaapp", "es.ingdirect.android", "com.transferwise.android", "de.traderepublic.app",
            ),
            ProtectedApps.Heuristic.propose(apps),
        )
    }

    @Test fun theLabelCounts() {
        assertTrue(ProtectedApps.Heuristic.looksProtected("com.example.app", "Contraseñas"))
    }
}
