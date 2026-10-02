package tech.hybriq.adbuddy.a11y

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.FakePrefs
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.DeviceInfo
import tech.hybriq.adbuddy.bridge.UiLimits

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

    private val installedApps = listOf(
        "com.example.mybank" to "Mi Banco",
        "com.google.android.apps.messaging" to "Messages",
        "com.google.android.gm" to "Gmail",
        "com.vendor.sms" to "Vendor SMS",
        "com.whatsapp" to "WhatsApp",
    )

    @Test fun theProposalProtectsTheAppsThatReceiveCodesBeforeAnyConfirmation() {
        val p = ProtectedApps(FakePrefs())
        assertTrue(p.propose(installedApps, ProtectedApps.Heuristic.receivers("com.vendor.sms")))
        assertFalse(p.confirmed)
        assertEquals(
            setOf("com.example.mybank", "com.google.android.apps.messaging", "com.google.android.gm", "com.vendor.sms", "com.google.android.gms"),
            p.list(),
        )
        // What reaches the host in device.info protects them from screen and tap too.
        assertEquals(p.list().sorted(), p.deviceInfo().protectedApps)
        assertTrue(p.isProtected("com.google.android.gm"))
        assertFalse(p.isProtected("com.whatsapp"))
    }

    @Test fun theReceiversSurviveWhenTheHeuristicFillsTheList() {
        // A phone with as many bank-like apps as the protocol allows: the apps that hold the codes still make the list.
        val banks = (0 until UiLimits.MAX_PROTECTED_APPS).map { "com.bank$it.app" to "Bank $it" }
        val got = ProtectedApps.Heuristic.proposeWithReceivers(banks + ("com.google.android.gm" to "Gmail"), ProtectedApps.Heuristic.receivers(null))
        assertEquals(UiLimits.MAX_PROTECTED_APPS, got.size)
        assertTrue("com.google.android.gm" in got && "com.google.android.gms" in got)
    }

    @Test fun aReceiverThatIsNotInstalledIsNotProposedExceptThePlayServices() {
        val p = ProtectedApps(FakePrefs())
        p.propose(listOf("com.whatsapp" to "WhatsApp"), ProtectedApps.Heuristic.receivers(null))
        assertEquals(setOf("com.google.android.gms"), p.list())
    }

    @Test fun aNewProposalWithReceiversChangesAnOldOneAndAConfirmationStopsIt() {
        val p = ProtectedApps(FakePrefs())
        assertTrue(p.propose(installedApps)) // what M5a proposed
        assertEquals(setOf("com.example.mybank"), p.list())
        assertTrue(p.propose(installedApps, ProtectedApps.Heuristic.receivers(null))) // M5c adds the receivers
        assertTrue("com.google.android.gm" in p.list())
        assertFalse(p.propose(installedApps, ProtectedApps.Heuristic.receivers(null))) // nothing new
        p.confirm(setOf("com.mine.app"))
        assertFalse(p.propose(installedApps, ProtectedApps.Heuristic.receivers(null)))
        assertEquals(setOf("com.mine.app"), p.list())
    }

    @Test fun proposeAndConfirmAreSafeFromTwoThreads() {
        val p = ProtectedApps(FakePrefs())
        val proposer = Thread { repeat(200) { p.propose(installedApps, ProtectedApps.Heuristic.receivers(null)) } }
        proposer.start()
        p.confirm(setOf("com.mine.app"))
        proposer.join()
        // However they interleave, a confirmation is never overwritten by a proposal made after it.
        assertTrue(p.confirmed)
        assertEquals(setOf("com.mine.app"), p.list())
    }

    @Test fun confirmingAnEmptyOrUnchangedListStillConfirms() {
        val p = ProtectedApps(FakePrefs())
        p.propose(listOf("com.example.mybank" to "Mi Banco"))
        p.confirm(p.list()) // the proposal, unchanged
        assertTrue(p.confirmed)
        val q = ProtectedApps(FakePrefs())
        q.confirm(emptySet())
        assertTrue(q.confirmed)
        assertFalse(q.propose(listOf("com.example.mybank" to "Mi Banco")))
        assertEquals(emptySet<String>(), q.list())
    }

    @Test fun confirmKeepsOnlyValidPackagesAndNeverAdbuddy() {
        val p = ProtectedApps(FakePrefs())
        p.confirm(setOf(Packages.ADBUDDY, "com.ok.app", "not a package", "x"))
        assertEquals(setOf("com.ok.app"), p.list())
        assertTrue(p.isProtected(Packages.ADBUDDY))
    }

    @Test fun theHostGetsTheWholeListEvenWhenEmpty() {
        val p = ProtectedApps(FakePrefs())
        p.confirm(setOf("com.b.app", "com.a.app"))
        assertEquals(listOf("com.a.app", "com.b.app"), p.deviceInfo().protectedApps)
        p.confirm(emptySet())
        // An empty list is "protect nothing but ADBuddy", not "no change": it must be on the wire.
        val json = Protocol.json.encodeToString(DeviceInfo.serializer(), p.deviceInfo())
        assertTrue(json, json.contains("\"protectedApps\":[]"))
    }

    @Test fun adbuddyIsAlwaysProtected() {
        val p = ProtectedApps(FakePrefs())
        assertTrue(p.isProtected(Packages.ADBUDDY))
        assertFalse(p.list().contains(Packages.ADBUDDY))
        assertFalse(p.isProtected("com.whatsapp"))
    }
}
