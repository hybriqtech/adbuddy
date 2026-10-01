package tech.hybriq.adbuddy.a11y

import android.content.SharedPreferences
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
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

    /** Just enough of SharedPreferences for ProtectedApps. */
    private class FakePrefs : SharedPreferences {
        val map = mutableMapOf<String, Any>()
        override fun getAll(): MutableMap<String, *> = map
        override fun getString(key: String?, defValue: String?) = map[key] as String? ?: defValue
        @Suppress("UNCHECKED_CAST")
        override fun getStringSet(key: String?, defValues: MutableSet<String>?) = map[key] as MutableSet<String>? ?: defValues
        override fun getInt(key: String?, defValue: Int) = map[key] as Int? ?: defValue
        override fun getLong(key: String?, defValue: Long) = map[key] as Long? ?: defValue
        override fun getFloat(key: String?, defValue: Float) = map[key] as Float? ?: defValue
        override fun getBoolean(key: String?, defValue: Boolean) = map[key] as Boolean? ?: defValue
        override fun contains(key: String?) = map.containsKey(key)
        override fun edit(): SharedPreferences.Editor = object : SharedPreferences.Editor {
            val pending = mutableMapOf<String, Any>()
            override fun putString(key: String, value: String?) = apply { pending[key] = value!! }
            override fun putStringSet(key: String, values: MutableSet<String>?) = apply { pending[key] = values!!.toMutableSet() }
            override fun putInt(key: String, value: Int) = apply { pending[key] = value }
            override fun putLong(key: String, value: Long) = apply { pending[key] = value }
            override fun putFloat(key: String, value: Float) = apply { pending[key] = value }
            override fun putBoolean(key: String, value: Boolean) = apply { pending[key] = value }
            override fun remove(key: String) = apply { pending.remove(key) }
            override fun clear() = apply { map.clear() }
            override fun commit(): Boolean { map.putAll(pending); return true }
            override fun apply() { map.putAll(pending) }
        }
        override fun registerOnSharedPreferenceChangeListener(l: SharedPreferences.OnSharedPreferenceChangeListener?) {}
        override fun unregisterOnSharedPreferenceChangeListener(l: SharedPreferences.OnSharedPreferenceChangeListener?) {}
    }

    @Test fun proposeReturnsWhetherItChanged() {
        val p = ProtectedApps(FakePrefs())
        val apps = listOf("com.example.mybank" to "Mi Banco", "com.whatsapp" to "WhatsApp")
        assertTrue(p.propose(apps))
        assertEquals(setOf("com.example.mybank"), p.list())
        assertFalse(p.propose(apps))
    }

    @Test fun aProposalAfterConfirmDoesNotOverwriteIt() {
        val p = ProtectedApps(FakePrefs())
        p.confirm(setOf("com.mine.app"))
        assertTrue(p.confirmed)
        assertFalse(p.propose(listOf("com.example.mybank" to "Mi Banco")))
        assertEquals(setOf("com.mine.app"), p.list())
    }

    @Test fun adbuddyIsAlwaysProtected() {
        val p = ProtectedApps(FakePrefs())
        assertTrue(p.isProtected(Packages.ADBUDDY))
        assertFalse(p.list().contains(Packages.ADBUDDY))
        assertFalse(p.isProtected("com.whatsapp"))
    }
}
